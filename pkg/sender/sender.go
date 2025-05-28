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
	frameSize                  int
	minFrameSize               int
	maxFrameSize               int
	adaptiveFrameSizingEnabled bool

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

	rttStats         *RTTStats
	lastFrameSizeAdj time.Time

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

	minFrameSize := 1024      // 1KB minimum
	maxFrameSize := 64 * 1024 // 64KB maximum

	if frameSize < minFrameSize {
		frameSize = minFrameSize
	} else if frameSize > maxFrameSize {
		frameSize = maxFrameSize
	}

	now := time.Now()
	s := &Sender{
		id:                         id,
		frameSize:                  frameSize,
		minFrameSize:               minFrameSize,
		maxFrameSize:               maxFrameSize,
		adaptiveFrameSizingEnabled: true,
		src:                        src,
		frameCh:                    make(chan *SendFrame),
		ackCh:                      make(chan *stream.Ack),
		dynamicAllocationEnabled:   false,
		transfers:                  make(map[int]*Transfer),
		totalThroughput:            0,
		allocationRatios:           make(map[int]float64),
		maxBufferCount:             maxBufferCount,
		retryFrames:                make([]*SendFrame, 0),
		limiter:                    make(chan struct{}, maxBufferCount),
		waitAcks:                   make(map[uint32]*SendFrame),
		bufferFrames:               make([]*SendFrame, 0),
		rttStats:                   NewRTTStats(),
		lastFrameSizeAdj:           now,
		sendShutdown:               shutdown.New(),
		ackShutdown:                shutdown.New(),
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

func (sender *Sender) EnableAdaptiveFrameSizing() {
	sender.mu.Lock()
	defer sender.mu.Unlock()

	sender.adaptiveFrameSizingEnabled = true
}

func (sender *Sender) SetFrameSizeBounds(minSize, maxSize int) error {
	if !stream.IsValidFrameSize(minSize) || !stream.IsValidFrameSize(maxSize) {
		return fmt.Errorf("invalid frame size bounds")
	}

	if minSize > maxSize {
		return fmt.Errorf("minimum frame size cannot be larger than maximum")
	}

	sender.mu.Lock()
	defer sender.mu.Unlock()

	sender.minFrameSize = minSize
	sender.maxFrameSize = maxSize

	if sender.frameSize < minSize {
		sender.frameSize = minSize
	} else if sender.frameSize > maxSize {
		sender.frameSize = maxSize
	}

	return nil
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
	for _, transfer := range sender.transfers {
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

		if sender.adaptiveFrameSizingEnabled {
			now := time.Now()
			if now.Sub(sender.lastFrameSizeAdj) > time.Second {
				sender.mu.Lock()

				smoothedRTT := sender.rttStats.GetSmoothedRTT()
				rttVar := sender.rttStats.GetRTTVariation()
				minRTT := sender.rttStats.GetMinRTT()

				currentSize := sender.frameSize
				newSize := currentSize

				// If network is stable (low RTT variation), increase frame size
				if rttVar < smoothedRTT/4 && smoothedRTT < minRTT*1.5 {
					// Network is stable, increase frame size
					newSize = int(float64(currentSize) * 1.25) // Increase by 25%

					if newSize > sender.maxFrameSize {
						newSize = sender.maxFrameSize
					}
				} else if rttVar > smoothedRTT/2 || smoothedRTT > minRTT*2 {
					// Network is unstable or congested, decrease frame size
					newSize = int(float64(currentSize) * 0.75) // Decrease by 25%

					if newSize < sender.minFrameSize {
						newSize = sender.minFrameSize
					}
				}

				if newSize != currentSize {
					sender.frameSize = newSize
				}

				sender.lastFrameSizeAdj = now
				sender.mu.Unlock()
			}
		}

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

		sender.mu.Lock()
		currentFrameSize := sender.frameSize
		sender.mu.Unlock()

		// no retry frames, get a new frame from src
		buf := make([]byte, currentFrameSize)
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

		if sender.dynamicAllocationEnabled {
			if count%10 == 0 {
				sender.updateAllocationRatios()
			}

			if len(sender.transfers) > 1 {
				var maxThroughput, minThroughput float64
				maxThroughput = 0
				minThroughput = float64(^uint(0) >> 1) // Max int value

				for _, transfer := range sender.transfers {
					if transfer.currentThroughput > maxThroughput {
						maxThroughput = transfer.currentThroughput
					}
					if transfer.currentThroughput > 0 && transfer.currentThroughput < minThroughput {
						minThroughput = transfer.currentThroughput
					}
				}

				if minThroughput == float64(^uint(0)>>1) || maxThroughput == 0 ||
					maxThroughput/minThroughput < 1.5 {
					sf.SetTransferID(-1)
				} else {
					var fastestWorkerID int
					for id, transfer := range sender.transfers {
						if transfer.currentThroughput == maxThroughput {
							fastestWorkerID = id
							break
						}
					}
					sf.SetTransferID(fastestWorkerID)
				}
			}
		}
		sender.mu.Unlock()

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
			if !waitSendFrame.GetSendTime().IsZero() {
				rtt := time.Since(waitSendFrame.GetSendTime())
				waitSendFrame.SetRTT(rtt)
				sender.rttStats.UpdateRTT(waitSendFrame.GetSendTime())
			}

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
