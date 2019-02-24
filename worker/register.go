package worker

import (
	"fmt"
	"net"
	"time"

	"github.com/fatedier/fft/pkg/log"
	"github.com/fatedier/fft/pkg/msg"
	"github.com/fatedier/fft/version"
)

type Register struct {
	port           int64
	advicePublicIP string
	serverAddr     string
}

func NewRegister(port int64, advicePublicIP string, serverAddr string) *Register {
	return &Register{
		port:           port,
		advicePublicIP: advicePublicIP,
		serverAddr:     serverAddr,
	}
}

func (r *Register) Register(conn net.Conn) error {
	msg.WriteMsg(conn, &msg.RegisterWorker{
		Version:  version.Full(),
		PublicIP: r.advicePublicIP,
		BindPort: r.port,
	})

	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	m, err := msg.ReadMsg(conn)
	if err != nil {
		log.Warn("read RegisterWorkerResp error: %v", err)
		return err
	}
	conn.SetReadDeadline(time.Time{})

	resp, ok := m.(*msg.RegisterWorkerResp)
	if !ok {
		return fmt.Errorf("read RegisterWorkerResp format error")
	}

	if resp.Error != "" {
		return fmt.Errorf(resp.Error)
	}
	return nil
}

func (r *Register) RunKeepAlive(conn net.Conn) error {
	var err error
	for {
		// send ping and read pong
		for {
			msg.WriteMsg(conn, &msg.Ping{})

			_, err = msg.ReadMsg(conn)
			if err != nil {
				conn.Close()
				break
			}

			time.Sleep(10 * time.Second)
		}

		for {
			conn, err = net.Dial("tcp", r.serverAddr)
			if err != nil {
				time.Sleep(10 * time.Second)
				continue
			}

			err = r.Register(conn)
			if err != nil {
				time.Sleep(10 * time.Second)
				continue
			}

			break
		}
	}
	return nil
}
