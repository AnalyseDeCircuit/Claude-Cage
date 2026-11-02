import { native, nativeAvailable } from "./desktop.js";

// Existing imports remain valid; desktop callers use desktop.js directly.
export {
  native,
  nativeAvailable,
  readClipboard,
  writeClipboard,
} from "./desktop.js";

let sessionRenewal;
async function responseError(response) {
  const error = new Error(
    (await response.text()).trim() || "连接未完成，请重试",
  );
  error.status = response.status;
  return error;
}
async function renewSession() {
  if (!sessionRenewal) {
    sessionRenewal = (async () => {
      const { url } = await native("refreshSession");
      const entry = new URL(url);
      if (entry.origin !== location.origin) {
        await native("reconnect");
        throw new Error("工作台地址已更新，连接恢复后请重试刚才的操作。");
      }
      const response = await fetch("/api/session", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ token: entry.hash.slice(1) }),
      });
      if (!response.ok) throw await responseError(response);
    })().finally(() => {
      sessionRenewal = null;
    });
  }
  return sessionRenewal;
}
export async function api(path, body, method, { signal } = {}) {
  const options = {
    method: method || (body === undefined ? "GET" : "POST"),
    headers: body === undefined ? {} : { "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
    signal,
  };
  let response = await fetch("/api" + path, options);
  // Authentication fails before a handler runs, so retrying this rejected
  // request once does not repeat an accepted mutation.
  if (response.status === 401 && path !== "/session" && nativeAvailable()) {
    await renewSession();
    signal?.throwIfAborted();
    response = await fetch("/api" + path, options);
  }
  if (!response.ok) throw await responseError(response);
  return response.json();
}

export async function initializeSession(signal) {
  const token = location.hash.slice(1);
  if (!token) return;
  await api("/session", { token }, undefined, { signal });
  signal?.throwIfAborted();
  history.replaceState(null, "", location.pathname);
}

export function controlSocketURL(path) {
  return `${location.protocol === "https:" ? "wss" : "ws"}://${location.host}/api${path}`;
}
export function normalizeURL(text) {
  const value = text.trim();
  if (!value || value === "about:blank") return "about:blank";
  if (/^https?:\/\//i.test(value)) return new URL(value).href;
  if (/^[a-z][a-z\d+.-]*:/i.test(value) && !/^localhost:\d+/i.test(value))
    throw new Error("地址仅支持 http 或 https");
  if (
    !/\s/.test(value) &&
    (value.includes(".") || /^localhost(?::\d+)?(?:\/|$)/i.test(value))
  )
    return new URL(
      (/^localhost|^127\./.test(value) ? "http://" : "https://") + value,
    ).href;
  return "https://www.google.com/search?q=" + encodeURIComponent(value);
}
