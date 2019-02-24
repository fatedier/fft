package worker

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/fatedier/fft/pkg/log"
	"github.com/fatedier/fft/pkg/msg"
	gio "github.com/fatedier/golib/io"
)

type TransferConn struct {
	isSender bool
	id       string
	conn     net.Conn

	pairConnCh chan *TransferConn
}

func NewTransferConn(id string, conn net.Conn, isSender bool) *TransferConn {
	return &TransferConn{
		isSender:   isSender,
		id:         id,
		conn:       conn,
		pairConnCh: make(chan *TransferConn),
	}
}

type MatchController struct {
	conns map[string]*TransferConn

	mu sync.Mutex
}

func NewMatchController() *MatchController {
	return &MatchController{
		conns: make(map[string]*TransferConn),
	}
}

// block until there is a same ID transfer conn or timeout
func (mc *MatchController) DealTransferConn(tc *TransferConn, timeout time.Duration) error {
	mc.mu.Lock()
	pairConn, ok := mc.conns[tc.id]
	if !ok {
		mc.conns[tc.id] = tc
	} else {
		delete(mc.conns, tc.id)
	}
	mc.mu.Unlock()

	if !ok {
		select {
		case pairConn := <-tc.pairConnCh:
			if tc.isSender {
				msg.WriteMsg(tc.conn, &msg.NewSendFileStreamResp{})
				msg.WriteMsg(pairConn.conn, &msg.NewReceiveFileStreamResp{})
			} else {
				msg.WriteMsg(tc.conn, &msg.NewReceiveFileStreamResp{})
				msg.WriteMsg(pairConn.conn, &msg.NewSendFileStreamResp{})
			}
			go func() {
				gio.Join(tc.conn, pairConn.conn)
				log.Info("ID [%s] join pair connections closed", tc.id)
			}()
		case <-time.After(timeout):
			mc.mu.Lock()
			if tmp, ok := mc.conns[tc.id]; ok && tmp == tc {
				delete(mc.conns, tc.id)
			}
			mc.mu.Unlock()
			return fmt.Errorf("timeout waiting pair connection")
		}
	} else {
		select {
		case pairConn.pairConnCh <- tc:
		default:
			return fmt.Errorf("no target pair connection")
		}
	}
	return nil
}
