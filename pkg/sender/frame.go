package sender

import (
	"sync"
	"time"

	"github.com/fatedier/fft/pkg/stream"
)

type SendFrame struct {
	frame      *stream.Frame
	tr         *Transfer
	sendTime   time.Time
	retryTimes int
	hasAck     bool

	mu sync.Mutex
}

func NewSendFrame(frame *stream.Frame) *SendFrame {
	return &SendFrame{
		frame: frame,
	}
}

func (sf *SendFrame) UpdateSendTime() {
	sf.mu.Lock()
	sf.sendTime = time.Now()
	sf.mu.Unlock()
}

func (sf *SendFrame) FrameID() uint32 {
	return sf.frame.FrameID
}

func (sf *SendFrame) Frame() *stream.Frame {
	return sf.frame
}

func (sf *SendFrame) HasAck() bool {
	return sf.hasAck
}

func (sf *SendFrame) SetAck() {
	sf.hasAck = true
}
