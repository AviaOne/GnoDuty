package gnoduty

import (
	"github.com/gorilla/websocket"
)

type StatusType int

const (
	Statusmissed StatusType = iota
	StatusPrevote
	StatusPrecommit
	StatusSigned
	StatusProposed
)

type StatusUpdate struct {
	Height int64
	Status StatusType
	Final  bool
}

// TmConn kept for dashboard server compatibility
type TmConn struct {
	*websocket.Conn
}

// WsRun delegates to PollRun — TM2 public nodes don't support WS subscriptions
func (cc *ChainConfig) WsRun() {
	cc.PollRun()
}
