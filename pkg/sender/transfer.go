package sender

import (
	"sort"
	"sync"
	"time"

	"github.com/fatedier/fft/pkg/stream"

	"github.com/fatedier/golib/control/limit"
	"github.com/fatedier/golib/control/shutdown"
)

type Transfer struct {
	id             int
	maxBufferCount int
	inSlowStart    bool
	waitAcks       map[uint32]*SendFrame

	framesSent        uint64
	bytesTransferred  uint64
	startTime         time.Time
	lastMetricTime    time.Time
	currentThroughput float64 // bytes per second
	
	rttStats          *RTTStats
	congestionWindow  int64
	lastCongestionAdj time.Time

	s            *stream.FrameStream
	limiter      *limit.Limiter
	frameCh      chan *SendFrame
	ackCh        chan *stream.Ack
	mu           sync.Mutex
	sendShutdown *shutdown.Shutdown
	recvShutdown *shutdown.Shutdown
}

func NewTransfer(id int, maxBufferCount int, s *stream.FrameStream,
	frameCh chan *SendFrame, ackCh chan *stream.Ack) *Transfer {

	if maxBufferCount <= 0 {
		maxBufferCount = 10
	}
	now := time.Now()
	t := &Transfer{
		id:                id,
		maxBufferCount:    maxBufferCount,
		inSlowStart:       true,
		waitAcks:          make(map[uint32]*SendFrame),
		framesSent:        0,
		bytesTransferred:  0,
		startTime:         now,
		lastMetricTime:    now,
		currentThroughput: 0,
		rttStats:          NewRTTStats(),
		congestionWindow:  int64(1),
		lastCongestionAdj: now,
		s:                 s,
		limiter:           limit.NewLimiter(int64(1)),
		frameCh:           frameCh,
		ackCh:             ackCh,
		sendShutdown:      shutdown.New(),
		recvShutdown:      shutdown.New(),
	}

	t.limiter.SetLimit(int64(1))
	return t
}

// Block until all frames sended
func (t *Transfer) Run() (noAckFrames []*SendFrame) {
	go t.ackReceiver()
	go t.frameSender()

	t.recvShutdown.WaitDone()
	t.sendShutdown.WaitDone()

	for _, f := range t.waitAcks {
		noAckFrames = append(noAckFrames, f)
	}
	if len(noAckFrames) > 0 {
		sort.Slice(noAckFrames, func(i, j int) bool {
			return noAckFrames[i].FrameID() < noAckFrames[j].FrameID()
		})
	}

	t.limiter.Close()
	return
}

func (t *Transfer) frameSender() {
	defer t.sendShutdown.Done()

	for {
		// block by limiter
		n := int(t.limiter.LimitNum())
		err := t.limiter.Acquire(time.Second)
		if err != nil {
			if err == limit.ErrTimeout {
				if n/2 == 0 {
					n = 1
				}
				t.limiter.SetLimit(int64(n))
				continue
			} else {
				return
			}
		}

		if n < t.maxBufferCount {
			if t.inSlowStart {
				n = 2 * n
			} else {
				rttVar := t.rttStats.GetRTTVariation()
				smoothedRTT := t.rttStats.GetSmoothedRTT()
				
				if rttVar < smoothedRTT/4 {
					n += n/4
				} else {
					n++
				}
			}

			if n > t.maxBufferCount {
				t.inSlowStart = false
				n = t.maxBufferCount
			}
			t.limiter.SetLimit(int64(n))
		}

		sf, ok := <-t.frameCh
		if !ok {
			t.s.Close()
			return
		}

		if sf.GetTransferID() != -1 && sf.GetTransferID() != t.id {
			select {
			case t.frameCh <- sf:
			default:
				sf.SetTransferID(t.id)
			}
			continue
		}

		sf.UpdateSendTime()

		t.mu.Lock()
		t.waitAcks[sf.FrameID()] = sf
		t.mu.Unlock()

		if !t.inSlowStart && t.framesSent > 20 {
			t.mu.Lock()
			now := time.Now()
			smoothedRTT := t.rttStats.GetSmoothedRTT()
			rttTimeout := smoothedRTT * 3 // Timeout threshold
			
			for frameID, waitFrame := range t.waitAcks {
				if frameID != sf.FrameID() && !waitFrame.sendTime.IsZero() {
					elapsed := now.Sub(waitFrame.sendTime)
					if elapsed > rttTimeout {
						waitFrame.retryTimes++
						waitFrame.UpdateSendTime() // Update send time for retransmission
						
						err = t.s.WriteFrame(waitFrame.Frame())
						if err != nil {
							t.mu.Unlock()
							return
						}
					}
				}
			}
			t.mu.Unlock()
		}

		err = t.s.WriteFrame(sf.Frame())
		if err != nil {
			return
		}
	}
}

func (t *Transfer) ackReceiver() {
	defer t.recvShutdown.Done()

	for {
		ack, err := t.s.ReadAck()
		if err != nil {
			t.limiter.Close()
			return
		}

		t.mu.Lock()
		sf, ok := t.waitAcks[ack.FrameID]
		if ok {
			if !sf.sendTime.IsZero() {
				t.rttStats.UpdateRTT(sf.sendTime)
			}

			t.framesSent++
			if sf.Frame().Buf != nil {
				t.bytesTransferred += uint64(len(sf.Frame().Buf))
			}

			// Calculate throughput every 10 frames or at least once per second
			now := time.Now()
			if t.framesSent%10 == 0 || now.Sub(t.lastMetricTime) > time.Second {
				elapsedSeconds := now.Sub(t.lastMetricTime).Seconds()
				if elapsedSeconds > 0 {
					// Calculate bytes per second
					t.currentThroughput = float64(t.bytesTransferred) / elapsedSeconds

					if now.Sub(t.lastCongestionAdj) > 100*time.Millisecond {
						smoothedRTT := t.rttStats.GetSmoothedRTT()
						rttVar := t.rttStats.GetRTTVariation()
						minRTT := t.rttStats.GetMinRTT()
						
						currentLimit := t.limiter.LimitNum()
						newLimit := currentLimit

						if t.inSlowStart {
							newLimit = currentLimit * 2
							
							if smoothedRTT > minRTT*2 && t.framesSent > 20 {
								t.inSlowStart = false
								newLimit = currentLimit
							}
						} else {
							if rttVar < smoothedRTT/4 {
								newLimit = currentLimit + (currentLimit / 8)
							} else {
								newLimit = currentLimit + (currentLimit / 16)
							}
							
							if smoothedRTT > minRTT*3 {
								newLimit = currentLimit / 2
								if newLimit < 1 {
									newLimit = 1
								}
							}
						}

						// Cap at maxBufferCount
						if newLimit > int64(t.maxBufferCount) {
							newLimit = int64(t.maxBufferCount)
							t.inSlowStart = false
						}

						t.congestionWindow = newLimit
						t.limiter.SetLimit(newLimit)
						t.lastCongestionAdj = now
					}

					t.lastMetricTime = now
					t.bytesTransferred = 0
				}
			}

			delete(t.waitAcks, ack.FrameID)
		}
		t.mu.Unlock()
		if ok {
			t.limiter.Release()
		}

		t.ackCh <- ack
	}
}
