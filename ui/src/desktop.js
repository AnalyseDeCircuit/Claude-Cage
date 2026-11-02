// Current macOS WebKit bridge. Host UI capabilities stay separate from the
// authenticated Go service API; selecting a directory does not authorize it.
export const nativeAvailable = () =>
  Boolean(window.webkit?.messageHandlers?.cage);

export async function native(action, extra = {}) {
  return window.webkit.messageHandlers.cage.postMessage({ action, ...extra });
}

export const readClipboard = () =>
  nativeAvailable() ? native("clipboardRead") : navigator.clipboard.readText();

export const writeClipboard = (text) =>
  nativeAvailable()
    ? native("clipboardWrite", { text })
    : navigator.clipboard.writeText(text);
