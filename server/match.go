package server

import (
	"fmt"
	"net"
	"sync"
	"time"
)

type SendConn struct {
	id       string
	conn     net.Conn
	filename string
	fsize    int64

	recvConnCh chan *RecvConn
}

func NewSendConn(id string, conn net.Conn, filename string, fsize int64) *SendConn {
	return &SendConn{
		id:         id,
		conn:       conn,
		filename:   filename,
		fsize:      fsize,
		recvConnCh: make(chan *RecvConn),
	}
}

type RecvConn struct {
	id   string
	conn net.Conn
}

func NewRecvConn(id string, conn net.Conn) *RecvConn {
	return &RecvConn{
		id:   id,
		conn: conn,
	}
}

type MatchController struct {
	senders map[string]*SendConn

	mu sync.Mutex
}

func NewMatchController() *MatchController {
	return &MatchController{
		senders: make(map[string]*SendConn),
	}
}

// block until there is a same ID recv conn or timeout
func (mc *MatchController) DealSendConn(sc *SendConn, timeout time.Duration) error {
	mc.mu.Lock()
	if _, ok := mc.senders[sc.id]; ok {
		mc.mu.Unlock()
		return fmt.Errorf("id is repeated")
	}
	mc.senders[sc.id] = sc
	mc.mu.Unlock()

	select {
	case <-sc.recvConnCh:
	case <-time.After(timeout):
		mc.mu.Lock()
		if tmp, ok := mc.senders[sc.id]; ok && tmp == sc {
			delete(mc.senders, sc.id)
		}
		mc.mu.Unlock()
		return fmt.Errorf("timeout waiting recv conn")
	}
	return nil
}

func (mc *MatchController) DealRecvConn(rc *RecvConn) (filename string, fsize int64, err error) {
	mc.mu.Lock()
	sc, ok := mc.senders[rc.id]
	if ok {
		delete(mc.senders, rc.id)
	}
	mc.mu.Unlock()

	if !ok {
		err = fmt.Errorf("no target sender")
		return
	}
	filename = sc.filename
	fsize = sc.fsize

	select {
	case sc.recvConnCh <- rc:
	default:
		err = fmt.Errorf("no target sender")
		return
	}
	return
}
