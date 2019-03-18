package client

import (
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	fio "github.com/fatedier/fft/pkg/io"
	"github.com/fatedier/fft/pkg/msg"
	"github.com/fatedier/fft/pkg/receiver"
	"github.com/fatedier/fft/pkg/stream"

	"github.com/cheggaaa/pb"
)

func (svc *Service) recvFile(id string, filePath string) error {
	isDir := false
	finfo, err := os.Stat(filePath)
	if err == nil && finfo.IsDir() {
		isDir = true
	}

	conn, err := net.Dial("tcp", svc.serverAddr)
	if err != nil {
		return err
	}
	conn = tls.Client(conn, &tls.Config{InsecureSkipVerify: true})
	defer conn.Close()

	msg.WriteMsg(conn, &msg.ReceiveFile{
		ID:         id,
		CacheCount: int64(svc.cacheCount),
	})

	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	raw, err := msg.ReadMsg(conn)
	if err != nil {
		return err
	}
	conn.SetReadDeadline(time.Time{})

	m, ok := raw.(*msg.ReceiveFileResp)
	if !ok {
		return fmt.Errorf("get send file response format error")
	}
	if m.Error != "" {
		return fmt.Errorf(m.Error)
	}

	if len(m.Workers) == 0 {
		return fmt.Errorf("no available workers")
	}

	fmt.Printf("Recv filename: %s Size: %s\n", m.Name, pb.Format(m.Fsize).To(pb.U_BYTES).String())
	if svc.debugMode {
		fmt.Printf("Workers: %v\n", m.Workers)
	}

	realPath := filePath
	if isDir {
		realPath = filepath.Join(filePath, m.Name)
	}
	f, err := os.Create(realPath)
	if err != nil {
		return err
	}
	defer f.Close()

	var wait sync.WaitGroup
	count := m.Fsize
	bar := pb.New(int(count))
	bar.ShowSpeed = true
	bar.SetUnits(pb.U_BYTES)

	if !svc.debugMode {
		bar.Start()
	}

	callback := func(n int) {
		bar.Add(n)
	}

	recv := receiver.NewReceiver(0, fio.NewCallbackWriter(f, callback))
	for _, worker := range m.Workers {
		wait.Add(1)
		go func(addr string) {
			newRecvStream(recv, id, addr, svc.debugMode)
			wait.Done()
		}(worker)
	}

	recvDoneCh := make(chan struct{})
	streamCloseCh := make(chan struct{})
	go func() {
		recv.Run()
		close(recvDoneCh)
	}()
	go func() {
		wait.Wait()
		close(streamCloseCh)
	}()

	select {
	case <-recvDoneCh:
	case <-streamCloseCh:
		select {
		case <-recvDoneCh:
		case <-time.After(2 * time.Second):
		}
	}

	if !svc.debugMode {
		bar.Finish()
	}
	return nil
}

func newRecvStream(recv *receiver.Receiver, id string, addr string, debugMode bool) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		log(debugMode, "[%s] %v", addr, err)
		return
	}
	conn = tls.Client(conn, &tls.Config{InsecureSkipVerify: true})

	msg.WriteMsg(conn, &msg.NewReceiveFileStream{
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
	m, ok := raw.(*msg.NewReceiveFileStreamResp)
	if !ok {
		conn.Close()
		log(debugMode, "[%s] read NewReceiveFileStreamResp format error", addr)
		return
	}

	if m.Error != "" {
		conn.Close()
		log(debugMode, "[%s] new recv file stream error: %s", addr, m.Error)
		return
	}

	s := stream.NewFrameStream(conn)
	for {
		frame, err := s.ReadFrame()
		if err != nil {
			return
		}
		recv.RecvFrame(frame)
		err = s.WriteAck(&stream.Ack{
			FileID:  frame.FileID,
			FrameID: frame.FrameID,
		})
		if err != nil {
			return
		}
	}
}
