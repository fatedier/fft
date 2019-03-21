package worker

import (
	"crypto/tls"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/fatedier/fft/pkg/log"
	"github.com/fatedier/fft/pkg/msg"
	"github.com/fatedier/fft/version"
)

type Register struct {
	port           int64
	advicePublicIP string
	serverAddr     string
	conn           net.Conn

	closed bool
	mu     sync.Mutex
}

func NewRegister(port int64, advicePublicIP string, serverAddr string) (*Register, error) {
	conn, err := net.Dial("tcp", serverAddr)
	if err != nil {
		return nil, err
	}
	conn = tls.Client(conn, &tls.Config{InsecureSkipVerify: true})

	return &Register{
		port:           port,
		advicePublicIP: advicePublicIP,
		serverAddr:     serverAddr,
		conn:           conn,
		closed:         false,
	}, nil
}

func (r *Register) Register() error {
	msg.WriteMsg(r.conn, &msg.RegisterWorker{
		Version:  version.Full(),
		PublicIP: r.advicePublicIP,
		BindPort: r.port,
	})

	r.conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	m, err := msg.ReadMsg(r.conn)
	if err != nil {
		log.Warn("read RegisterWorkerResp error: %v", err)
		return err
	}
	r.conn.SetReadDeadline(time.Time{})

	resp, ok := m.(*msg.RegisterWorkerResp)
	if !ok {
		return fmt.Errorf("read RegisterWorkerResp format error")
	}

	if resp.Error != "" {
		return fmt.Errorf(resp.Error)
	}
	return nil
}

func (r *Register) RunKeepAlive() {
	var err error
	for {
		// send ping and read pong
		for {
			// in case it is closed before
			if r.conn == nil {
				break
			}

			msg.WriteMsg(r.conn, &msg.Ping{})

			_, err = msg.ReadMsg(r.conn)
			if err != nil {
				r.conn.Close()
				break
			}

			time.Sleep(10 * time.Second)
		}

		for {
			r.mu.Lock()
			closed := r.closed
			r.mu.Unlock()
			if r.closed {
				return
			}

			conn, err := net.Dial("tcp", r.serverAddr)
			if err != nil {
				time.Sleep(10 * time.Second)
				continue
			}
			conn = tls.Client(conn, &tls.Config{InsecureSkipVerify: true})

			r.mu.Lock()
			closed = r.closed
			if closed {
				conn.Close()
				r.mu.Unlock()
				return
			}
			r.conn = conn
			r.mu.Unlock()

			err = r.Register()
			if err != nil {
				r.conn.Close()
				time.Sleep(10 * time.Second)
				continue
			}

			break
		}
	}
}

func (r *Register) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
	r.conn.Close()
}

// Reset can be only called after Close
func (r *Register) Reset() {
	r.closed = false
	r.conn = nil
}
