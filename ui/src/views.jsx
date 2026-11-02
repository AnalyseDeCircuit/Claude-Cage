import React, {
  useEffect,
  useRef,
  useState,
  useImperativeHandle,
  forwardRef,
} from "react";
import { useAppearance } from "./appearance";
import { terminalTheme, systemCodeFont } from "./appearance-state.mjs";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { readClipboard, writeClipboard } from "./desktop";
import { connectDisplay } from "./display-connection";
import { connectTerminal } from "./terminal-connection";
import { useWindowVisibility } from "./window-visibility";
import { RefreshCw, Clipboard, ArrowUpRight } from "lucide-react";
import "@xterm/xterm/css/xterm.css";

export const RemoteView = forwardRef(function RemoteView(
  { visible, keyboard, tabId, onStatus, onFallback },
  ref,
) {
  const windowVisible = useWindowVisibility();
  const screenVisible = visible && windowVisible;
  const box = useRef(),
    display = useRef(),
    current = useRef({ visible, keyboard, onStatus, onFallback });
  current.current = { visible: screenVisible, keyboard, tabId, onStatus, onFallback };
  const retryState = useRef({ failures: 0 });
  const [connection, setConnection] = useState("connecting");
  useImperativeHandle(ref, () => ({
    focus: () => display.current?.client?.focus(),
    edit: (action) =>
      box.current?.contains(document.activeElement)
        ? display.current?.input?.edit(action)
        : false,
    copy: () => display.current?.input?.copy(),
    paste: () => display.current?.input?.readAndPaste(),
    pasteText: (text) => display.current?.input?.paste(text),
    transfer: () =>
      onFallback(display.current?.input?.text() || "", "可在此使用本机复制、粘贴。"),
  }));
  useEffect(() => {
    if (!screenVisible) return;
    const next = connectDisplay(box.current, {
      current: () => current.current,
      retryState: retryState.current,
      onState: setConnection,
    });
    display.current = next;
    return () => {
      next.close();
      display.current = null;
    };
  }, [screenVisible]);
  useEffect(() => {
    if (screenVisible && connection === "connected") {
      const t = setTimeout(() => display.current?.client?.focus(), 80);
      return () => clearTimeout(t);
    }
  }, [screenVisible, connection]);
  return (
    <div className={`remote-view ${visible ? "" : "concealed"}`}>
      <div className="remote-canvas" ref={box} />
      {connection !== "connected" && (
        <div className="view-overlay">
          <RefreshCw className={connection === "connecting" ? "spin" : ""} />
          <h3>
            {connection === "connecting" ? "正在连接画面" : "画面连接已断开"}
          </h3>
          <p>后台任务不会因画面断开而自动结束。</p>
          {connection === "disconnected" && (
            <button
              className="primary"
              onClick={() => display.current?.reconnect()}
            >
              重新连接画面
            </button>
          )}
        </div>
      )}
    </div>
  );
});

export function TerminalView({ session, visible, onStatus, onControls }) {
  const { appearance, theme, variant } = useAppearance();
  const box = useRef(),
    instance = useRef(),
    fit = useRef(),
    socket = useRef();
  const [connected, setConnected] = useState(false),
    [attempt, setAttempt] = useState(0);
  const visibleRef = useRef(visible);
  visibleRef.current = visible;
  useEffect(() => {
    const terminal = new Terminal({
      fontFamily: theme.fonts.code || systemCodeFont,
      fontSize: appearance.codeFontSize,
      lineHeight: 1.3,
      cursorBlink: true,
      minimumContrastRatio: 4.5,
      scrollback: 6000,
      theme: terminalTheme(theme, variant),
    });
    const fitter = new FitAddon();
    terminal.loadAddon(fitter);
    terminal.open(box.current);
    instance.current = terminal;
    fit.current = fitter;
    setConnected(false);
    const resize = () => {
      if (!visibleRef.current) return;
      try {
        fitter.fit();
        socket.current?.resize(terminal.cols, terminal.rows);
      } catch {}
    };
    const channel = connectTerminal(session.id, {
      onOpen() {
        setConnected(true);
        resize();
        if (visibleRef.current) terminal.focus();
      },
      onData: (data) => terminal.write(data),
      onClose: () => setConnected(false),
    });
    socket.current = channel;
    const input = terminal.onData((data) => channel.write(data));
    onControls?.({
      edit(action) {
        if (!box.current?.contains(document.activeElement)) return false;
        if (action === "copy") {
          if (terminal.hasSelection())
            void writeClipboard(terminal.getSelection()).catch(() =>
              onStatus("无法写入本机剪贴板"),
            );
          return true;
        }
        if (action === "paste") {
          void readClipboard()
            .then((text) => terminal.paste(text))
            .catch(() => onStatus("无法读取本机剪贴板"));
          return true;
        }
        if (action === "selectAll") {
          terminal.selectAll();
          return true;
        }
        return false;
      },
    });
    terminal.attachCustomKeyEventHandler((event) => {
      if (!event.metaKey || event.type !== "keydown") return true;
      const key = event.key.toLowerCase();
      if (key === "c") {
        event.preventDefault();
        if (terminal.hasSelection())
          writeClipboard(terminal.getSelection())
            .then(() => onStatus("已复制终端选中文本"))
            .catch(() => onStatus("请使用右键菜单复制选中文本"));
        return false;
      }
      if (key === "v") {
        event.preventDefault();
        readClipboard()
          .then((text) => terminal.paste(text))
          .catch(() => onStatus("请使用浏览器的粘贴菜单"));
        return false;
      }
      if (key === "a") {
        event.preventDefault();
        terminal.selectAll();
        return false;
      }
      if (["+", "=", "-"].includes(key)) {
        event.preventDefault();
        terminal.options.fontSize = Math.max(
          10,
          Math.min(26, terminal.options.fontSize + (key === "-" ? -1 : 1)),
        );
        resize();
        return false;
      }
      return !event.metaKey;
    });
    const observer = new ResizeObserver(resize);
    observer.observe(box.current);
    resize();
    return () => {
      onControls?.(null);
      observer.disconnect();
      input.dispose();
      channel.close();
      socket.current = null;
      terminal.dispose();
      instance.current = null;
    };
  }, [session.id, attempt]);
  useEffect(() => {
    if (visible) {
      fit.current?.fit();
      instance.current?.focus();
      const t = instance.current;
      if (t) socket.current?.resize(t.cols, t.rows);
    }
  }, [visible]);
  useEffect(() => {
    if (!instance.current) return;
    instance.current.options.theme = terminalTheme(theme, variant);
    instance.current.options.fontFamily = theme.fonts.code || systemCodeFont;
    instance.current.options.fontSize = appearance.codeFontSize;
    if (visibleRef.current) fit.current?.fit();
  }, [theme, variant, appearance.codeFontSize]);
  return (
    <div className={`terminal-view ${visible ? "" : "concealed"}`}>
      <div className="terminal-host" ref={box} />
      {!connected && (
        <div className="terminal-note">
          {session.closed
            ? "终端会话已结束。可从项目旁的 + 新建终端。"
            : "正在连接终端…"}
          {!session.closed && (
            <button onClick={() => setAttempt((v) => v + 1)}>
              <RefreshCw size={13} />
              重新连接
            </button>
          )}
        </div>
      )}
    </div>
  );
}

export function Modal({ title, children, onClose, wide = false, footer }) {
  const panel = useRef(),
    closeRef = useRef(onClose);
  closeRef.current = onClose;
  useEffect(() => {
    const previous = document.activeElement;
    const first = panel.current.querySelector(
      "input:not([disabled]),button:not([disabled]),select:not([disabled])",
    );
    first?.focus();
    const key = (e) => {
      if (
        [...document.querySelectorAll('[role="dialog"]')].at(-1) !==
        panel.current
      )
        return;
      if (e.key === "Escape") {
        e.preventDefault();
        e.stopPropagation();
        closeRef.current?.();
      }
      if (e.key === "Tab") {
        const nodes = [
          ...panel.current.querySelectorAll(
            "input:not([disabled]),button:not([disabled]),select:not([disabled]),textarea:not([disabled]),a[href]",
          ),
        ].filter((n) => n.offsetParent !== null);
        if (!nodes.length) return;
        const index = nodes.indexOf(document.activeElement);
        if (e.shiftKey && index <= 0) {
          e.preventDefault();
          nodes.at(-1).focus();
        } else if (!e.shiftKey && (index === nodes.length - 1 || index < 0)) {
          e.preventDefault();
          nodes[0].focus();
        }
      }
    };
    const closeEvent = () => {
      if (
        [...document.querySelectorAll('[role="dialog"]')].at(-1) ===
        panel.current
      )
        closeRef.current?.();
    };
    document.addEventListener("cage-close-modal", closeEvent);
    document.addEventListener("keydown", key, true);
    return () => {
      document.removeEventListener("keydown", key, true);
      document.removeEventListener("cage-close-modal", closeEvent);
      previous?.focus?.();
    };
  }, []);
  return (
    <div
      className="modal-scrim"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onClose?.();
      }}
    >
      <section
        ref={panel}
        className={`modal ${wide ? "wide" : ""}`}
        role="dialog"
        aria-modal="true"
        aria-label={title}
      >
        <header className="modal-head">
          <h2>{title}</h2>
          {onClose && (
            <button
              className="icon-button close-modal"
              aria-label="关闭面板"
              onClick={onClose}
            >
              ×
            </button>
          )}
        </header>
        {children}
        {footer && <footer className="modal-footer">{footer}</footer>}
      </section>
    </div>
  );
}
