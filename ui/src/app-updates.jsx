import React, { useEffect, useRef, useState } from "react";
import { ArrowDownToLine, Check, LoaderCircle, RefreshCw } from "lucide-react";
import { native, nativeAvailable } from "./desktop";
import { Modal } from "./views";

const activePhases = new Set(["checking", "downloading", "downloaded", "verifying", "installing"]);
export function useAppUpdates() {
  const [state, setState] = useState({ phase: "idle", message: "" });
  const current = useRef(state);
  current.current = state;
  const available = nativeAvailable();
  const run = async (operation) => {
    if (!available) return;
    try {
      const next = await native("appUpdate", { operation });
      if (next?.phase) setState(next);
    } catch (error) {
      setState((previous) => ({ ...previous, phase: "failed", message: error.message, retryable: true }));
    }
  };
  useEffect(() => {
    if (!available) return;
    let live = true;
    const receive = (event) => {
      if (event.detail?.phase) setState(event.detail);
    };
    window.addEventListener("cage-app-update", receive);
    (async () => {
      try {
        let next = await native("appUpdate", { operation: "status" });
        if (!live) return;
        if (!next || next.phase === "idle") next = await native("appUpdate", { operation: "check" });
        if (live && next?.phase) setState(next);
      } catch (error) {
        if (live) setState({ phase: "failed", message: error.message, retryable: true });
      }
    })();
    const timer = setInterval(() => {
      if (["idle", "current", "failed"].includes(current.current.phase)) void run("check");
    }, 6 * 60 * 60 * 1000);
    return () => {
      live = false;
      clearInterval(timer);
      window.removeEventListener("cage-app-update", receive);
    };
  }, [available]);
  return { state, run, available };
}

export function AppUpdateButton({ updates }) {
  const [details, setDetails] = useState(false);
  const { state, run, available } = updates;
  if (!available) return null;
  const busy = activePhases.has(state.phase);
  const downloadable = ["available", "ready"].includes(state.phase);
  const percent = state.totalBytes > 0
    ? Math.min(100, Math.floor((state.receivedBytes || 0) / state.totalBytes * 100))
    : null;
  const label = state.phase === "available" ? "下载更新"
    : state.phase === "ready" ? "安装更新并重启"
    : state.phase === "downloading" ? `正在下载更新${percent === null ? "" : ` ${percent}%`}`
    : state.phase === "verifying" || state.phase === "downloaded" ? "正在校验更新"
    : state.phase === "installing" ? "正在安装更新"
    : state.phase === "checking" ? "正在检查更新"
    : state.phase === "failed" ? "查看更新状态"
    : "检查更新";
  const Icon = busy ? LoaderCircle : downloadable ? ArrowDownToLine : state.phase === "current" ? Check : RefreshCw;
  return (
    <>
      <button
        className={`icon-button app-update-button ${downloadable ? "available" : ""}`}
        aria-label={label}
        title={state.phase === "failed" ? state.message || label : label}
        onClick={() => {
          setDetails(true);
          if (downloadable) void run("install");
          else if (["idle", "current"].includes(state.phase)) void run("check");
        }}
      >
        <Icon size={17} className={busy ? "spin" : ""} />
        {state.phase === "available" && <i className="update-available-dot" />}
      </button>
      {details && (
        <Modal title="Cage 更新" onClose={() => setDetails(false)}>
          <div className="app-update-details">
            <p>{state.message || "检查是否有可安装的新版本。"}</p>
            <dl>
              {state.currentVersion && <><dt>当前版本</dt><dd>{state.currentVersion}</dd></>}
              {state.latestVersion && <><dt>可用版本</dt><dd>{state.latestVersion}</dd></>}
            </dl>
            {state.phase === "downloading" && (
              <>
                <progress aria-label="更新下载进度" value={percent ?? undefined} max="100" />
                <p>{percent === null ? "正在下载…" : `已下载 ${percent}%`}</p>
              </>
            )}
            {state.phase === "ready" && <p>更新已下载。保存工作后可继续安装，项目与登录状态会保留。</p>}
            <div className="modal-actions">
              {state.phase === "downloading" && <button onClick={() => run("cancel")}>取消下载</button>}
              {!busy && (
                <button onClick={() => run("check")}>重新检查</button>
              )}
              {!busy && (downloadable || (state.phase === "failed" && state.latestVersion)) && (
                <button className="primary" onClick={() => run("install")}>
                  {state.phase === "ready" ? "安装并重启" : state.phase === "failed" ? "重试更新" : "下载更新"}
                </button>
              )}
              <button onClick={() => setDetails(false)}>关闭</button>
            </div>
          </div>
        </Modal>
      )}
    </>
  );
}
