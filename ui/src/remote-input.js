import { getKeycode, getKeysym } from "../.generated/novnc/core/input/util.js";
import { readClipboard, writeClipboard, nativeAvailable } from "./desktop";

// A user gesture starts every host clipboard access. Fences and serialization
// in the shared RFB patch ensure that repeated pastes are processed in order.
export function installRemoteInput(
  rfb,
  container,
  { mode, onStatus, onFallback, isActive, insertText },
) {
  const listeners = [],
    handled = new Set(),
    physical = new Set(),
    remote = new Map();
  let lastText = "",
    copyRequested = false,
    copyTimer,
    pending = 0,
    composing = false,
    disposed = false;
  const target = document.createElement("textarea");
  target.setAttribute("aria-label", "Cage 网页输入");
  target.setAttribute("autocomplete", "off");
  target.setAttribute("autocapitalize", "off");
  target.spellcheck = false;
  target.className = "remote-text-input";
  target.style.cssText = "position:fixed;left:0;top:0";
  container.append(target);
  rfb.cageSetKeyboardTarget(target);
  const on = (object, type, fn, options) => {
    object.addEventListener(type, fn, options);
    listeners.push(() => object.removeEventListener(type, fn, options));
  };
  const focused = () =>
    isActive() &&
    (container.contains(document.activeElement) ||
      document.activeElement === target);
  const position = (x, y) => {
    if (composing) return;
    target.style.left = `${Math.max(0, Math.min(x, window.innerWidth - 280))}px`;
    target.style.top = `${Math.max(0, Math.min(y, window.innerHeight - 36))}px`;
  };
  const bounds = container.getBoundingClientRect();
  position(bounds.left + 12, bounds.top + 12);
  on(
    container,
    "pointerdown",
    (event) => position(event.clientX, event.clientY),
    true,
  );
  const commit = (text) => {
    if (disposed || !isActive() || !text) return;
    // X0tigervnc cannot map arbitrary Unicode to its finite X11 keycode pool.
    // The browser inserts committed text directly, without clipboard mutation.
    insertText(text).catch(() => {
      if (!disposed) onStatus("文字输入未完成，请重新连接画面后重试");
    });
  };
  const resetComposition = () => {
    composing = false;
    target.value = "";
  };
  on(target, "compositionstart", () => {
    composing = true;
  });
  on(target, "compositionend", (event) => {
    if (composing) commit(event.data || "");
    resetComposition();
  });
  on(target, "input", (event) => {
    if (composing || event.isComposing) return;
    // WebKit may deliver the final input after compositionend. That text has
    // already been sent. Input without composition covers emoji and dictation.
    if (
      !["insertFromComposition", "insertCompositionText"].includes(
        event.inputType,
      )
    )
      commit(target.value);
    target.value = "";
  });
  on(target, "blur", resetComposition);
  const say = (text) => {
    if (!disposed) onStatus(text);
  };
  const chord = (key) => {
    rfb.sendKey(0xffe3, "ControlLeft", true);
    rfb.sendKey(key.charCodeAt(0), "Key" + key.toUpperCase(), true);
    rfb.sendKey(key.charCodeAt(0), "Key" + key.toUpperCase(), false);
    rfb.sendKey(0xffe3, "ControlLeft", false);
  };
  const copy = (key = "c") => {
    copyRequested = true;
    chord(key);
    say("正在复制所选文本…");
    clearTimeout(copyTimer);
    copyTimer = setTimeout(() => {
      copyRequested = false;
      onFallback(lastText, "未收到新的选中文本，可在这里复制 Cage 剪贴板。");
    }, 1600);
  };
  const paste = (text) => {
    if (disposed) return;
    pending++;
    say("正在粘贴…");
    rfb.focus();
    rfb.cagePasteText(text).then(
      () => {
        pending--;
        say(pending ? "正在粘贴…" : "已粘贴");
      },
      () => {
        pending--;
        say("粘贴连接未就绪，请重新连接画面后重试");
      },
    );
  };
  const readAndPaste = async () => {
    try {
      paste(await readClipboard());
    } catch {
      onFallback("", "在此粘贴文本，然后发送到 Cage。");
    }
  };
  on(rfb, "clipboard", async (event) => {
    lastText = event.detail.text;
    if (!copyRequested) return;
    copyRequested = false;
    clearTimeout(copyTimer);
    try {
      await writeClipboard(lastText);
      say("已复制到本机");
    } catch {
      onFallback(lastText, "请在这里复制到本机。");
    }
  });
  const release = () => {
    for (const [code, keysym] of remote) rfb.sendKey(keysym, code, false);
    remote.clear();
    physical.clear();
    handled.clear();
  };
  on(window, "blur", release);
  on(
    document,
    "keydown",
    (event) => {
      if (!focused()) return;
      if (
        composing ||
        event.isComposing ||
        event.keyCode === 229 ||
        ["Process", "Dead"].includes(event.key)
      ) {
        // Leave the default action to the host IME; noVNC must not cancel it
        // or send candidate navigation/confirmation keys into the guest.
        event.stopImmediatePropagation();
        return;
      }
      const code = getKeycode(event),
        mapping = mode() === "mac",
        malformedCommandLetter =
          event.metaKey &&
          /^Key[A-Z]$/.test(code) &&
          !/^[a-z]$/i.test(event.key),
        key = malformedCommandLetter
          ? code[3].toLowerCase()
          : event.key.toLowerCase();
      if (["Shift", "Control", "Alt"].includes(event.key)) physical.add(code);
      if (
        mapping &&
        event.metaKey &&
        ["Shift", "Control", "Alt"].includes(event.key)
      ) {
        const sym = getKeysym(event);
        rfb.sendKey(sym, code, true);
        remote.set(code, sym);
        event.preventDefault();
        event.stopImmediatePropagation();
        return;
      }
      const clipboard =
        (event.metaKey || event.ctrlKey) &&
        !event.altKey &&
        ((!event.shiftKey && ["c", "x"].includes(key)) || key === "v");
      if (clipboard) {
        event.stopImmediatePropagation();
        handled.add(code);
        if (key === "v") {
          if (nativeAvailable()) {
            event.preventDefault();
            void readAndPaste();
          } else {
            target.value = "";
            target.focus({ preventScroll: true });
          }
        } else {
          event.preventDefault();
          copy(key);
        }
        return;
      }
      if (
        mapping &&
        event.metaKey &&
        !["Meta", "Shift", "Control", "Alt", "CapsLock"].includes(event.key)
      ) {
        let sym = getKeysym(event);
        if (
          (malformedCommandLetter || sym === null || sym < 32) &&
          /^Key[A-Z]$/.test(code)
        )
          sym = (event.shiftKey ? code[3] : code[3].toLowerCase()).codePointAt(
            0,
          );
        if (sym !== null) {
          const held = (p) => [...physical].some((c) => c.startsWith(p)),
            ctrl = physical.has("ControlRight")
              ? [0xffe4, "ControlRight"]
              : [0xffe3, "ControlLeft"],
            shift = physical.has("ShiftRight")
              ? [0xffe2, "ShiftRight"]
              : [0xffe1, "ShiftLeft"];
          rfb.sendKey(...ctrl, true);
          if (event.shiftKey) rfb.sendKey(...shift, true);
          rfb.sendKey(sym, code, true);
          rfb.sendKey(sym, code, false);
          if (event.shiftKey && !held("Shift")) rfb.sendKey(...shift, false);
          if (!event.ctrlKey || !held("Control")) rfb.sendKey(...ctrl, false);
          event.preventDefault();
          event.stopImmediatePropagation();
          handled.add(code);
          return;
        }
      }
      if (mapping && event.key === "Meta") event.stopImmediatePropagation();
    },
    true,
  );
  on(
    document,
    "keyup",
    (event) => {
      const code = getKeycode(event);
      // noVNC ignores releases for keys it never sent. Let keyup through so a
      // modifier pressed before composition can still be released normally.
      physical.delete(code);
      if (remote.has(code)) {
        rfb.sendKey(remote.get(code), code, false);
        remote.delete(code);
        event.stopImmediatePropagation();
        return;
      }
      if (
        handled.delete(code) ||
        (focused() && mode() === "mac" && event.key === "Meta")
      )
        event.stopImmediatePropagation();
    },
    true,
  );
  on(
    document,
    "paste",
    (event) => {
      if (!focused() && event.target !== target) return;
      event.preventDefault();
      event.stopImmediatePropagation();
      if (event.clipboardData.types.includes("text/plain"))
        paste(event.clipboardData.getData("text/plain"));
      else say("当前支持文本粘贴");
    },
    true,
  );
  return {
    copy,
    readAndPaste,
    paste,
    text: () => lastText,
    edit(action) {
      if (composing) return false;
      if (action === "copy") copy();
      else if (action === "cut") copy("x");
      else if (action === "paste") void readAndPaste();
      else if (action === "selectAll") chord("a");
      else if (action === "undo") chord("z");
      else if (action === "redo") {
        rfb.sendKey(0xffe3, "ControlLeft", true);
        rfb.sendKey(0xffe1, "ShiftLeft", true);
        rfb.sendKey(0x5a, "KeyZ", true);
        rfb.sendKey(0x5a, "KeyZ", false);
        rfb.sendKey(0xffe1, "ShiftLeft", false);
        rfb.sendKey(0xffe3, "ControlLeft", false);
      } else return false;
      return true;
    },
    dispose() {
      disposed = true;
      clearTimeout(copyTimer);
      release();
      listeners.forEach((remove) => remove());
      rfb.cageSetKeyboardTarget(null);
      target.remove();
    },
  };
}
