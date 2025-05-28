package sender

import (
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fatedier/fft/pkg/stream"

	"github.com/fatedier/golib/control/shutdown"
)

type AckWaitingObj struct {
	Frame        *stream.Frame
	HasAck       bool
	LastSendTime time.Time
}

type Sender struct {
	id uint32

	// each frame size
	frameSize int

	// send src to remote Receiver
	src io.Reader

	frameCh chan *SendFrame

	// get each ack message from ackCh
	ackCh chan *stream.Ack

	dynamicAllocationEnabled bool
	transfers                map[int]*Transfer
	totalThroughput          float64
	allocationRatios         map[int]float64
	transfersMu              sync.RWMutex

	retryFrames []*SendFrame

	maxBufferCount int
	limiter        chan struct{}
	waitAcks       map[uint32]*SendFrame
	bufferFrames   []*SendFrame

	// 1 means all frames has been sent
	sendAll      bool
	mu           sync.Mutex
	sendShutdown *shutdown.Shutdown
	ackShutdown  *shutdown.Shutdown

	count uint32
}

func NewSender(id uint32, src io.Reader, frameSize int, maxBufferCount int) (*Sender, error) {
	if !stream.IsValidFrameSize(frameSize) {
		return nil, fmt.Errorf("invalid frameSize")
	}
	if maxBufferCount <= 0 {
		maxBufferCount = 100
	}

	s := &Sender{
		id:                     id,
		frameSize:              frameSize,
		src:                    src,
		frameCh:                make(chan *SendFrame),
		ackCh:                  make(chan *stream.Ack),
		dynamicAllocationEnabled: false,
		transfers:              make(map[int]*Transfer),
		totalThroughput:        0,
		allocationRatios:       make(map[int]float64),
		maxBufferCount:         maxBufferCount,
		retryFrames:            make([]*SendFrame, 0),
		limiter:                make(chan struct{}, maxBufferCount),
		waitAcks:               make(map[uint32]*SendFrame),
		bufferFrames:           make([]*SendFrame, 0),
		sendShutdown:           shutdown.New(),
		ackShutdown:            shutdown.New(),
	}
	for i := 0; i < maxBufferCount; i++ {
		s.limiter <- struct{}{}
	}
	return s, nil
}

func (sender *Sender) EnableDynamicAllocation() {
	sender.transfersMu.Lock()
	defer sender.transfersMu.Unlock()
	
	sender.dynamicAllocationEnabled = true
}

func (sender *Sender) HandleStream(s *stream.FrameStream) {
	sender.mu.Lock()
	if sender.sendAll {
		sender.mu.Unlock()
		s.Close()
		return
	}
	sender.mu.Unlock()

	id := atomic.AddUint32(&sender.count, 1)
	trBufferCount := sender.maxBufferCount / 2
	if trBufferCount <= 0 {
		trBufferCount = 1
	}
	tr := NewTransfer(int(id), trBufferCount, s, sender.frameCh, sender.ackCh)
	
	if sender.dynamicAllocationEnabled {
		sender.transfersMu.Lock()
		sender.transfers[int(id)] = tr
		totalTransfers := len(sender.transfers)
		if totalTransfers > 0 {
			equalRatio := 1.0 / float64(totalTransfers)
			for transferID := range sender.transfers {
				sender.allocationRatios[transferID] = equalRatio
			}
		}
		sender.transfersMu.Unlock()
	}

	// block until transfer exit
	noAckFrames := tr.Run()
	
	if sender.dynamicAllocationEnabled {
		sender.transfersMu.Lock()
		delete(sender.transfers, int(id))
		delete(sender.allocationRatios, int(id))
		sender.transfersMu.Unlock()
	}
	
	if len(noAckFrames) > 0 {
		sender.mu.Lock()
		sender.retryFrames = append(sender.retryFrames, noAckFrames...)
		sender.mu.Unlock()
		for i := 0; i < len(noAckFrames); i++ {
			sender.limiter <- struct{}{}
		}
	}
}

func (sender *Sender) Run() {
	go sender.ackHandler()
	go sender.loopSend()

	sender.sendShutdown.WaitDone()
	sender.ackShutdown.WaitDone()
}

func (sender *Sender) updateAllocationRatios() {
	sender.transfersMu.RLock()
	defer sender.transfersMu.RUnlock()
	
	if len(sender.transfers) <= 1 {
		for id := range sender.transfers {
			sender.allocationRatios[id] = 1.0
		}
		return
	}
	
	// Calculate total throughput across all transfers
	totalThroughput := 0.0
	for id, transfer := range sender.transfers {
		if transfer.currentThroughput > 0 {
			totalThroughput += transfer.currentThroughput
		} else {
			totalThroughput += 1.0
		}
	}
	
	if totalThroughput > 0 {
		for id, transfer := range sender.transfers {
			throughput := transfer.currentThroughput
			if throughput <= 0 {
				throughput = 1.0 // Default value if no data yet
			}
			sender.allocationRatios[id] = throughput / totalThroughput
		}
	} else {
		equalRatio := 1.0 / float64(len(sender.transfers))
		for id := range sender.transfers {
			sender.allocationRatios[id] = equalRatio
		}
	}
}

func (sender *Sender) loopSend() {
	defer sender.sendShutdown.Done()

	var count uint32
	for {
		<-sender.limiter

		// retry first
		var retryFrame *SendFrame
		sender.mu.Lock()
		if len(sender.retryFrames) > 0 {
			retryFrame = sender.retryFrames[0]
			sender.retryFrames = sender.retryFrames[1:]
		}
		sender.mu.Unlock()

		if retryFrame != nil {
			sender.frameCh <- retryFrame
			continue
		}

		// don't need get frames from src
		if sender.sendAll {
			continue
		}

		// no retry frames, get a new frame from src
		buf := make([]byte, sender.frameSize)
		n, err := sender.src.Read(buf)
		if err == io.EOF {
			// send last frame and it's buffer is nil
			f := stream.NewFrame(sender.id, count, nil)
			sf := NewSendFrame(f)

			sender.mu.Lock()
			sender.sendAll = true
			sender.waitAcks[sf.FrameID()] = sf
			sender.bufferFrames = append(sender.bufferFrames, sf)
			sender.mu.Unlock()

			sender.frameCh <- sf
			return
		}
		if err != nil {
			close(sender.frameCh)
			return
		}
		buf = buf[:n]

		f := stream.NewFrame(0, count, buf)
		sf := NewSendFrame(f)
		sender.mu.Lock()
		sender.waitAcks[sf.FrameID()] = sf
		sender.bufferFrames = append(sender.bufferFrames, sf)
		sender.mu.Unlock()

		if sender.dynamicAllocationEnabled {
			if count%10 == 0 {
				sender.updateAllocationRatios()
			}
		}

		sender.frameCh <- sf
		count++
	}
}

func (sender *Sender) ackHandler() {
	defer sender.ackShutdown.Done()

	for {
		ack, ok := <-sender.ackCh
		if !ok {
			return
		}

		finished := false
		sender.mu.Lock()
		waitSendFrame, ok := sender.waitAcks[ack.FrameID]
		if ok {
			waitSendFrame.SetAck()
			delete(sender.waitAcks, ack.FrameID)

			// if all frames has been sent and no waiting acks, we are success
			if sender.sendAll && len(sender.waitAcks) == 0 {
				finished = true
			}

			removeCount := 0
			// remove all continuous buffer frames with ack
			for _, sf := range sender.bufferFrames {
				if sf.HasAck() {
					removeCount++
					sender.limiter <- struct{}{}
				} else {
					break
				}
			}
			sender.bufferFrames = sender.bufferFrames[removeCount:]
		}
		sender.mu.Unlock()

		if finished {
			close(sender.ackCh)
			close(sender.frameCh)
			close(sender.limiter)
			return
		}
	}
}
