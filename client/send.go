package client

import (
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"github.com/fatedier/fft/pkg/msg"
	"github.com/fatedier/fft/pkg/sender"
	"github.com/fatedier/fft/pkg/stream"
)

func (svc *Service) sendFile(id string, filePath string) error {
	conn, err := net.Dial("tcp", svc.serverAddr)
	if err != nil {
		return err
	}
	defer conn.Close()

	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	finfo, err := f.Stat()
	if err != nil {
		return err
	}
	if finfo.IsDir() {
		return fmt.Errorf("send file can't be a directory")
	}

	msg.WriteMsg(conn, &msg.SendFile{
		ID:   id,
		Name: finfo.Name(),
	})

	conn.SetReadDeadline(time.Now().Add(120 * time.Second))
	raw, err := msg.ReadMsg(conn)
	if err != nil {
		return err
	}
	conn.SetReadDeadline(time.Time{})

	m, ok := raw.(*msg.SendFileResp)
	if !ok {
		return fmt.Errorf("get send file response format error")
	}
	if m.Error != "" {
		return fmt.Errorf(m.Error)
	}

	if len(m.Workers) == 0 {
		return fmt.Errorf("no available workers")
	}
	fmt.Printf("ID: %s\n", m.ID)
	if svc.debugMode {
		fmt.Printf("Workers: %v\n", m.Workers)
	}

	var wait sync.WaitGroup
	doneCh := make(chan struct{})
	s := sender.NewSender(0, f)

	for _, worker := range m.Workers {
		wait.Add(1)
		go func(addr string) {
			newSendStream(doneCh, s, m.ID, addr, svc.debugMode)
			wait.Done()
		}(worker)
	}
	s.Run()
	close(doneCh)
	wait.Wait()
	return nil
}

func newSendStream(doneCh chan struct{}, s *sender.Sender, id string, addr string, debugMode bool) {
	first := true
	for {
		select {
		case <-doneCh:
			return
		default:
		}

		if !first {
			time.Sleep(3 * time.Second)
		} else {
			first = false
		}

		conn, err := net.Dial("tcp", addr)
		if err != nil {
			log(debugMode, "[%s] %v", addr, err)
			continue
		}

		msg.WriteMsg(conn, &msg.NewSendFileStream{
			ID: id,
		})

		conn.SetReadDeadline(time.Now().Add(10 * time.Second))
		raw, err := msg.ReadMsg(conn)
		if err != nil {
			conn.Close()
			log(debugMode, "[%s] %v", addr, err)
			continue
		}
		conn.SetReadDeadline(time.Time{})
		m, ok := raw.(*msg.NewSendFileStreamResp)
		if !ok {
			conn.Close()
			log(debugMode, "[%s] read NewSendFileStreamResp format error", addr)
			continue
		}

		if m.Error != "" {
			conn.Close()
			log(debugMode, "[%s] new send file stream error: %s", addr, m.Error)
			continue
		}
		fmt.Printf("connect to worker [%s] success\n", addr)

		s.HandleStream(stream.NewFrameStream(conn))
		break
	}
}
