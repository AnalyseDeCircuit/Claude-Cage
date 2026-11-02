import { api, initializeSession } from "./api.js";

// Owns service snapshots, polling and one browser event subscription. View
// selection, tab ordering and local preferences remain with the interface.
export function connectWorkbench({
  visible,
  onState,
  onSnapshot,
  onBrowser,
  onNotice,
}) {
  const lifetime = new AbortController();
  let closed = false,
    started = false,
    polling = false,
    timer,
    stream,
    streamReady = false,
    initialized = false,
    lastJob = "",
    browser = { tabs: [] },
    browserRevision = 0;
  const request = (path) =>
    api(path, undefined, undefined, { signal: lifetime.signal });
  const publish = (change) => {
    if (!closed) onState(change);
  };

  function subscribe(ready) {
    if (!ready) {
      stream?.close();
      stream = null;
      streamReady = false;
      return;
    }
    if (stream) return;
    const current = new EventSource("/api/browser/events");
    stream = current;
    const live = () => !closed && stream === current;
    current.onopen = () => {
      if (live()) streamReady = true;
    };
    current.onerror = () => {
      if (live()) streamReady = false;
    };
    current.onmessage = (event) => {
      if (!live()) return;
      const next = JSON.parse(event.data);
      browserRevision++;
      browser = { ...browser, ...next };
      publish({ browser });
      onBrowser(next);
    };
  }

  async function refreshSettings() {
    if (closed) return;
    const results = await Promise.allSettled([
      request("/settings"),
      request("/proxy"),
    ]);
    if (closed) return;
    if (results[0].status === "fulfilled")
      publish({ settings: results[0].value });
    else onNotice(results[0].reason.message);
    publish({
      proxy: results[1].status === "fulfilled" ? results[1].value : null,
    });
  }

  function updateStatus(status) {
    if (closed) return;
    publish({ status, connectionError: "" });
    subscribe(status.ready);
  }

  async function refresh() {
    if (closed || !started || polling) return;
    clearTimeout(timer);
    polling = true;
    let jobRunning = false;
    try {
      const status = await request("/status");
      if (closed) return;
      jobRunning = status.job.running;
      updateStatus(status);
      if (!status.job.running && status.job.id && lastJob !== status.job.id) {
        lastJob = status.job.id;
        await refreshSettings();
        if (!closed && status.job.error) onNotice(status.job.error);
      }
      if (status.ready && !closed) {
        const revision = browserRevision;
        const [nextBrowser, terminals] = await Promise.all([
          !streamReady || !initialized ? request("/browser/tabs") : null,
          request("/terminals"),
        ]);
        if (closed) return;
        // An event received during the request is newer than its HTTP snapshot.
        if (nextBrowser && revision === browserRevision) browser = nextBrowser;
        publish({ browser });
        onSnapshot({ browser, terminals });
        initialized = true;
      }
    } catch (error) {
      publish({ connectionError: error.message });
    } finally {
      polling = false;
      if (!closed)
        timer = setTimeout(refresh, jobRunning ? 1200 : visible ? 3000 : 15000);
    }
  }

  (async () => {
    try {
      await initializeSession(lifetime.signal);
      const settings = await request("/settings");
      if (closed) return;
      publish({ settings });
      window.dispatchEvent(new Event("cage-session-ready"));
      try {
        const proxy = await request("/proxy");
        publish({ proxy });
      } catch {
        // Settings remain usable when the proxy core cannot be read.
      }
      if (closed) return;
      started = true;
      await refresh();
    } catch (error) {
      publish({ fatal: error.message });
    }
  })();

  return {
    refresh,
    refreshSettings,
    updateStatus,
    setVisible(next) {
      const resumed = next && !visible;
      visible = next;
      if (resumed) void refresh();
    },
    close() {
      closed = true;
      clearTimeout(timer);
      subscribe(false);
      lifetime.abort();
    },
  };
}
