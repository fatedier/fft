package client

import (
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	fio "github.com/fatedier/fft/pkg/io"
	"github.com/fatedier/fft/pkg/msg"
	"github.com/fatedier/fft/pkg/sender"
	"github.com/fatedier/fft/pkg/stream"

	"github.com/cheggaaa/pb"
)

func (svc *Service) sendFile(id string, filePath string) error {
	conn, err := net.Dial("tcp", svc.serverAddr)
	if err != nil {
		return err
	}
	conn = tls.Client(conn, &tls.Config{InsecureSkipVerify: true})
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
		ID:         id,
		Name:       finfo.Name(),
		Fsize:      finfo.Size(),
		CacheCount: int64(svc.cacheCount),
	})

	fmt.Printf("Wait receiver...\n")
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
	svc.cacheCount = int(m.CacheCount)
	fmt.Printf("ID: %s\n", m.ID)
	if svc.debugMode {
		fmt.Printf("Workers: %v\n", m.Workers)
	}

	var wait sync.WaitGroup
	count := finfo.Size()
	bar := pb.New(int(count))
	bar.ShowSpeed = true
	bar.SetUnits(pb.U_BYTES)
	if !svc.debugMode {
		bar.Start()
	}

	callback := func(n int) {
		bar.Add(n)
	}

	s, err := sender.NewSender(0, fio.NewCallbackReader(f, callback), svc.frameSize, svc.cacheCount)
	if err != nil {
		return err
	}

	for _, worker := range m.Workers {
		wait.Add(1)
		go func(addr string) {
			newSendStream(s, m.ID, addr, svc.debugMode)
			wait.Done()
		}(worker)
	}
	go s.Run()
	wait.Wait()

	if !svc.debugMode {
		bar.Finish()
	}
	return nil
}

func newSendStream(s *sender.Sender, id string, addr string, debugMode bool) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		log(debugMode, "[%s] %v", addr, err)
		return
	}
	conn = tls.Client(conn, &tls.Config{InsecureSkipVerify: true})

	msg.WriteMsg(conn, &msg.NewSendFileStream{
		ID: id,
	})

	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	raw, err := msg.ReadMsg(conn)
	if err != nil {
		conn.Close()
		log(debugMode, "[%s] %v", addr, err)
		return
	}
	conn.SetReadDeadline(time.Time{})
	m, ok := raw.(*msg.NewSendFileStreamResp)
	if !ok {
		conn.Close()
		log(debugMode, "[%s] read NewSendFileStreamResp format error", addr)
		return
	}

	if m.Error != "" {
		conn.Close()
		log(debugMode, "[%s] new send file stream error: %s", addr, m.Error)
		return
	}

	s.HandleStream(stream.NewFrameStream(conn))
}
