package server

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/fatedier/fft/pkg/log"
	"github.com/fatedier/fft/pkg/msg"
)

var (
	ErrPublicAddr = errors.New("no public address")
)

type Worker struct {
	conn           net.Conn
	port           int64
	advicePublicIP string
	publicAddr     string
}

func NewWorker(port int64, advicePublicIP string, conn net.Conn) *Worker {
	return &Worker{
		port:           port,
		advicePublicIP: advicePublicIP,
		conn:           conn,
	}
}

func (w *Worker) PublicAddr() string {
	return w.publicAddr
}

func (w *Worker) DetectPublicAddr() error {
	host, _, err := net.SplitHostPort(w.conn.RemoteAddr().String())
	if err != nil {
		return fmt.Errorf("parse worker address error: %v", err)
	}

	ip := w.advicePublicIP
	if ip == "" {
		ip = host
	}
	detectAddr := net.JoinHostPort(ip, fmt.Sprintf("%d", w.port))
	log.Debug("worker detect address: %s", detectAddr)

	detectConn, err := net.Dial("tcp", detectAddr)
	if err != nil {
		log.Warn("dial worker public address error: %v", err)
		return ErrPublicAddr
	}
	detectConn = tls.Client(detectConn, &tls.Config{InsecureSkipVerify: true})
	defer detectConn.Close()

	msg.WriteMsg(detectConn, &msg.Ping{})

	detectConn.SetReadDeadline(time.Now().Add(5 * time.Second))
	m, err := msg.ReadMsg(detectConn)
	if err != nil {
		log.Warn("read pong from detectConn error: %v", err)
		return ErrPublicAddr
	}
	if _, ok := m.(*msg.Pong); !ok {
		return ErrPublicAddr
	}

	w.publicAddr = detectAddr
	return nil
}

func (w *Worker) RunKeepAlive(closeCallback func()) {
	defer func() {
		if closeCallback != nil {
			closeCallback()
		}
	}()

	for {
		w.conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		m, err := msg.ReadMsg(w.conn)
		if err != nil {
			w.conn.Close()
			return
		}

		if _, ok := m.(*msg.Ping); !ok {
			w.conn.Close()
			return
		}
		msg.WriteMsg(w.conn, &msg.Pong{})
	}
}

type WorkerGroup struct {
	workers map[string]*Worker

	mu sync.RWMutex
}

func NewWorkerGroup() *WorkerGroup {
	return &WorkerGroup{
		workers: make(map[string]*Worker),
	}
}

func (wg *WorkerGroup) RegisterWorker(w *Worker) {
	closeCallback := func() {
		wg.mu.Lock()
		delete(wg.workers, w.PublicAddr())
		wg.mu.Unlock()
	}

	wg.mu.Lock()
	wg.workers[w.PublicAddr()] = w
	go w.RunKeepAlive(closeCallback)
	wg.mu.Unlock()
}

func (wg *WorkerGroup) GetAvailableWorkerAddrs() []string {
	addrs := make([]string, 0)

	wg.mu.RLock()
	defer wg.mu.RUnlock()
	for addr, _ := range wg.workers {
		addrs = append(addrs, addr)
	}
	return addrs
}
