package main

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
)

func sendFile(serverAddr, id, filePath string, frameSize, cacheCount int, callback func(n int)) error {
	conn, err := net.Dial("tcp", serverAddr)
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
		CacheCount: int64(cacheCount),
	})

	fmt.Printf("Wait receiver...\n")
	conn.SetReadDeadline(time.Now().Add(120 * time.Second))
	raw, err := msg.ReadMsg(conn)
	if err != nil {
		return fmt.Errorf("error waiting for receiver: %v", err)
	}
	conn.SetReadDeadline(time.Time{})
	fmt.Printf("Received response from server\n")

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
	fmt.Printf("Workers: %v\n", m.Workers)

	callbackReader := fio.NewCallbackReader(f, callback)

	s, err := sender.NewSender(0, callbackReader, frameSize, cacheCount)
	if err != nil {
		return err
	}

	var wait sync.WaitGroup
	for _, workerAddr := range m.Workers {
		wait.Add(1)
		go func(addr string) {
			defer wait.Done()
			connectToWorker(s, m.ID, addr)
		}(workerAddr)
	}

	go s.Run()

	wait.Wait()

	return nil
}
