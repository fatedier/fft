package main

import (
	"fmt"
	"io"
	"net"
	"os"
	"time"

	"github.com/fatedier/fft/pkg/stream"
	"github.com/fatedier/fft/pkg/transfer"
)

var (
	targetAddr = ":9999"
	count      = 2
	localFile  = "./t"
	frameSize  = 50
)

func main() {
	frameCh := make(chan *stream.Frame, count)
	for i := 0; i < count; i++ {
		conn, err := net.Dial("tcp", targetAddr)
		if err != nil {
			fmt.Println(err)
			return
		}

		s := stream.NewFrameStream(conn)
		t := transfer.NewTransfer(i, s, frameCh)
		go t.Run()
	}

	file, err := os.Open(localFile)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer file.Close()

	id := 0
	for {
		buf := make([]byte, frameSize)
		n, err := file.Read(buf)
		if err == io.EOF {
			f := stream.NewFrame(0, uint32(id), nil)
			frameCh <- f

			close(frameCh)
			break
		}
		if err != nil {
			close(frameCh)
			return
		}
		buf = buf[:n]

		fmt.Println("get file content:", string(buf))
		f := stream.NewFrame(0, uint32(id), buf)
		frameCh <- f
		id++
	}

	time.Sleep(2 * time.Second)
}
