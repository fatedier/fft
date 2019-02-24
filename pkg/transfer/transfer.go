package transfer

import (
	"github.com/fatedier/fft/pkg/stream"
)

type Transfer struct {
	id      int
	s       *stream.FrameStream
	frameCh chan *stream.Frame
	closeCh chan struct{}
}

func NewTransfer(id int, s *stream.FrameStream, frameCh chan *stream.Frame) *Transfer {
	return &Transfer{
		id:      id,
		s:       s,
		frameCh: frameCh,
		closeCh: make(chan struct{}),
	}
}

// Block until all frames sended
func (t *Transfer) Run() {
	defer func() {
		close(t.closeCh)
	}()

	for {
		frame, ok := <-t.frameCh
		if !ok {
			t.s.Close()
			return
		}

		err := t.s.WriteFrame(frame)
		if err != nil {
			return
		}
	}
}

func (t *Transfer) WaitDone() {
	<-t.closeCh
}
