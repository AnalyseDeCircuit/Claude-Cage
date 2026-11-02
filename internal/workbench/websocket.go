package workbench

import (
	"net/http"

	"github.com/gorilla/websocket"
)

func (s *Server) upgrade(w http.ResponseWriter, r *http.Request) (*websocket.Conn, error) {
	up := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return r.Header.Get("Origin") == s.origin }, ReadBufferSize: 16384, WriteBufferSize: 16384}
	return up.Upgrade(w, r, nil)
}
