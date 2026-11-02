import test from "node:test";
import assert from "node:assert/strict";
import { connectWorkbench } from "./workbench-connection.js";
import { connectTerminal } from "./terminal-connection.js";

const settle = () => new Promise((resolve) => setImmediate(resolve));
const json = (value) => new Response(JSON.stringify(value));
function runtime(t, handle) {
  const saved = Object.fromEntries(
    ["fetch", "window", "location", "history", "EventSource", "WebSocket"].map(
      (key) => [key, globalThis[key]],
    ),
  );
  const calls = [],
    events = [],
    timers = new Map(),
    notices = [],
    snapshots = [],
    updates = [],
    browserEvents = [];
  const status = { ready: true, job: { running: false } };
  globalThis.location = new URL("http://localhost:17777/#entry-ticket");
  globalThis.history = {
    replaceState() {
      globalThis.location.hash = "";
    },
  };
  globalThis.window = new EventTarget();
  globalThis.EventSource = class {
    constructor(path) {
      this.path = path;
      events.push(this);
    }
    close() {
      this.closed = true;
    }
  };
  globalThis.fetch = async (path, options) => {
    calls.push({ path, options });
    const response = await handle?.(path, options);
    if (response) return response;
    switch (path) {
      case "/api/session":
        return json({});
      case "/api/settings":
        return json({ workspace: "/project" });
      case "/api/proxy":
        return json({ selected: "node" });
      case "/api/status":
        return json(status);
      case "/api/browser/tabs":
        return json({ tabs: [{ id: "initial" }], active: "initial" });
      case "/api/terminals":
        return json([{ id: "terminal-1" }]);
      default:
        throw Error(`Unexpected request ${path}`);
    }
  };
  t.mock.method(globalThis, "setTimeout", (callback, delay) => {
    const key = Symbol();
    timers.set(key, { callback, delay });
    return key;
  });
  t.mock.method(globalThis, "clearTimeout", (key) => timers.delete(key));
  const connection = connectWorkbench({
    visible: true,
    onState: (change) => updates.push(change),
    onSnapshot: (snapshot) => snapshots.push(snapshot),
    onBrowser: (snapshot) => browserEvents.push(snapshot),
    onNotice: (message) => notices.push(message),
  });
  t.after(() => {
    connection.close();
    for (const [key, value] of Object.entries(saved)) {
      if (value === undefined) delete globalThis[key];
      else globalThis[key] = value;
    }
  });
  return {
    connection,
    calls,
    events,
    timers,
    status,
    notices,
    snapshots,
    updates,
    browserEvents,
  };
}

test("workbench owns session startup, polling cadence and its event subscription", async (t) => {
  const state = runtime(t);
  const { connection, calls, events, timers, status, snapshots, updates } =
    state;
  await settle();
  assert.equal(location.hash, "");
  assert.equal(calls[0].path, "/api/session");
  assert.deepEqual(JSON.parse(calls[0].options.body), {
    token: "entry-ticket",
  });
  assert.equal(events.length, 1);
  assert.equal(events[0].path, "/api/browser/events");
  assert.equal(snapshots[0].terminals[0].id, "terminal-1");
  assert.equal([...timers.values()][0].delay, 3000);
  events[0].onopen();
  events[0].onmessage({
    data: JSON.stringify({
      tabs: [{ id: "event-tab" }],
      active: "event-tab",
      focus: true,
    }),
  });
  const browserReads = calls.filter(
    (call) => call.path === "/api/browser/tabs",
  ).length;
  connection.setVisible(false);
  await connection.refresh();
  assert.equal([...timers.values()][0].delay, 15000);
  assert.equal(
    calls.filter((call) => call.path === "/api/browser/tabs").length,
    browserReads,
  );
  assert.equal(snapshots.at(-1).browser.active, "event-tab");
  status.job = { running: true };
  connection.setVisible(true);
  await settle();
  assert.equal([...timers.values()][0].delay, 1200);
  status.ready = false;
  // The explicit stop-and-exit flow also reads status outside the poller.
  connection.updateStatus(status);
  assert.equal(events[0].closed, true);
  status.ready = true;
  await connection.refresh();
  assert.equal(events.length, 2);
  const oldMessage = events[1].onmessage;
  const count = updates.length;
  const requests = calls.length;
  connection.close();
  oldMessage({ data: '{"tabs":[]}' });
  connection.updateStatus(status);
  await connection.refreshSettings();
  assert.equal(updates.length, count);
  assert.equal(calls.length, requests);
  assert.equal(events.length, 2);
  assert.equal(events[1].closed, true);
  assert.equal(timers.size, 0);
  assert.ok(calls.every((call) => call.options.signal.aborted));
  assert.ok(!calls.some((call) => /\/stop$|\/browser\/close$/.test(call.path)));
});

test("a slow browser snapshot cannot replace a newer event", async (t) => {
  let release;
  const waiting = new Promise((resolve) => {
    release = resolve;
  });
  const state = runtime(t, (path) =>
    path === "/api/browser/tabs" ? waiting : null,
  );
  await settle();
  state.events[0].onmessage({ data: '{"tabs":[{"id":"new"}],"active":"new"}' });
  release(json({ tabs: [{ id: "old" }], active: "old" }));
  await settle();
  assert.equal(state.snapshots[0].browser.active, "new");
});

test("disposing the workbench aborts an in-flight poll without reporting a failure", async (t) => {
  let aborted = false;
  const state = runtime(t, (path, { signal }) => {
    if (path !== "/api/status") return null;
    return new Promise((resolve, reject) =>
      signal.addEventListener(
        "abort",
        () => {
          aborted = true;
          reject(signal.reason);
        },
        { once: true },
      ),
    );
  });
  await settle();
  const count = state.updates.length;
  state.connection.close();
  await settle();
  assert.equal(aborted, true);
  assert.equal(state.updates.length, count);
  assert.equal(state.timers.size, 0);
});

test("terminal channel preserves the protocol and ignores callbacks after disposal", (t) => {
  const saved = {
    location: globalThis.location,
    WebSocket: globalThis.WebSocket,
  };
  const sockets = [],
    received = [];
  globalThis.location = new URL("http://localhost:17777/");
  globalThis.WebSocket = class {
    static OPEN = 1;
    constructor(url) {
      this.url = url;
      this.sent = [];
      sockets.push(this);
    }
    send(value) {
      this.sent.push(JSON.parse(value));
    }
    close() {
      this.closed = true;
    }
  };
  t.after(() => {
    for (const [key, value] of Object.entries(saved)) {
      if (value === undefined) delete globalThis[key];
      else globalThis[key] = value;
    }
  });
  const connection = connectTerminal("session-1", {
    onOpen: () => received.push("open"),
    onData: (data) => received.push([...data]),
    onClose: () => received.push("close"),
  });
  const ws = sockets[0];
  assert.equal(ws.url, "ws://localhost:17777/api/terminals/session-1/stream");
  assert.equal(ws.binaryType, "arraybuffer");
  connection.write("ignored-before-open");
  ws.readyState = WebSocket.OPEN;
  ws.onopen();
  connection.write("hello\n");
  connection.resize(120, 40);
  ws.onmessage({ data: new Uint8Array([65]).buffer });
  assert.deepEqual(ws.sent, [
    { type: "input", data: "hello\n" },
    { type: "resize", cols: 120, rows: 40 },
  ]);
  const late = ws.onclose;
  connection.close();
  late();
  connection.write("ignored-after-close");
  assert.deepEqual(received, ["open", [65]]);
  assert.equal(ws.closed, true);
  assert.equal(sockets.length, 1);
});
