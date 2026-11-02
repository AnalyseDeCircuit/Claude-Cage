import { controlSocketURL } from "./api.js";

// Reconnecting is explicit: a fresh view replays retained server output into a
// fresh terminal. Automatically appending that history would duplicate it.
export function connectTerminal(id, { onOpen, onData, onClose }) {
  let closed = false;
  const socket = new WebSocket(
    controlSocketURL(`/terminals/${encodeURIComponent(id)}/stream`),
  );
  socket.binaryType = "arraybuffer";
  socket.onopen = () => {
    if (!closed) onOpen();
  };
  socket.onmessage = (event) => {
    if (!closed) onData(new Uint8Array(event.data));
  };
  socket.onclose = () => {
    if (!closed) onClose();
  };
  const send = (message) => {
    if (!closed && socket.readyState === WebSocket.OPEN)
      socket.send(JSON.stringify(message));
  };
  return {
    write: (data) => send({ type: "input", data }),
    resize: (cols, rows) => send({ type: "resize", cols, rows }),
    close() {
      closed = true;
      socket.onopen = socket.onmessage = socket.onclose = null;
      socket.close();
    },
  };
}
