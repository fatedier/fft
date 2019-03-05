package main

import (
	"fmt"
	"io"
	"net"
	"os"

	"github.com/fatedier/fft/pkg/receiver"
	"github.com/fatedier/fft/pkg/stream"
)

var (
	bindAddr  = ":9999"
	localFile = "./dst.tmp"
)

func main() {
	file, err := os.Create(localFile)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer file.Close()

	recv := receiver.NewReceiver(0, file)
	go recv.Run()

	serve(recv)
}

func serve(recv *receiver.Receiver) {
	l, err := net.Listen("tcp", bindAddr)
	if err != nil {
		fmt.Println(err)
		return
	}

	for {
		conn, err := l.Accept()
		if err != nil {
			fmt.Println("accept error:", err)
			return
		}
		fmt.Println("get new conn")

		s := stream.NewFrameStream(conn)
		go handleStream(s, recv)
	}
}

func handleStream(s *stream.FrameStream, recv *receiver.Receiver) {
	for {
		frame, err := s.ReadFrame()
		if err == io.EOF {
			return
		}
		if err != nil {
			fmt.Println(err)
			return
		}

		fmt.Println("read frame:", frame.FrameID, "size:", len(frame.Buf))
		recv.RecvFrame(frame)
	}
}
