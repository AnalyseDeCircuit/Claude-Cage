import { useEffect, useRef, useState } from "react";
import { connectWorkbench } from "./workbench-connection.js";

export function useWorkbench({ visible, onSnapshot, onBrowser, onNotice }) {
  const callbacks = useRef();
  callbacks.current = { onSnapshot, onBrowser, onNotice };
  const connection = useRef();
  const [state, setState] = useState({
    status: null,
    settings: null,
    proxy: null,
    browser: { tabs: [] },
    connectionError: "",
    fatal: "",
  });
  useEffect(() => {
    const current = connectWorkbench({
      visible,
      onState: (change) => setState((previous) => ({ ...previous, ...change })),
      onSnapshot: (snapshot) => callbacks.current.onSnapshot(snapshot),
      onBrowser: (snapshot) => callbacks.current.onBrowser(snapshot),
      onNotice: (message) => callbacks.current.onNotice(message),
    });
    connection.current = current;
    return () => {
      connection.current = null;
      current.close();
    };
  }, []);
  useEffect(() => {
    connection.current?.setVisible(visible);
  }, [visible]);
  return {
    ...state,
    setStatus: (status) => connection.current?.updateStatus(status),
    refreshSettings: () => connection.current?.refreshSettings(),
  };
}
