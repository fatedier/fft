package main

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

func recvFile(serverAddr, id, filePath string, cacheCount int, callback func(n int)) error {
	isDir := false
	finfo, err := os.Stat(filePath)
	if err == nil && finfo.IsDir() {
		isDir = true
	}

	conn, err := net.Dial("tcp", serverAddr)
	if err != nil {
		return err
	}
	conn = tls.Client(conn, &tls.Config{InsecureSkipVerify: true})
	defer conn.Close()

	fmt.Printf("Receiver sending registration to server...\n")
	err = msg.WriteMsg(conn, &msg.ReceiveFile{
		ID:         id,
		CacheCount: int64(cacheCount),
	})
	if err != nil {
		return fmt.Errorf("failed to send receive file message: %v", err)
	}
	fmt.Printf("Receiver waiting for server response...\n")

	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	raw, err := msg.ReadMsg(conn)
	if err != nil {
		return fmt.Errorf("receiver failed to read response: %v", err)
	}
	conn.SetReadDeadline(time.Time{})
	fmt.Printf("Receiver got response from server\n")

	m, ok := raw.(*msg.ReceiveFileResp)
	if !ok {
		return fmt.Errorf("get receive file response format error")
	}
	if m.Error != "" {
		return fmt.Errorf(m.Error)
	}

	if len(m.Workers) == 0 {
		return fmt.Errorf("no available workers")
	}

	fmt.Printf("Recv filename: %s Size: %s\n", m.Name, pb.Format(m.Fsize).To(pb.U_BYTES).String())
	fmt.Printf("Workers: %v\n", m.Workers)

	realPath := filePath
	if isDir {
		realPath = filepath.Join(filePath, m.Name)
	}
	f, err := os.Create(realPath)
	if err != nil {
		return err
	}
	defer f.Close()

	callbackWriter := fio.NewCallbackWriter(f, callback)
	recv := receiver.NewReceiver(0, callbackWriter)

	var wait sync.WaitGroup
	for _, worker := range m.Workers {
		wait.Add(1)
		go func(addr string) {
			newRecvStream(recv, id, addr)
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

	return nil
}

func newRecvStream(recv *receiver.Receiver, id string, addr string) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		fmt.Printf("[%s] Error connecting to worker: %v\n", addr, err)
		return
	}
	conn = tls.Client(conn, &tls.Config{InsecureSkipVerify: true})
	defer conn.Close()

	msg.WriteMsg(conn, &msg.NewReceiveFileStream{
		ID: id,
	})

	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	raw, err := msg.ReadMsg(conn)
	if err != nil {
		fmt.Printf("[%s] Error reading response: %v\n", addr, err)
		return
	}
	conn.SetReadDeadline(time.Time{})

	m, ok := raw.(*msg.NewReceiveFileStreamResp)
	if !ok {
		fmt.Printf("[%s] Invalid response format\n", addr)
		return
	}

	if m.Error != "" {
		fmt.Printf("[%s] Worker error: %s\n", addr, m.Error)
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
