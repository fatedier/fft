package sender

import (
	"io"
	"sync/atomic"

	"github.com/fatedier/fft/pkg/stream"
	"github.com/fatedier/fft/pkg/transfer"
)

type Sender struct {
	id      uint32
	src     io.Reader
	frameCh chan *stream.Frame

	count uint32
}

func NewSender(id uint32, src io.Reader) *Sender {
	return &Sender{
		id:      id,
		src:     src,
		frameCh: make(chan *stream.Frame, 100),
	}
}

func (sender *Sender) HandleStream(s *stream.FrameStream) {
	id := atomic.AddUint32(&sender.count, 1)
	tr := transfer.NewTransfer(int(id), s, sender.frameCh)

	go tr.Run()
	tr.WaitDone()
}

func (sender *Sender) Run() {
	count := 0
	for {
		buf := make([]byte, stream.FrameSize)
		n, err := sender.src.Read(buf)
		if err == io.EOF {
			f := stream.NewFrame(sender.id, uint32(count), nil)
			sender.frameCh <- f

			close(sender.frameCh)
			break
		}
		if err != nil {
			close(sender.frameCh)
			return
		}
		buf = buf[:n]

		f := stream.NewFrame(0, uint32(count), buf)
		sender.frameCh <- f
		count++
	}
}
