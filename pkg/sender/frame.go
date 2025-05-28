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
	transferID int        // ID of the transfer this frame is assigned to

	mu sync.Mutex
}

func NewSendFrame(frame *stream.Frame) *SendFrame {
	return &SendFrame{
		frame:      frame,
		transferID: -1, // -1 means not assigned to any specific transfer
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

func (sf *SendFrame) SetTransferID(id int) {
	sf.transferID = id
}

func (sf *SendFrame) GetTransferID() int {
	return sf.transferID
}
