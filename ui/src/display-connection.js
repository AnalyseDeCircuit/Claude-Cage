import RFB from "../.generated/novnc/core/rfb.js";
import { api, controlSocketURL } from "./api.js";
import { installRemoteInput } from "./remote-input.js";

// Owns one visible display connection, its bounded retries and resize/input
// resources. Closing it never sends an environment or browser stop command.
export function connectDisplay(element, { current, retryState, onState }) {
  let closed = false,
    client,
    input,
    clearAttempt = () => {};

  function connect() {
    if (closed) return;
    clearAttempt();
    let disposed = false,
      resizeObserver,
      resizeTimer,
      reconnectTimer;
    const request = new AbortController();
    let rfb, remoteInput;
    clearAttempt = () => {
      disposed = true;
      request.abort();
      clearTimeout(resizeTimer);
      clearTimeout(reconnectTimer);
      resizeObserver?.disconnect();
      remoteInput?.dispose();
      rfb?.disconnect();
      input = client = null;
    };
    const command = (path, body) =>
      api(path, body, undefined, { signal: request.signal });
    onState("connecting");
    const retry = () => {
      if (disposed) return;
      onState("disconnected");
      clearTimeout(reconnectTimer);
      reconnectTimer = setTimeout(
        connect,
        Math.min(1000 * 2 ** retryState.failures++, 5000),
      );
    };
    command("/vnc-credentials")
      .then(({ password }) => {
        if (disposed) return;
        rfb = new RFB(element, controlSocketURL("/vnc"), {
          credentials: { password },
        });
        client = rfb;
        rfb.scaleViewport = true;
        rfb.resizeSession = false;
        rfb.background = "#ffffff";
        rfb.qualityLevel = 8;
        rfb.compressionLevel = 2;
        rfb.addEventListener("connect", () => {
          if (disposed) return;
          retryState.failures = 0;
          onState("connected");
          const resize = () => {
            clearTimeout(resizeTimer);
            resizeTimer = setTimeout(() => {
              if (disposed) return;
              const { width, height } = element.getBoundingClientRect();
              if (width >= 320 && height >= 240)
                command("/browser/resize", {
                  width: Math.min(8192, Math.floor(width * 2)),
                  height: Math.min(8192, Math.floor(height * 2)),
                }).catch(() => {
                  if (!disposed)
                    current().onStatus("画面尺寸未能调整，请重新连接画面");
                });
            }, 160);
          };
          resizeObserver = new ResizeObserver(resize);
          resizeObserver.observe(element);
          resize();
          remoteInput = installRemoteInput(rfb, element, {
            insertText: (text) => {
              const id = current().tabId;
              return rfb.cageRunInput(() => {
                if (disposed || !current().visible || current().tabId !== id)
                  throw new Error("Input target changed");
                return command("/browser/insert-text", { id, text });
              });
            },
            mode: () => current().keyboard,
            isActive: () => !disposed && current().visible,
            onStatus: (text) => {
              if (!disposed) current().onStatus(text);
            },
            onFallback: (...args) => {
              if (!disposed) current().onFallback(...args);
            },
          });
          input = remoteInput;
          if (current().visible) rfb.focus();
        });
        rfb.addEventListener("disconnect", () => {
          if (disposed) return;
          remoteInput?.dispose();
          remoteInput = input = null;
          retry();
        });
        rfb.addEventListener("securityfailure", retry);
      })
      .catch(retry);
  }
  connect();
  return {
    get client() {
      return client;
    },
    get input() {
      return input;
    },
    reconnect: connect,
    close() {
      closed = true;
      clearAttempt();
    },
  };
}
