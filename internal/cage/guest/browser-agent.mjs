// Chromium is controlled through inherited pipes, never an exposed CDP port.
import { spawn, execFile } from "node:child_process";
import { promisify } from "node:util";
const runFile = promisify(execFile);
import { readFileSync, writeFileSync, mkdirSync, existsSync } from "node:fs";
import http from "node:http";
import { auditBrowser } from "./browser-audit.mjs";
import { DownloadManager } from "./browser-downloads.mjs";
import { timingSafeEqual } from "node:crypto";

const config = JSON.parse(
  readFileSync("/etc/claude-cage/browser-control.json", "utf8"),
);
const selectionFile = "/home/cage-browser/workbench-state.json";
const homepageFile = "/home/cage-browser/homepage.json";
let homeURL = "https://www.bing.com/";
try {
  homeURL = checkedURL(JSON.parse(readFileSync(homepageFile, "utf8")).url);
} catch {}
let restoreURL = "";
try {
  restoreURL = JSON.parse(readFileSync(selectionFile, "utf8")).url || "";
} catch {}
function rememberSelection() {
  const tab = targets.get(activeId);
  if (tab)
    try {
      writeFileSync(selectionFile, JSON.stringify({ url: tab.url }), {
        mode: 0o600,
      });
    } catch {}
}
const downloadPath = "/home/cage-browser/Downloads";
mkdirSync(downloadPath, { recursive: true });
const flags = [
  "--user-data-dir=/home/cage-browser/profile",
  "--remote-debugging-pipe",
  // The pipe manages tabs for a human-operated browser. Keep it private while
  // preventing its presence alone from enabling Blink's automation marker.
  "--disable-blink-features=AutomationControlled",
  "--kiosk",
  "--no-first-run",
  "--restore-last-session",
  "--no-default-browser-check",
  "--disable-quic",
  "--disable-session-crashed-bubble",
  "--window-size=1440,900",
  "--force-device-scale-factor=2",
  `--lang=${process.env.CAGE_LANGUAGE || "en-US"}`,
];
if (!existsSync("/home/cage-browser/profile/Default/Sessions"))
  flags.push(homeURL);
const chrome = spawn("/usr/bin/chromium", flags, {
  stdio: ["ignore", "ignore", "inherit", "pipe", "pipe"],
});
let sequence = 0,
  buffer = Buffer.alloc(0),
  activeId = "",
  ready = false;
let shuttingDown = false;
const events = new Set();
const auditContexts = new Set();
const pending = new Map(),
  targets = new Map(),
  sessions = new Map();
const downloads = new DownloadManager({
  directory: "/home/cage-browser",
  changed: () => broadcastState(),
  cancel: id => cdp("Browser.cancelDownload", { guid: id }),
  save: async (id, name) => {
    let output;
    try {
      output = (await runFile("/usr/bin/sudo", ["/usr/local/bin/cage-save-download", id, name], { maxBuffer: 16384 })).stdout;
    } catch (error) {
      output = error.stdout || '{"ok":false,"error":"下载保存未完成，请检查共享目录后重试。"}';
    }
    return JSON.parse(output);
  },
});
function cdp(method, params = {}, sessionId, timeout = 4500) {
  return new Promise((resolve, reject) => {
    const id = ++sequence;
    const timer = setTimeout(() => {
      pending.delete(id);
      reject(new Error("Browser command timed out"));
    }, timeout);
    pending.set(id, { resolve, reject, timer });
    chrome.stdio[3].write(
      JSON.stringify({
        id,
        method,
        params,
        ...(sessionId ? { sessionId } : {}),
      }) + "\0",
    );
  });
}
chrome.stdio[4].on("data", (chunk) => {
  buffer = Buffer.concat([buffer, chunk]);
  let end;
  while ((end = buffer.indexOf(0)) >= 0) {
    const raw = buffer.subarray(0, end);
    buffer = buffer.subarray(end + 1);
    let event;
    try {
      event = JSON.parse(raw);
    } catch {
      continue;
    }
    if (event.id) {
      const call = pending.get(event.id);
      if (!call) continue;
      pending.delete(event.id);
      clearTimeout(call.timer);
      event.error
        ? call.reject(new Error(event.error.message))
        : call.resolve(event.result);
      continue;
    }
    const p = event.params || {};
    if (
      event.method === "Target.targetCreated" ||
      event.method === "Target.targetInfoChanged"
    ) {
      const t = p.targetInfo;
      if (t.type === "page" && !auditContexts.has(t.browserContextId)) {
        const fresh = !targets.has(t.targetId);
        targets.set(t.targetId, t);
        if (t.targetId === activeId) rememberSelection();
        broadcastState();
        if (!activeId) activeId = t.targetId;
        if (ready && fresh && t.openerId)
          void activate(t.targetId, true).catch(() => {});
      }
    }
    if (event.method === "Target.targetDestroyed") {
      if (!targets.has(p.targetId)) continue;
      targets.delete(p.targetId);
      sessions.delete(p.targetId);
      broadcastState();
      if (activeId === p.targetId) {
        activeId = [...targets.keys()][0] || "";
        if (activeId) void activate(activeId).catch(() => {});
      }
    }
    if (event.method === "Browser.downloadWillBegin")
      downloads.begin(p.guid, p.suggestedFilename);
    if (event.method === "Browser.downloadProgress") downloads.progress(p);
  }
});
chrome.on("exit", () => {
  ready = false;
  for (const p of pending.values()) {
    clearTimeout(p.timer);
    p.reject(new Error("Browser stopped"));
  }
  pending.clear();
  process.exit(shuttingDown ? 0 : 1);
});
for (const channel of chrome.stdio.slice(3))
  channel.on("error", () => {
    if (!shuttingDown) process.exit(1);
  });
async function session(id) {
  if (!targets.has(id)) throw new Error("Tab no longer exists");
  if (!sessions.has(id))
    sessions.set(
      id,
      cdp("Target.attachToTarget", { targetId: id, flatten: true }).then(
        (r) => r.sessionId,
      ),
    );
  return sessions.get(id);
}
async function activate(id, focus = false) {
  if (!targets.has(id)) throw new Error("Tab no longer exists");
  await cdp("Target.activateTarget", { targetId: id });
  activeId = id;
  rememberSelection();
  broadcastState(focus);
  const { windowId } = await cdp("Browser.getWindowForTarget", {
    targetId: id,
  });
  await cdp("Browser.setWindowBounds", {
    windowId,
    bounds: { windowState: "fullscreen" },
  }).catch(() => {});
}
function checkedURL(value) {
  if (value === "about:blank") return value;
  const u = new URL(value);
  if (!["http:", "https:"].includes(u.protocol) || u.username || u.password)
    throw new Error("Only HTTP(S) URLs are accepted");
  return u.href;
}
async function newTab(url = homeURL) {
  const { targetId } = await cdp("Target.createTarget", {
    url: checkedURL(url),
    background: false,
  });
  // The response and discovery event may arrive in either order.
  if (!targets.has(targetId))
    targets.set(targetId, { targetId, type: "page", url, title: "" });
  await activate(targetId, true);
  return { id: targetId };
}
async function editor(workspace = config.workspace, reload = false) {
  if (typeof workspace !== "string" || !workspace.startsWith("/")) throw new Error("Invalid workspace");
  const editorURL = `http://127.0.0.1:18080/?folder=${encodeURIComponent(workspace)}`;
  const existing = [...targets.values()].find((t) =>
    t.url === editorURL,
  );
  if (existing && !reload) {
    await activate(existing.targetId);
    return { id: existing.targetId };
  }
  const response = await fetch("http://127.0.0.1:18080/login", {
    method: "POST",
    redirect: "manual",
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    body: new URLSearchParams({ password: config.editorPassword }),
    signal: AbortSignal.timeout(5000),
  });
  const cookies = response.headers.getSetCookie().map((raw) => {
    const first = raw.split(";")[0],
      index = first.indexOf("=");
    return {
      name: first.slice(0, index),
      value: first.slice(index + 1),
      domain: "127.0.0.1",
      path: "/",
      httpOnly: true,
      sameSite: "Lax",
    };
  });
  if (!cookies.length) throw new Error("Editor authentication is unavailable");
  await cdp("Storage.setCookies", { cookies });
  if (existing) {
    await activate(existing.targetId);
    await cdp("Page.reload", {}, await session(existing.targetId));
    return { id: existing.targetId };
  }
  return newTab(editorURL);
}
async function restartEditor(workspace) {
  await runFile("/usr/bin/sudo", ["/usr/bin/systemctl", "restart", "cage-editor.service"], { timeout: 18000, maxBuffer: 4096 });
  const deadline = Date.now() + 5000;
  while (Date.now() < deadline) {
    try {
      const response = await fetch("http://127.0.0.1:18080/healthz", { signal: AbortSignal.timeout(1000) });
      if (response.ok) return editor(workspace || config.workspace, true);
    } catch {}
    await new Promise(resolve => setTimeout(resolve, 200));
  }
  throw new Error("编辑器尚未恢复，请稍后重试。");
}
async function tabState() {
  const { targetInfos } = await cdp("Target.getTargets");
  for (const tab of targetInfos)
    if (tab.type === "page" && !auditContexts.has(tab.browserContextId)) targets.set(tab.targetId, tab);
  let back = false,
    forward = false;
  if (activeId && targets.has(activeId)) {
    try {
      const h = await cdp(
        "Page.getNavigationHistory",
        {},
        await session(activeId),
      );
      back = h.currentIndex > 0;
      forward = h.currentIndex < h.entries.length - 1;
    } catch {}
  }
  return { ...publicState(), back, forward };
}
function displayTitle(tab) {
  if (tab.url === "about:blank") return "新标签页";
  const title = tab.title;
  if (
    !title ||
    title === tab.url ||
    title === tab.url.replace(/^https?:\/\//, "")
  ) {
    try {
      return new URL(tab.url).hostname;
    } catch {
      return "新标签页";
    }
  }
  return title.slice(0, 160);
}
function publicState() {
  return {
    active: activeId,
    downloads: downloads.snapshot(),
    tabs: [...targets.values()].map((t) => ({
      id: t.targetId,
      title: displayTitle(t),
      url: t.url,
      kind: t.url.startsWith("http://127.0.0.1:18080/") ? "editor" : "web",
      workspace: t.url.startsWith("http://127.0.0.1:18080/") ? new URL(t.url).searchParams.get("folder") || config.workspace : undefined,
    })),
  };
}
function broadcastState(focus = false) {
  const payload =
    "data: " + JSON.stringify({ ...publicState(), focus }) + "\n\n";
  for (const stream of events) stream.write(payload);
}
setInterval(() => {
  for (const stream of events) stream.write(": heartbeat\n\n");
}, 10000).unref();
let displaySize = "";
async function resizeDisplay(width, height) {
  if (
    !Number.isInteger(width) ||
    !Number.isInteger(height) ||
    width < 640 ||
    height < 480 ||
    width > 8192 ||
    height > 8192
  )
    throw new Error("Invalid display size");
  const name = `cage-${width}x${height}`;
  if (displaySize === name) return { ok: true };
  const { stdout } = await runFile("/usr/bin/xrandr", ["--query"]);
  if (!stdout.includes(name)) {
    const rate = (((width + 32) * (height + 8) * 60) / 1000000).toFixed(3);
    await runFile("/usr/bin/xrandr", [
      "--newmode",
      name,
      rate,
      String(width),
      String(width + 8),
      String(width + 16),
      String(width + 32),
      String(height),
      String(height + 1),
      String(height + 2),
      String(height + 8),
      "+HSync",
      "-VSync",
    ]);
    await runFile("/usr/bin/xrandr", ["--addmode", "VNC-0", name]);
  }
  await runFile("/usr/bin/xrandr", ["--output", "VNC-0", "--mode", name]);
  displaySize = name;
  for (const old of stdout.matchAll(/^\s+(cage-\d+x\d+)/gm))
    if (old[1] !== name) {
      await runFile("/usr/bin/xrandr", ["--delmode", "VNC-0", old[1]]).catch(
        () => {},
      );
      await runFile("/usr/bin/xrandr", ["--rmmode", old[1]]).catch(() => {});
    }
  return { ok: true };
}
let mutations = Promise.resolve();
function mutate(fn) {
  const result = mutations.then(fn);
  mutations = result.catch(() => {});
  return result;
}

const server = http.createServer(async (req, res) => {
  res.setHeader("Cache-Control", "no-store");
  res.setHeader("Content-Type", "application/json");
  const auth = Buffer.from(req.headers.authorization || ""),
    expected = Buffer.from("Bearer " + config.token);
  if (auth.length !== expected.length || !timingSafeEqual(auth, expected)) {
    res.writeHead(401);
    res.end("{}");
    return;
  }
  if (req.headers.origin) {
    res.writeHead(403);
    res.end("{}");
    return;
  }
  try {
    if (!ready) throw new Error("Browser is starting");
    if (req.method === "GET" && req.url === "/health") {
      res.end('{"ready":true}');
      return;
    }
    if (req.method === "GET" && req.url === "/events") {
      res.setHeader("Content-Type", "text/event-stream");
      res.setHeader("Connection", "keep-alive");
      res.flushHeaders();
      events.add(res);
      res.write("data: " + JSON.stringify(publicState()) + "\n\n");
      res.on("close", () => events.delete(res));
      return;
    }
    if (req.method === "GET" && req.url === "/tabs") {
      res.end(JSON.stringify(await tabState()));
      return;
    }
    if (req.method !== "POST") {
      res.writeHead(404);
      res.end("{}");
      return;
    }
    let text = "";
    for await (const chunk of req) {
      text += chunk;
      if (text.length > 65536) throw new Error("Request too large");
    }
    const input = JSON.parse(text || "{}");
    if (req.url === "/download-retry") {
      res.end(JSON.stringify(downloads.retry(input.id)));
      return;
    }
    if (req.url === "/audit") {
      if (typeof input.timezone !== "string" || input.timezone.length > 128) throw new Error("诊断时区无效");
      res.end(JSON.stringify(await auditBrowser(cdp, auditContexts, input.network === true, input.timezone)));
      return;
    }
    const result = await mutate(async () => {
      if (req.url === "/homepage") {
        const url = checkedURL(input.url);
        if (url === "about:blank") throw new Error("Homepage must be an HTTP(S) URL");
        writeFileSync(homepageFile, JSON.stringify({ url }), { mode: 0o600 });
        homeURL = url;
        for (const tab of targets.values()) {
          if (tab.url === "about:blank" || /^chrome:\/\/(newtab|new-tab-page|new-tab-page-third-party)\/?$/.test(tab.url)) {
            const sid = await session(tab.targetId);
            await cdp("Page.navigate", { url: homeURL }, sid);
          }
        }
        return { ok: true };
      }
      if (req.url === "/resize")
        return resizeDisplay(input.width, input.height);
      if (req.url === "/new") return newTab(input.url || undefined);
      if (req.url === "/editor") return editor(input.workspace || config.workspace);
      if (req.url === "/editor-restart") return restartEditor(input.workspace);
      const id = input.id || activeId;
      if (!targets.has(id)) throw new Error("Tab no longer exists");
      switch (req.url) {
        case "/insert-text":
          if (input.id !== activeId || typeof input.text !== "string" ||
              !input.text || Buffer.byteLength(input.text, "utf8") > 8192)
            throw new Error("Input target or text is invalid");
          await cdp("Input.insertText", { text: input.text }, await session(id));
          break;
        case "/activate":
          await activate(id);
          break;
        case "/close":
          if (targets.size === 1) await newTab();
          await cdp("Target.closeTarget", { targetId: id });
          break;
        case "/navigate":
          await cdp(
            "Page.navigate",
            { url: checkedURL(input.url) },
            await session(id),
          );
          break;
        case "/reload":
          await cdp("Page.reload", {}, await session(id));
          break;
        case "/back":
        case "/forward": {
          const sid = await session(id),
            h = await cdp("Page.getNavigationHistory", {}, sid),
            entry = h.entries[h.currentIndex + (req.url === "/back" ? -1 : 1)];
          if (entry)
            await cdp(
              "Page.navigateToHistoryEntry",
              { entryId: entry.id },
              sid,
            );
          break;
        }
        default:
          throw new Error("Unknown browser action");
      }
      return { ok: true };
    });
    res.end(JSON.stringify(result));
  } catch (error) {
    res.writeHead(400);
    res.end(JSON.stringify({ error: error.message }));
  }
});
server.listen(18500, "127.0.0.1");
(async () => {
  await cdp("Browser.getVersion");
  await cdp("Target.setDiscoverTargets", { discover: true });
  const { targetInfos } = await cdp("Target.getTargets");
  for (const t of targetInfos)
    if (t.type === "page") targets.set(t.targetId, t);
  await cdp("Browser.setDownloadBehavior", {
    behavior: "allowAndName",
    downloadPath,
    eventsEnabled: true,
  });
  if (!targets.size) await newTab();
  else
    await activate(
      [...targets.values()].find((t) => t.url === restoreURL)?.targetId ||
        [...targets.keys()][0],
    );
  ready = true;
})().catch(() => process.exit(1));
process.on("SIGTERM", () => {
  shuttingDown = true;
  server.close();
  cdp("Browser.close").catch(() => {});
  setTimeout(() => process.exit(0), 2000).unref();
});
