package worker

import (
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	fio "github.com/fatedier/fft/pkg/io"
	"github.com/fatedier/fft/pkg/log"
	"github.com/fatedier/fft/pkg/msg"

	gio "github.com/fatedier/golib/io"

	"golang.org/x/time/rate"
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

	rateLimit *rate.Limiter
	statFunc  func(int)
	mu        sync.Mutex
}

func NewMatchController(rateByte int, statFunc func(int)) *MatchController {
	if rateByte < 50*1024 {
		rateByte = 50 * 1024
	}
	return &MatchController{
		conns:     make(map[string]*TransferConn),
		rateLimit: rate.NewLimiter(rate.Limit(float64(rateByte)), 16*1024),
		statFunc:  statFunc,
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
			var sender, receiver io.ReadWriteCloser
			if tc.isSender {
				wrapReader := fio.NewCallbackReader(fio.NewRateReader(tc.conn, mc.rateLimit), mc.statFunc)
				sender = gio.WrapReadWriteCloser(wrapReader, tc.conn, func() error {
					return tc.conn.Close()
				})
				receiver = pairConn.conn
			} else {
				wrapReader := fio.NewCallbackReader(fio.NewRateReader(pairConn.conn, mc.rateLimit), mc.statFunc)
				sender = gio.WrapReadWriteCloser(wrapReader, pairConn.conn, func() error {
					return pairConn.conn.Close()
				})
				receiver = tc.conn
			}
			msg.WriteMsg(sender, &msg.NewSendFileStreamResp{})
			msg.WriteMsg(receiver, &msg.NewReceiveFileStreamResp{})

			go func() {
				gio.Join(sender, receiver)
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
