import test from "node:test";
import assert from "node:assert/strict";
import {
  native,
  nativeAvailable,
  readClipboard,
  writeClipboard,
} from "./desktop.js";
import * as api from "./api.js";

function desktopRuntime(t, window, clipboard) {
  const saved = Object.fromEntries(
    ["window", "navigator"].map((key) => [
      key,
      Object.getOwnPropertyDescriptor(globalThis, key),
    ]),
  );
  Object.defineProperty(globalThis, "window", {
    configurable: true,
    value: window,
  });
  Object.defineProperty(globalThis, "navigator", {
    configurable: true,
    value: { clipboard },
  });
  t.after(() => {
    for (const [key, descriptor] of Object.entries(saved)) {
      if (descriptor) Object.defineProperty(globalThis, key, descriptor);
      else delete globalThis[key];
    }
  });
}

test("desktop calls retain the WebKit payload, clipboard target and folder cancellation", async (t) => {
  const calls = [];
  desktopRuntime(
    t,
    {
      webkit: {
        messageHandlers: {
          cage: {
            postMessage: async (body) => {
              calls.push(body);
              if (body.action === "clipboardRead") return "中文🙂";
              if (body.action === "chooseFolder") return "";
              return true;
            },
          },
        },
      },
    },
    {
      readText() {
        throw Error("unexpected browser clipboard read");
      },
      writeText() {
        throw Error("unexpected browser clipboard write");
      },
    },
  );
  assert.equal(nativeAvailable(), true);
  assert.equal(await readClipboard(), "中文🙂");
  await writeClipboard("文本🙂");
  assert.equal(await native("chooseFolder"), "");
  assert.deepEqual(calls, [
    { action: "clipboardRead" },
    { action: "clipboardWrite", text: "文本🙂" },
    { action: "chooseFolder" },
  ]);
  for (const [name, operation] of Object.entries({
    native,
    nativeAvailable,
    readClipboard,
    writeClipboard,
  }))
    assert.equal(api[name], operation, `existing ${name} import changed`);
});

test("ordinary browser clipboard keeps its existing permission failure", async (t) => {
  const writes = [];
  const denied = new Error("clipboard permission denied");
  let reject = false;
  desktopRuntime(
    t,
    {},
    {
      async readText() {
        if (reject) throw denied;
        return "browser text";
      },
      async writeText(text) {
        writes.push(text);
      },
    },
  );
  assert.equal(nativeAvailable(), false);
  assert.equal(await readClipboard(), "browser text");
  await writeClipboard("browser write");
  assert.deepEqual(writes, ["browser write"]);
  reject = true;
  await assert.rejects(readClipboard(), (error) => error === denied);
});
