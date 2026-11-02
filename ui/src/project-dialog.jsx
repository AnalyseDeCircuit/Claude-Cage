import React, { useEffect, useState } from "react";
import { FolderOpen, LoaderCircle } from "lucide-react";
import { api } from "./api";
import { native, nativeAvailable } from "./desktop";
import { Modal } from "./views";

export function ProjectDialog({
  current,
  status,
  initialPath = "",
  onClose,
  onOpened,
}) {
  const [path, setPath] = useState(initialPath);
  const [error, setError] = useState("");
  const [job, setJob] = useState(null);
  const [submitting, setSubmitting] = useState(false);
  const busy = submitting || Boolean(job) || Boolean(status.job?.running);
  useEffect(() => {
    if (!job || status.job?.id !== job || status.job.running) return;
    setJob(null);
    if (status.job.error) setError(status.job.error);
    else onOpened(path.trim());
  }, [status.job, job]);
  const choose = async () => {
    try {
      const selected = await native("chooseFolder");
      if (selected) {
        setPath(selected);
        setError("");
      }
    } catch {
      setError("未能选择文件夹，请直接填写路径。");
    }
  };
  const open = async (event) => {
    event.preventDefault();
    if (busy || !path.trim()) return;
    if (path.trim() === current.workspace) {
      onOpened(path.trim());
      return;
    }
    setSubmitting(true);
    setError("");
    try {
      const result = await api("/settings", {
        ...current,
        workspace: path.trim(),
      });
      setJob(result.id);
    } catch (e) {
      setError(e.message);
    } finally {
      setSubmitting(false);
    }
  };
  return (
    <Modal
      title="打开项目"
      onClose={busy ? undefined : onClose}
      footer={
        <>
          <span className="save-note">
            {busy
              ? status.job?.message || "正在打开项目…"
              : "选择本地文件夹作为项目"}
          </span>
          <button disabled={busy} onClick={onClose}>
            取消
          </button>
          <button
            className="primary"
            form="open-project"
            type="submit"
            disabled={!path.trim() || busy}
          >
            {busy ? (
              <LoaderCircle className="spin" size={15} />
            ) : (
              <FolderOpen size={15} />
            )}
            打开项目
          </button>
        </>
      }
    >
      <form
        id="open-project"
        className="modal-body project-form"
        onSubmit={open}
      >
        <p>选择的目录会加入左侧项目列表，可在目录下新建终端。</p>
        <label htmlFor="project-path">项目文件夹</label>
        <div className="folder-input">
          <input
            id="project-path"
            autoFocus
            value={path}
            onChange={(event) => {
              setPath(event.target.value);
              setError("");
            }}
            placeholder="/Users/你的名字/Projects/my-project"
            disabled={busy}
            spellCheck="false"
            autoComplete="off"
          />
          {nativeAvailable() && (
            <button type="button" onClick={choose} disabled={busy}>
              选择文件夹…
            </button>
          )}
        </div>
        <p className="field-help">
          这个文件夹中的改动会同步到本机。所有项目共用 Cage
          环境，已有终端和网页会保持运行。
        </p>
        {error && (
          <div className="error-message" role="alert">
            {error}
          </div>
        )}
      </form>
    </Modal>
  );
}
