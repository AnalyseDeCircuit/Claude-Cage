import logo from "./logo.png";
import React, { useEffect, useRef, useState } from "react";
import { createRoot } from "react-dom/client";
import {
  ArrowLeft,
  ArrowRight,
  ArrowUpRight,
  Copy,
  FileCode2,
  FolderOpen,
  Globe2,
  LoaderCircle,
  MoreHorizontal,
  Plus,
  PanelLeftOpen,
  ChevronRight,
  Power,
  RefreshCw,
  Shield,
  TerminalSquare,
  X,
} from "lucide-react";
import { api, normalizeURL } from "./api";
import { native, nativeAvailable } from "./desktop";
import { RemoteView, TerminalView, Modal } from "./views";
import { SettingsPanel } from "./settings";
import { AppearanceProvider } from "./appearance";
import { EnvironmentShield } from "./environment-shield";
import { WorkspaceSidebar } from "./workspace-sidebar";
import { WorkspaceTabs } from "./workspace-tabs";
import { BrowserFavorites } from "./browser-favorites";
import { BrowserDownloads, useDownloadNotifications } from "./browser-downloads";
import { ProjectDialog } from "./project-dialog";
import {
  readLocal,
  orderedTabs,
  projectName,
  tabName,
} from "./workspace-state.mjs";
import { SafetyPanel } from "./safety";
import { networkState, isGuestLocalURL } from "./safety-state.mjs";
import { needsProxySetup } from "./onboarding-state.mjs";
import { HostGitPreparation } from "./host-git";
import { NetworkLatency } from "./network-latency";
import { useWindowVisibility } from "./window-visibility";
import { AppUpdateButton, useAppUpdates } from "./app-updates";
import "./style.css";
import "./appearance.css";
import { stopAndExit } from "./quit";
import { useWorkbench } from "./use-workbench";

function App() {
  const appUpdates = useAppUpdates();
  const [recoveringEditor, setRecoveringEditor] = useState(false);
  const windowVisible = useWindowVisibility();
  const [tabs, setTabs] = useState([]),
    [active, setActive] = useState(localStorage.getItem("cage-active") || ""),
    [address, setAddress] = useState(""),
    [settingsOpen, setSettingsOpen] = useState(false),
    [hostGitOpen, setHostGitOpen] = useState(false),
    [settingsSection, setSettingsSection] = useState("keyboard"),
    [home, setHome] = useState(true),
    [sidebarCollapsed, setSidebarCollapsed] = useState(
      readLocal("cage-sidebar-collapsed", false),
    ),
    [selectedProject, setSelectedProject] = useState(""),
    [savedTabs, setSavedTabs] = useState(readLocal("cage-tab-layout", {})),
    [projectDialog, setProjectDialog] = useState(null),
    [rename, setRename] = useState(null),
    [toast, setToast] = useState(""),
    [dialog, setDialog] = useState(null),
    [transfer, setTransfer] = useState(null),
    [details, setDetails] = useState(false),
    [safetyOpen, setSafetyOpen] = useState(false),
    [quitting, setQuitting] = useState(null);
  const terminalControls = useRef(new Map());
  const remote = useRef(),
    addressField = useRef(),
    activeRef = useRef(active),
    lastBrowserTab = useRef(""),
    browserIds = useRef(new Set()),
    initialized = useRef(false),
    launchAcknowledged = useRef(false),
    quitAfterStop = useRef(null);
  activeRef.current = active;
  const {
    status,
    setStatus,
    settings,
    proxy,
    browser,
    connectionError,
    fatal,
    refreshSettings,
  } = useWorkbench({
    visible: windowVisible,
    onNotice: (message) => setToast(message),
    onSnapshot({ browser: b, terminals }) {
      const all = [
        ...b.tabs,
        ...terminals.map((item) => ({
          ...item,
          id: "terminal:" + item.id,
          sessionId: item.id,
          kind: "terminal",
        })),
      ];
      setTabs((previous) => {
        const map = new Map(all.map((item) => [item.id, item]));
        return [
          ...previous
            .filter((item) => map.has(item.id))
            .map((item) => map.get(item.id)),
          ...all.filter((item) => !previous.some((p) => p.id === item.id)),
        ];
      });
      if (
        !activeRef.current ||
        !all.some((item) => item.id === activeRef.current) ||
        (initialized.current && b.active && !browserIds.current.has(b.active))
      )
        setActive(b.active || all[0]?.id || "");
      browserIds.current = new Set(b.tabs.map((item) => item.id));
      initialized.current = true;
    },
    onBrowser(b) {
      setTabs((previous) => {
        const map = new Map(b.tabs.map((item) => [item.id, item]));
        return [
          ...previous
            .filter((item) => item.kind === "terminal" || map.has(item.id))
            .map((item) => map.get(item.id) || item),
          ...b.tabs.filter((item) => !previous.some((p) => p.id === item.id)),
        ];
      });
      if (
        b.active &&
        (b.focus ||
          !activeRef.current ||
          !activeRef.current.startsWith("terminal:"))
      )
        setActive(b.active);
      browserIds.current = new Set(b.tabs.map((item) => item.id));
    },
  });
  const current = tabs.find((t) => t.id === active),
    terminal = current?.kind === "terminal",
    busy = Boolean(status?.job?.running),
    showHome = home || !status?.vm_running || !current,
    showRemote = status?.ready && !terminal && !showHome;
  const running = Boolean(status?.vm_running);
  const projects = status?.projects || [];
  const workspace = selectedProject || status?.workspace;
  const contentWorkspace =
    terminal && !showHome ? current.workspace : workspace;
  const setupProxy = needsProxySetup(settings, status, proxy);
  const network = connectionError
    ? { mode: "unknown", label: "外网状态待确认" }
    : networkState(status);
  const runtimeLabel = connectionError
    ? "连接中断"
    : busy
      ? network.mode === "checking" || status?.job?.kind === "safety"
        ? "检查中"
        : status?.job?.kind === "stop"
          ? "停止中"
          : running
            ? "应用中"
            : "启动中"
      : status?.ready
        ? network.mode === "open"
          ? "运行中"
          : "离线可用"
        : running
          ? status?.environment_update_required ? "环境待更新" : "浏览器待恢复"
          : "已停止";
  const notify = (text) => setToast(text);
  useDownloadNotifications(browser.downloads, notify);
  const confirm = (options) =>
    new Promise((resolve) => setDialog({ ...options, resolve }));
  const openSettings = (section = "keyboard") => {
    setSafetyOpen(false);
    setSettingsSection(section);
    setSettingsOpen(true);
  };
  const select = async (id) => {
    setHome(false);
    setActive(id);
    localStorage.setItem("cage-active", id);
    const tab = tabs.find((t) => t.id === id);
    if (tab?.kind === "terminal") setSelectedProject(tab.workspace);
    if (tab && tab.kind !== "terminal") {
      try {
        await api("/browser/activate", { id });
        remote.current?.focus();
      } catch (e) {
        notify(e.message);
      }
    }
  };
  const action = async (path, body = {}) => {
    try {
      return await api(path, body);
    } catch (e) {
      notify(e.message);
      return null;
    }
  };
  const openBrowserPage = async (
    url = settings?.browser_home_url || "https://www.bing.com/",
  ) => {
    const result = await action("/browser/new", { url });
    if (result) {
      setHome(false);
      setTabs((list) =>
        list.some((t) => t.id === result.id)
          ? list
          : [
              ...list,
              {
                id: result.id,
                kind: "web",
                url,
                title: "新标签页",
              },
            ],
      );
      setActive(result.id);
    }
    return result;
  };
  const newPage = () => openBrowserPage();
  const openBrowser = () => {
    const tab =
      tabs.find(
        (t) => t.id === lastBrowserTab.current && t.kind !== "terminal",
      ) ||
      tabs.find((t) => t.id === browser.active && t.kind !== "terminal") ||
      tabs.find((t) => t.kind !== "terminal");
    return tab ? select(tab.id) : newPage();
  };
  const newTerminal = async (path = workspace) => {
    setHome(false);
    const result = await action("/terminals", { workspace: path });
    if (result) {
      setSelectedProject(result.workspace);
      const tab = {
        ...result,
        id: "terminal:" + result.id,
        sessionId: result.id,
        kind: "terminal",
      };
      setTabs((list) => [...list.filter((t) => t.id !== tab.id), tab]);
      setActive(tab.id);
    }
  };
  const newTerminalHere = () => newTerminal(contentWorkspace);
  const openEditor = async (path = workspace) => {
    setHome(false);
    const result = await action("/browser/editor", { workspace: path });
    if (result) {
      setTabs((list) =>
        list.some((t) => t.id === result.id)
          ? list
          : [
              ...list,
              {
                id: result.id,
                kind: "editor",
                workspace: path,
                url: `http://127.0.0.1:18080/?folder=${encodeURIComponent(path)}`,
                title: "文件编辑器",
              },
            ],
      );
      setActive(result.id);
    }
  };
  const closeTab = async (tab) => {
    if (tab.kind === "terminal") {
      if (
        !tab.closed &&
        !(await confirm({
          title: "关闭这个终端会话？",
          body: "该终端中的交互命令可能被中断。网页和其他终端不受影响。",
          confirm: "关闭终端",
        }))
      )
        return;
      try {
        await api("/terminals/" + tab.sessionId, undefined, "DELETE");
      } catch (e) {
        notify(e.message);
        return;
      }
    } else {
      if (
        tab.kind === "editor" &&
        !(await confirm({
          title: "关闭文件编辑器？",
          body: "请先保存未完成的修改。文件仍保留在共享项目中。",
          confirm: "关闭编辑器",
        }))
      )
        return;
      if (!(await action("/browser/close", { id: tab.id }))) return;
    }
    setTabs((list) => list.filter((t) => t.id !== tab.id));
    if (active === tab.id) {
      const group = displayTabs.filter((item) =>
        tab.kind === "terminal"
          ? item.kind === "terminal" && item.workspace === tab.workspace
          : item.kind !== "terminal",
      );
      const at = group.findIndex((item) => item.id === tab.id);
      const next = group[at + 1] || group[at - 1];
      if (next) await select(next.id);
      else setActive("");
    }
  };
  const navigate = async (e) => {
    e?.preventDefault();
    try {
      const url = normalizeURL(address);
      if (network.mode !== "open" && !isGuestLocalURL(url)) {
        setSafetyOpen(true);
        notify("请先完成关键检查，再访问外部网站。");
        return;
      }
      if (current && current.kind !== "terminal")
        await action("/browser/navigate", { id: current.id, url });
      else {
        const result = await action("/browser/new", { url });
        if (result) setActive(result.id);
      }
      remote.current?.focus();
    } catch (error) {
      notify(error.message);
    }
  };
  const stop = async () => {
    if (
      await confirm({
        title: "停止当前环境？",
        body: "请先保存文件。当前终端会话会结束，浏览器登录和项目文件会保留。",
        confirm: "停止环境",
      })
    )
      await action("/stop");
  };
  const cancelAutomaticExit = () => {
    quitAfterStop.current = null;
    setQuitting(null);
  };
  const stopAndQuit = async () => {
    const request = { id: null };
    quitAfterStop.current = request;
    setQuitting({ error: "" });
    try {
      await stopAndExit({
        stop: () => api("/stop", {}),
        readStatus: () => api("/status"),
        exit: () => native("quitNow"),
        isCancelled: () => quitAfterStop.current !== request,
        onStatus: setStatus,
      });
    } catch (error) {
      if (quitAfterStop.current === request) {
        quitAfterStop.current = null;
        setQuitting({ error: error.message });
      }
    }
  };
  useEffect(() => {
    if (!settings || !status || fatal || connectionError || launchAcknowledged.current || !nativeAvailable()) return;
    // Effects run after the workbench render commits. HTML navigation alone
    // cannot confirm a successful upgrade when session/API startup failed.
    let live = true;
    native("workbenchReady").then((accepted) => {
      if (live && accepted === true) launchAcknowledged.current = true;
    }).catch(() => {});
    return () => { live = false; };
  }, [settings, status, fatal, connectionError]);
  useEffect(() => {
    if (current && current.kind !== "terminal")
      lastBrowserTab.current = current.id;
    if (
      current?.kind !== "terminal" &&
      document.activeElement !== addressField.current
    )
      setAddress(current?.url === "about:blank" ? "" : current?.url || "");
    document.title =
      (current?.title && current.title !== "新标签页"
        ? current.title + " · "
        : "") + "Cage";
    if (active) localStorage.setItem("cage-active", active);
  }, [current?.url, current?.title, active]);
  useEffect(() => {
    if (!toast) return;
    const timer = setTimeout(() => setToast(""), 4500);
    return () => clearTimeout(timer);
  }, [toast]);
  useEffect(() => {
    const key = (event) => {
      if (!event.metaKey) return;
      const k = event.key.toLowerCase();
      if (document.querySelector("[data-browser-dialog]")) {
        if (k === "w") {
          event.preventDefault();
          event.stopImmediatePropagation();
          document.dispatchEvent(new Event("cage-close-modal"));
        }
        return;
      }
      if (dialog) {
        if (k === "w") {
          event.preventDefault();
          event.stopImmediatePropagation();
          const resolve = dialog.resolve;
          setDialog(null);
          resolve(false);
        }
        return;
      }
      if (k === "," && !projectDialog && !rename && !hostGitOpen) {
        event.preventDefault();
        event.stopImmediatePropagation();
        openSettings();
        return;
      }
      if (
        settingsOpen ||
        hostGitOpen ||
        transfer ||
        details ||
        safetyOpen ||
        projectDialog ||
        rename
      ) {
        if (k === "w") {
          event.preventDefault();
          event.stopImmediatePropagation();
          document.dispatchEvent(new Event("cage-close-modal"));
        }
        return;
      }
      let handle;
      if (k === "o") {
        event.preventDefault();
        event.stopImmediatePropagation();
        if (event.shiftKey) {
          document.dispatchEvent(new Event("cage-show-favorites"));
          return;
        }
        if (!busy && running) setProjectDialog({ path: "" });
        return;
      }
      if (k === "t" && !event.shiftKey) handle = newPage;
      if (k === "d") handle = () => document.dispatchEvent(new Event("cage-add-favorite"));
      if (k === "enter" && event.shiftKey) handle = newTerminalHere;
      if (k === "l")
        handle = () => {
          addressField.current?.focus();
          addressField.current?.select();
        };
      if (k === "w" && current) handle = () => closeTab(current);
      if (/^[1-9]$/.test(k)) {
        const tab = k === "9" ? switchTabs.at(-1) : switchTabs[Number(k) - 1];
        if (tab) handle = () => select(tab.id);
      }
      if (event.shiftKey && ["[", "]", "{", "}"].includes(k)) {
        const direction = ["[", "{"].includes(k) ? -1 : 1,
          at = switchTabs.findIndex((t) => t.id === active),
          tab =
            switchTabs[
              (at + direction + switchTabs.length) % switchTabs.length
            ];
        if (tab) handle = () => select(tab.id);
      }
      if (handle) {
        event.preventDefault();
        event.stopImmediatePropagation();
        if (
          !busy &&
          (status?.ready ||
            (running && k === "enter") ||
            (k === "w" && terminal))
        )
          void handle();
      }
    };
    window.addEventListener("keydown", key, true);
    window.cageEditAction = (command) => {
      if (
        settingsOpen ||
        transfer ||
        dialog ||
        details ||
        safetyOpen ||
        projectDialog ||
        rename ||
        showHome
      )
        return false;
      if (terminal)
        return terminalControls.current.get(current.id)?.edit(command) || false;
      return remote.current?.edit(command) || false;
    };
    window.cageNativeAction = async (command) => {
      if (hostGitOpen && command !== "quit") {
        if (command === "closeTab")
          document.dispatchEvent(new Event("cage-close-modal"));
        return;
      }
      if (command === "settings") {
        if (!projectDialog && !rename && !dialog) openSettings();
        return;
      }
      if (command === "openProject") {
        if (
          !busy &&
          running &&
          !settingsOpen &&
          !dialog &&
          !projectDialog &&
          !rename
        )
          setProjectDialog({ path: "" });
        return;
      }
      if (
        (settingsOpen || dialog || projectDialog || rename) &&
        ["newTab", "newTerminal"].includes(command)
      )
        return;
      if (command === "newTab" && status?.ready) await newPage();
      if (command === "newTerminal" && status?.vm_running)
        await newTerminalHere();
      if (command === "closeTab") {
        if (document.querySelector("[data-browser-dialog]")) {
          document.dispatchEvent(new Event("cage-close-modal"));
          return;
        }
        if (dialog) {
          const resolve = dialog.resolve;
          setDialog(null);
          resolve(false);
        } else if (
          settingsOpen ||
          transfer ||
          details ||
          safetyOpen ||
          projectDialog ||
          rename
        )
          document.dispatchEvent(new Event("cage-close-modal"));
        else if (current) await closeTab(current);
      }
      if (command === "quit") {
        if (quitAfterStop.current) return;
        if (status && !connectionError && !status.vm_running && !busy) {
          await native("quitNow");
          return;
        }
        setDialog({
          title: "退出 Cage",
          body: "退出前请保存文件和设置。可以保留环境，让后台任务继续运行；下次打开会重新连接。",
          confirm: "停止环境并退出",
          secondary: "保留运行并退出",
          resolve: async (result) => {
            if (result === "secondary") {
              await native("quitNow");
            } else if (result) {
              await stopAndQuit();
            }
          },
        });
      }
    };
    return () => {
      window.removeEventListener("keydown", key, true);
      delete window.cageNativeAction;
      delete window.cageEditAction;
    };
  }, [
    status,
    settingsOpen,
    hostGitOpen,
    safetyOpen,
    details,
    transfer,
    dialog,
    current,
    tabs,
    busy,
    active,
    connectionError,
    projectDialog,
    rename,
    showHome,
    savedTabs,
    selectedProject,
  ]);
  const project = projectName(contentWorkspace);
  const tabLayoutKey = status?.environment_directory || status?.workspace;
  const metadata = savedTabs[tabLayoutKey] || {};
  const displayTabs = orderedTabs(tabs, metadata);
  const browserTabs = displayTabs.filter((tab) => tab.kind !== "terminal");
  const switchTabs =
    terminal && !showHome
      ? displayTabs.filter(
          (tab) =>
            tab.kind === "terminal" && tab.workspace === current.workspace,
        )
      : browserTabs;
  const renameTab = (tab) =>
    setRename({ id: tab.id, title: tabName(tab, metadata, project) });
  const pinTab = (id) =>
    changeTabMetadata({
      ...metadata,
      [id]: { ...metadata[id], pinned: !metadata[id]?.pinned },
    });
  const persist = (key, value, setter) => {
    try {
      localStorage.setItem(key, JSON.stringify(value));
      setter(value);
    } catch {
      notify("未能保存界面偏好，请检查本地存储空间。");
    }
  };
  const changeTabMetadata = (next) =>
    persist(
      "cage-tab-layout",
      { ...savedTabs, [tabLayoutKey]: next },
      setSavedTabs,
    );
  const reorder = (from, to, scopeOrder) => {
    if (scopeOrder) {
      const ids = new Set(scopeOrder);
      let index = 0;
      const order = displayTabs.map((tab) => ids.has(tab.id) ? scopeOrder[index++] : tab.id);
      changeTabMetadata({ ...metadata, order });
      return;
    }
    if (from === to) return;
    const order = displayTabs.map((tab) => tab.id).filter((id) => id !== from);
    order.splice(order.indexOf(to), 0, from);
    changeTabMetadata({ ...metadata, order });
  };
  const openedProject = async (path) => {
    await refreshSettings();
    setSelectedProject(path);
    setProjectDialog(null);
    setHome(true);
  };
  useEffect(() => {
    if (status?.workspace) setSelectedProject(status.workspace);
  }, [status?.workspace]);
  const finishDialog = (value) => {
    const resolve = dialog.resolve;
    setDialog(null);
    resolve(value);
  };
  const dialogs = (
    <>
      {hostGitOpen && (
        <HostGitPreparation
          onClose={() => setHostGitOpen(false)}
          onReady={() => {
            setHostGitOpen(false);
            action("/start");
          }}
        />
      )}
      {dialog && (
        <Modal
          title={dialog.title}
          onClose={() => finishDialog(false)}
          footer={
            <>
              <button onClick={() => finishDialog(false)}>取消</button>
              {dialog.secondary && (
                <button onClick={() => finishDialog("secondary")}>
                  {dialog.secondary}
                </button>
              )}
              <button className="primary" onClick={() => finishDialog(true)}>
                {dialog.confirm}
              </button>
            </>
          }
        >
          <div className="modal-body">
            <p>{dialog.body}</p>
          </div>
        </Modal>
      )}
      {quitting && (
        <Modal
          title={quitting.error ? "未能完成退出" : "正在停止环境"}
          onClose={cancelAutomaticExit}
          footer={
            <>
              <button onClick={cancelAutomaticExit}>
                {quitting.error ? "留在 Cage" : "取消自动退出"}
              </button>
              {quitting.error && (
                <button className="primary" onClick={stopAndQuit}>
                  重试停止并退出
                </button>
              )}
            </>
          }
        >
          <div
            className="modal-body"
            role={quitting.error ? "alert" : "status"}
          >
            <p>
              {quitting.error ||
                (connectionError
                  ? "正在恢复工作台连接，恢复后会继续确认停止结果。"
                  : "停止完成后 Cage 会自动退出。取消自动退出只保留窗口，环境仍会停止。")}
            </p>
            {!quitting.error && (
              <p>
                <LoaderCircle size={16} className="spin" />{" "}
                {status?.job?.running
                  ? status.job.message
                  : "正在确认环境已停止…"}
              </p>
            )}
          </div>
        </Modal>
      )}
    </>
  );
  if (fatal)
    return (
      <>
        <div className="boot-screen">
          <Shield size={36} />
          <h1>连接 Cage 工作台</h1>
          <p>{fatal}</p>
          <p className="muted">
            请从 Cage 应用打开，或在终端运行 <code>cage desktop</code>。
          </p>
          <button
            className="primary"
            onClick={() =>
              nativeAvailable() ? native("reconnect") : location.reload()
            }
          >
            重新连接
          </button>
        </div>
        {dialogs}
      </>
    );
  if (!status || !settings)
    return (
      <>
        <div className="boot-screen">
          <div className="brand-mark">C</div>
          <h1>Cage</h1>
          <p>
            <LoaderCircle size={16} className="spin" />
            正在连接工作区…
          </p>
        </div>
        {dialogs}
      </>
    );
  return (
    <div className={`app ${sidebarCollapsed ? "sidebar-collapsed" : ""}`}>
      <div
        className="workbench-shell"
        inert={settingsOpen || hostGitOpen || undefined}
      >
        {!sidebarCollapsed && (
          <WorkspaceSidebar
            updateAction={<AppUpdateButton updates={appUpdates} />}
            workspace={selectedProject || status.workspace}
            defaultWorkspace={status.environment_directory}
            projects={projects}
            tabs={displayTabs}
            saved={metadata}
            active={active}
            home={showHome}
            busy={busy}
            ready={status.ready}
            running={running}
            runtimeLabel={runtimeLabel}
            network={network}
            onHome={() => setHome(true)}
            onOpenProject={(path = "") => setProjectDialog({ path })}
            onSelect={select}
            onClose={closeTab}
            onRename={renameTab}
            onPin={pinTab}
            onReorder={reorder}
            onProjectSelect={setSelectedProject}
            onNewTerminal={newTerminal}
            onBrowser={openBrowser}
            onEditor={openEditor}
            onSettings={() => openSettings()}
            onSafety={() => setSafetyOpen(true)}
            onCollapse={() =>
              persist("cage-sidebar-collapsed", true, setSidebarCollapsed)
            }
          />
        )}
        <div className="workbench-main">
          <header className="chrome">
            {!showHome && !terminal ? (
              <div className="browser-tab-bar">
                {sidebarCollapsed && (
                  <button
                    className="icon-button sidebar-expand"
                    aria-label="展开侧栏"
                    onClick={() =>
                      persist(
                        "cage-sidebar-collapsed",
                        false,
                        setSidebarCollapsed,
                      )
                    }
                  >
                    <img className="sidebar-expand-logo" src={logo} alt="" />
                    <PanelLeftOpen className="sidebar-expand-icon" size={18} />
                  </button>
                )}
                <WorkspaceTabs
                  tabs={browserTabs}
                  saved={metadata}
                  active={active}
                  label="浏览器标签页"
                  horizontal
                  busy={busy}
                  ready={status.ready}
                  onSelect={select}
                  onClose={closeTab}
                  onRename={renameTab}
                  onPin={pinTab}
                  onReorder={reorder}
                />
                <button
                  className="icon-button browser-new-tab"
                  aria-label="新建网页"
                  title="新建网页 ⌘T"
                  disabled={!status.ready || busy}
                  onClick={newPage}
                >
                  <Plus size={17} />
                </button>
              </div>
            ) : (
              <div className="content-heading">
                {sidebarCollapsed && (
                  <button
                    className="icon-button sidebar-expand"
                    aria-label="展开侧栏"
                    title="展开侧栏"
                    onClick={() =>
                      persist(
                        "cage-sidebar-collapsed",
                        false,
                        setSidebarCollapsed,
                      )
                    }
                  >
                    <img className="sidebar-expand-logo" src={logo} alt="" />
                    <PanelLeftOpen className="sidebar-expand-icon" size={18} />
                  </button>
                )}
                <div className="content-breadcrumb">
                  <button
                    onClick={() => setHome(true)}
                    title={contentWorkspace}
                  >
                    <FolderOpen size={15} />
                    <span>{project}</span>
                  </button>
                  {!showHome && current && (
                    <>
                      <ChevronRight size={13} />
                      <span className="current-title">
                        {tabName(current, metadata, project)}
                      </span>
                    </>
                  )}
                </div>
                <div className="heading-actions">
                  <button
                    className="quiet-button"
                    onClick={() => openEditor(contentWorkspace)}
                    disabled={!status.ready || busy}
                  >
                    <FileCode2 size={15} />
                    <span>打开文件</span>
                  </button>
                  <button
                    className="icon-button"
                    aria-label="新建终端"
                    title="新建终端 ⌘⇧↵"
                    onClick={newTerminalHere}
                    disabled={!running || busy}
                  >
                    <TerminalSquare size={17} />
                  </button>
                  {current && !showHome && (
                    <button
                      className="icon-button"
                      aria-label="关闭当前标签"
                      title="关闭当前标签 ⌘W"
                      onClick={() => closeTab(current)}
                      disabled={busy}
                    >
                      <X size={16} />
                    </button>
                  )}
                </div>
              </div>
            )}
            {!showHome && (
              <div className="navigation-row">
                {!terminal ? (
                  <>
                    <div className="nav-actions">
                      <button
                        className="icon-button"
                        aria-label="后退"
                        disabled={!status.ready || !browser.back}
                        onClick={() =>
                          action("/browser/back", { id: current?.id })
                        }
                      >
                        <ArrowLeft size={17} />
                      </button>
                      <button
                        className="icon-button"
                        aria-label="前进"
                        disabled={!status.ready || !browser.forward}
                        onClick={() =>
                          action("/browser/forward", { id: current?.id })
                        }
                      >
                        <ArrowRight size={17} />
                      </button>
                      <button
                        className="icon-button"
                        aria-label="重新加载网页"
                        disabled={!showRemote}
                        onClick={() =>
                          action("/browser/reload", { id: current?.id })
                        }
                      >
                        <RefreshCw size={15} />
                      </button>
                    </div>
                    <form className="address-bar" onSubmit={navigate}>
                      <Globe2 size={13} />
                      <input
                        ref={addressField}
                        aria-label="网页地址或搜索"
                        value={address}
                        onChange={(e) => setAddress(e.target.value)}
                        onFocus={(e) => e.target.select()}
                        placeholder={
                          status.ready
                            ? "输入网址或搜索，在 Cage 内打开"
                            : "启动环境后浏览网页"
                        }
                        disabled={!status.ready}
                        spellCheck="false"
                      />
                      <button
                        className="address-go"
                        type="submit"
                        aria-label="打开地址"
                        disabled={!status.ready}
                      >
                        <ArrowUpRight size={15} />
                      </button>
                    </form>
                    <BrowserFavorites current={current} ready={status.ready && !busy} onOpen={openBrowserPage} />
                    <BrowserDownloads downloads={browser.downloads} ready={status.ready && !busy} onStatus={notify} />
                    {current?.kind === "editor" && <button className="icon-button" aria-label="恢复编辑器" title="单独重启编辑器" disabled={!status.ready || busy || recoveringEditor} onClick={async () => {
                      if (!(await confirm({ title: "重启编辑器？", body: "请先保存文件。浏览器、终端和环境会继续运行。", confirm: "重启编辑器" }))) return;
                      setRecoveringEditor(true);
                      try { await api("/browser/editor-restart", { workspace: current.workspace || contentWorkspace }); notify("编辑器已恢复"); }
                      catch (error) { notify(error.message); }
                      finally { setRecoveringEditor(false); }
                    }}><RefreshCw size={15} className={recoveringEditor ? "spin" : ""} /></button>}
                    <div className="clipboard-actions">
                      <button
                        className="icon-button"
                        aria-label="复制到本机"
                        title="复制到本机"
                        disabled={!showRemote}
                        onClick={() => remote.current?.copy()}
                      >
                        <Copy size={15} />
                      </button>
                      <button
                        className="icon-button"
                        aria-label="粘贴到 Cage"
                        title="粘贴到 Cage"
                        disabled={!showRemote}
                        onClick={() => remote.current?.paste()}
                      >
                        <svg
                          width="15"
                          height="15"
                          viewBox="0 0 24 24"
                          fill="none"
                          stroke="currentColor"
                          strokeWidth="1.7"
                        >
                          <path d="M9 5H5v16h14V5h-4M9 3h6v4H9zM8 12h8M8 16h5" />
                        </svg>
                      </button>
                      <button
                        className="icon-button"
                        aria-label="文本传递"
                        title="文本传递"
                        disabled={!showRemote}
                        onClick={() => remote.current?.transfer()}
                      >
                        <MoreHorizontal size={17} />
                      </button>
                    </div>
                  </>
                ) : (
                  <>
                    <div className="terminal-path">
                      <TerminalSquare size={15} />
                      <span>{current?.workspace || status.workspace}</span>
                      <span className="terminal-kind">Linux · bash</span>
                    </div>
                    <button
                      className="text-button"
                      onClick={newTerminalHere}
                      disabled={busy}
                    >
                      <Plus size={15} />
                      新建终端
                    </button>
                  </>
                )}
                <button
                  className="icon-button files-button"
                  title="打开文件编辑器"
                  aria-label="打开文件编辑器"
                  disabled={!status.ready || busy}
                  onClick={() => openEditor(contentWorkspace)}
                >
                  <FileCode2 size={17} />
                </button>
              </div>
            )}
            {busy && <div className="progress-line" />}
          </header>
          {connectionError && (
            <div className="connection-banner" role="alert">
              工作台连接中断，后台状态暂时未知。
              <button
                onClick={() =>
                  nativeAvailable() ? native("reconnect") : location.reload()
                }
              >
                重新连接工作台
              </button>
            </div>
          )}
          <main className="workspace">
            {status.environment_update_required && !busy && (
              <div className="environment-update-banner" role="status">
                <span>{status.environment_update_error || "环境组件需要更新。请先保存编辑器中的文件。"}</span>
                <button onClick={() => openSettings("project")}>查看并更新</button>
              </div>
            )}
            {(status.vm_running || terminal) && (
              <>
                {status.ready ? (
                  <RemoteView
                    ref={remote}
                    visible={Boolean(showRemote && !settingsOpen)}
                    keyboard={status.keyboard}
                    tabId={current?.id}
                    onStatus={notify}
                    onFallback={(text, message) =>
                      setTransfer({ text, message })
                    }
                  />
                ) : (
                  !terminal &&
                  !showHome && (
                    <div className="view-overlay">
                      <RefreshCw />
                      <h3>浏览器连接暂时不可用</h3>
                      <p>环境和终端仍可运行；恢复浏览器不会停止现有终端。</p>
                      <button
                        className="primary"
                        disabled={busy}
                        onClick={() => setHostGitOpen(true)}
                      >
                        恢复浏览器
                      </button>
                    </div>
                  )
                )}
                {tabs
                  .filter((t) => t.kind === "terminal")
                  .map((tab) => (
                    <TerminalView
                      key={tab.id}
                      session={{ ...tab, id: tab.sessionId }}
                      onControls={(control) =>
                        control
                          ? terminalControls.current.set(tab.id, control)
                          : terminalControls.current.delete(tab.id)
                      }
                      visible={active === tab.id && !showHome && !settingsOpen}
                      onStatus={notify}
                    />
                  ))}
              </>
            )}
            {showHome && (
              <div className="welcome">
                <div className="welcome-content">
                  <EnvironmentShield
                    className="welcome-symbol"
                    healthy={status.ready && network.mode === "open"}
                    size={42}
                  />
                  <h1>
                    {busy
                      ? status.job.kind === "stop"
                        ? "正在停止 Cage 环境"
                        : status.job.kind === "project"
                          ? "正在打开项目"
                          : "正在准备 Cage 环境"
                      : running
                        ? !status.ready
                          ? "Cage 环境正在连接"
                          : status.environment_workspace
                            ? "Cage 环境已就绪"
                            : project
                        : setupProxy
                          ? "为 Claude 准备独立工作区"
                          : "先启动 Cage 环境"}
                  </h1>
                  <p className="welcome-subtitle">
                    {busy
                      ? status.job.message
                      : running
                        ? !status.ready
                          ? "浏览器连接尚未就绪，可以恢复连接或继续使用终端。"
                          : network.mode === "open"
                            ? "环境已连接，网络检查通过。可以浏览网页或打开本地项目。"
                            : `${network.label}。终端与本地文件仍可使用。`
                        : setupProxy
                          ? "先连接你的代理，再启动浏览器、终端和编辑器。无需安装额外的代理软件。"
                          : "准备独立的浏览器和终端，完成网络检查后开始工作。"}
                  </p>
                  {!running && (
                    <ol className="onboarding-steps" aria-label="开始使用 Cage">
                      {["配置代理", "启动环境", "开始工作"].map(
                        (label, index) => (
                          <li
                            key={label}
                            aria-current={
                              index === (setupProxy ? 0 : 1)
                                ? "step"
                                : undefined
                            }
                          >
                            <span>{index + 1}</span>
                            {label}
                          </li>
                        ),
                      )}
                    </ol>
                  )}
                  <div className="start-project-box">
                    <div className="environment-summary">
                      <EnvironmentShield
                        healthy={status.ready && network.mode === "open"}
                        size={24}
                      />
                      <span>
                        <strong>{running ? project : "Cage 隔离环境"}</strong>
                        <small>
                          {running
                            ? runtimeLabel
                            : setupProxy
                              ? "自备代理订阅、YAML 配置或节点"
                              : "代理已配置，首次启动会下载开发环境"}
                        </small>
                      </span>
                    </div>
                    <div className="start-project-footer">
                      <span>
                        <i className={`status-dot ${network.mode}`} />
                        {busy
                          ? runtimeLabel
                          : running
                            ? network.label
                            : setupProxy
                              ? "等待配置代理"
                              : "环境已停止"}
                      </span>
                      <button
                        className="primary"
                        disabled={busy || Boolean(connectionError)}
                        onClick={() => {
                          if (!running && setupProxy) openSettings("network");
                          else if (!running || !status.ready)
                            setHostGitOpen(true);
                          else if (status.environment_workspace)
                            setProjectDialog({ path: "" });
                          else newTerminal();
                        }}
                      >
                        {busy ? (
                          <LoaderCircle size={16} className="spin" />
                        ) : !running && setupProxy ? (
                          <Globe2 size={16} />
                        ) : !running ? (
                          <Power size={16} />
                        ) : !status.ready ? (
                          <RefreshCw size={16} />
                        ) : status.environment_workspace ? (
                          <FolderOpen size={16} />
                        ) : (
                          <TerminalSquare size={16} />
                        )}
                        {busy
                          ? "正在处理…"
                          : !running
                            ? setupProxy
                              ? "配置网络与代理"
                              : "启动 Cage 环境"
                            : !status.ready
                              ? "恢复浏览器"
                              : status.environment_workspace
                                ? "打开项目"
                                : "新建终端"}
                      </button>
                    </div>
                  </div>
                  {running && (
                    <div className="welcome-shortcuts">
                      <button onClick={newTerminalHere} disabled={busy}>
                        <TerminalSquare size={16} />
                        新建终端
                      </button>
                      <button
                        onClick={() => openEditor(contentWorkspace)}
                        disabled={!status.ready || busy}
                      >
                        <FileCode2 size={16} />
                        浏览文件
                      </button>
                    </div>
                  )}
                  {!running && (
                    <p className="welcome-hint">
                      {setupProxy
                        ? "Cage 不提供代理或 Claude 账户。连接后，在 Cage 内完成 Claude 登录。"
                        : "首次启动需要下载 Linux 镜像和开发工具，请保持网络连接。可在操作详情中查看进度，失败后可以重试。"}
                    </p>
                  )}
                  {running && status.ready && (
                    <p className="welcome-hint">
                      打开项目后，在终端输入 <code>claude</code>
                      ，按提示登录你的账户。
                      <br />
                      默认工作区和打开的项目与 Mac 共享，编辑和删除文件会同步。
                    </p>
                  )}
                  {status.project_error && (
                    <div className="start-error" role="alert">
                      {status.project_error}
                    </div>
                  )}
                  {status.job?.error && (
                    <div className="start-error" role="alert">
                      <strong>本次操作未完成</strong>
                      <p>{status.job.error}</p>
                      {status.can_restore && (
                        <button
                          className="text-button"
                          onClick={() => openSettings("project")}
                        >
                          查看恢复选项
                        </button>
                      )}
                    </div>
                  )}
                  {status.job?.log && (
                    <button
                      className="text-button details-link"
                      onClick={() => setDetails(true)}
                    >
                      查看操作详情
                    </button>
                  )}
                </div>
              </div>
            )}
          </main>
          <footer className="status-bar">
            {sidebarCollapsed && <AppUpdateButton updates={appUpdates} />}
            <button onClick={() => setSafetyOpen(true)} title="网络与隔离检查">
              <EnvironmentShield
                size={14}
                healthy={status.ready && network.mode === "open"}
              />
              <span>{running ? network.label : "网络检查"}</span>
            </button>
            <span className="status-divider" />
            <button onClick={() => openSettings("network")}>
              <Globe2 size={12} />
              <span>
                {status.network_error
                  ? "出口待确认"
                  : status.location
                    ? `${status.location.country} · ${status.location.city}`
                    : "指定代理出口"}
              </span>
            </button>
            <span className="status-divider" />
            <button onClick={() => openSettings("region")}>
              {settings.timezone}
              {settings.auto_timezone ? " · 自动" : ""}
            </button>
            <span className="status-divider" />
            <NetworkLatency
              key={`${proxy?.selected}:${proxy?.revision}:${running}:${busy}:${Boolean(connectionError)}`}
              enabled={running && !busy && !connectionError}
              revision={proxy?.revision}
              selected={proxy?.selected}
            />
            <span className="status-spacer" />
            <span className="status-feedback" role="status">
              {toast ||
                (busy
                  ? status.job.message
                  : running
                    ? status.ready
                      ? "文件与本机同步"
                      : "环境运行中 · 浏览器待恢复"
                    : "")}
            </span>
            {status.job?.log && (
              <button title="操作详情" onClick={() => setDetails(true)}>
                <MoreHorizontal size={15} />
              </button>
            )}
            <button
              onClick={stop}
              disabled={
                (!running && !busy) || (status?.job?.kind === "stop" && busy)
              }
              title="停止当前环境"
            >
              <Power size={12} />
              <span>停止环境</span>
            </button>
          </footer>
        </div>
      </div>
      {settingsOpen && (
        <SettingsPanel
          current={settings}
          status={status}
          initialSection={settingsSection}
          onClose={() => setSettingsOpen(false)}
          onSaved={refreshSettings}
          onConfirm={confirm}
        />
      )}
      {projectDialog && (
        <ProjectDialog
          current={settings}
          status={status}
          initialPath={projectDialog.path}
          onClose={() => setProjectDialog(null)}
          onOpened={openedProject}
        />
      )}
      {rename && (
        <Modal
          title="重命名标签"
          onClose={() => setRename(null)}
          footer={
            <>
              <span className="save-note">仅修改工作台中显示的名称</span>
              <button onClick={() => setRename(null)}>取消</button>
              <button
                className="primary"
                form="rename-tab"
                type="submit"
                disabled={!rename.title.trim()}
              >
                保存名称
              </button>
            </>
          }
        >
          <form
            className="modal-body project-form"
            id="rename-tab"
            onSubmit={(event) => {
              event.preventDefault();
              if (!rename.title.trim()) return;
              changeTabMetadata({
                ...metadata,
                [rename.id]: {
                  ...metadata[rename.id],
                  title: rename.title.trim(),
                },
              });
              setRename(null);
            }}
          >
            <label htmlFor="tab-title">标签名称</label>
            <input
              id="tab-title"
              autoFocus
              maxLength={100}
              value={rename.title}
              onChange={(event) =>
                setRename({ ...rename, title: event.target.value })
              }
            />
          </form>
        </Modal>
      )}
      {safetyOpen && (
        <SafetyPanel
          status={
            connectionError
              ? {
                  ...status,
                  safety: {
                    ...status.safety,
                    gate: null,
                    error: "控制服务连接中断，无法确认当前外网状态",
                  },
                }
              : status
          }
          onClose={() => setSafetyOpen(false)}
          onSettings={openSettings}
          onOpenPage={openBrowserPage}
          onConfirm={confirm}
        />
      )}
      {transfer && (
        <Modal
          title="文本传递"
          onClose={() => setTransfer(null)}
          footer={
            <>
              <span className="save-note">仅在你的操作下传递文本</span>
              <button onClick={() => setTransfer(null)}>关闭</button>
              <button
                className="primary"
                onClick={() => {
                  remote.current?.pasteText(transfer.text);
                  setTransfer(null);
                }}
              >
                粘贴到 Cage
              </button>
            </>
          }
        >
          <div className="modal-body">
            <p>{transfer.message}</p>
            <textarea
              aria-label="待传递文本"
              className="transfer-text"
              value={transfer.text}
              onChange={(e) =>
                setTransfer({ ...transfer, text: e.target.value })
              }
            />
          </div>
        </Modal>
      )}
      {details && (
        <Modal title="操作详情" wide onClose={() => setDetails(false)}>
          <div className="modal-body">
            <p>{status.job.message}</p>
            {status.job.error && (
              <div className="error-message">{status.job.error}</div>
            )}
            <pre className="job-log">{status.job.log || "暂无日志"}</pre>
          </div>
        </Modal>
      )}
      {dialogs}
    </div>
  );
}
createRoot(document.getElementById("root")).render(
  <AppearanceProvider>
    <App />
  </AppearanceProvider>,
);

// A launcher can reopen an existing window with a fresh one-use ticket.
window.addEventListener("hashchange", () => {
  if (location.hash) location.reload();
});
