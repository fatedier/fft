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
					n += n / 4
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
			var framesToRetry []*SendFrame

			t.mu.Lock()
			now := time.Now()
			smoothedRTT := t.rttStats.GetSmoothedRTT()
			rttTimeout := smoothedRTT * 3 // Timeout threshold

			retryCount := 0
			maxRetryPerCycle := 5

			for frameID, waitFrame := range t.waitAcks {
				if frameID != sf.FrameID() && !waitFrame.sendTime.IsZero() && waitFrame.retryTimes < 3 {
					elapsed := now.Sub(waitFrame.sendTime)
					if elapsed > rttTimeout {
						framesToRetry = append(framesToRetry, waitFrame)
						retryCount++
						if retryCount >= maxRetryPerCycle {
							break
						}
					}
				}
			}
			t.mu.Unlock()

			for _, waitFrame := range framesToRetry {
				waitFrame.retryTimes++
				waitFrame.UpdateSendTime()

				err = t.s.WriteFrame(waitFrame.Frame())
				if err != nil {
					return
				}
			}
		}

		err = t.s.WriteFrame(sf.Frame())
		if err != nil {
			return
		}
	}
}

func (t *Transfer) adjustLimitInSlowStart(currentLimit int64, smoothedRTT, minRTT time.Duration,
	rttMultiplier float64, isLocalNetwork, isRemoteNetwork bool) (newLimit int64, exitSlowStart bool) {

	newLimit = currentLimit
	exitSlowStart = false

	if isLocalNetwork {
		newLimit = currentLimit * 3
		if smoothedRTT > time.Duration(float64(minRTT)*1.5) && t.framesSent > 10 {
			exitSlowStart = true
			newLimit = currentLimit * 2
		}
	} else if isRemoteNetwork {
		newLimit = currentLimit * 2
		if smoothedRTT > time.Duration(float64(minRTT)*3) && t.framesSent > 30 {
			exitSlowStart = true
			newLimit = currentLimit
		}
	} else {
		newLimit = currentLimit * 2
		multiplier := time.Duration(int64(2 * rttMultiplier))
		if smoothedRTT > minRTT*multiplier && t.framesSent > 20 {
			exitSlowStart = true
			newLimit = currentLimit
		}
	}

	return newLimit, exitSlowStart
}

func (t *Transfer) classifyWorkerType(isLocalNetwork, isRemoteNetwork bool) (isSlowWorker, isFastWorker bool) {
	if t.framesSent <= 20 {
		return false, false
	}

	// Calculate expected throughput based on network type
	var expectedThroughput float64
	if isLocalNetwork {
		expectedThroughput = 500 * 1024 // 500KB/s as a reference point
	} else if isRemoteNetwork {
		expectedThroughput = 200 * 1024 // 200KB/s as a reference point
	} else {
		expectedThroughput = 300 * 1024 // 300KB/s as a reference point
	}

	isSlowWorker = t.currentThroughput < expectedThroughput*0.5
	isFastWorker = t.currentThroughput > expectedThroughput*1.5

	return isSlowWorker, isFastWorker
}

func (t *Transfer) adjustLimitForLocalNetwork(currentLimit int64, rttVar, smoothedRTT, minRTT time.Duration,
	isSlowWorker bool) int64 {

	newLimit := currentLimit

	if isSlowWorker {
		if rttVar < smoothedRTT/4 {
			newLimit = currentLimit + (currentLimit / 2)
		} else if rttVar < smoothedRTT/2 {
			newLimit = currentLimit + (currentLimit / 3)
		} else {
			newLimit = currentLimit + (currentLimit / 5)
		}
	} else { // Normal or fast worker
		if rttVar < smoothedRTT/4 {
			newLimit = currentLimit + (currentLimit / 4)
		} else if rttVar < smoothedRTT/2 {
			newLimit = currentLimit + (currentLimit / 6)
		} else {
			newLimit = currentLimit + (currentLimit / 10)
		}
	}

	if smoothedRTT > time.Duration(float64(minRTT)*2) {
		if isSlowWorker {
			newLimit = currentLimit * 4 / 5
		} else {
			newLimit = currentLimit * 3 / 4
		}
		if newLimit < 1 {
			newLimit = 1
		}
	}

	return newLimit
}

func (t *Transfer) adjustLimitForRemoteNetwork(currentLimit int64, rttVar, smoothedRTT, minRTT time.Duration,
	isSlowWorker bool) int64 {

	newLimit := currentLimit

	if isSlowWorker {
		if rttVar < smoothedRTT/8 {
			newLimit = currentLimit + (currentLimit / 8)
		} else if rttVar < smoothedRTT/4 {
			newLimit = currentLimit + (currentLimit / 12)
		} else {
			newLimit = currentLimit + (currentLimit / 20)
		}
	} else { // Normal or fast worker
		if rttVar < smoothedRTT/8 {
			newLimit = currentLimit + (currentLimit / 10)
		} else if rttVar < smoothedRTT/4 {
			newLimit = currentLimit + (currentLimit / 16)
		} else {
			newLimit = currentLimit + (currentLimit / 32)
		}
	}

	if smoothedRTT > time.Duration(float64(minRTT)*2) {
		if isSlowWorker {
			newLimit = currentLimit * 3 / 5
		} else {
			newLimit = currentLimit / 2
		}
		if newLimit < 1 {
			newLimit = 1
		}
	}

	return newLimit
}

func (t *Transfer) adjustLimitForUnknownNetwork(currentLimit int64, rttVar, smoothedRTT, minRTT time.Duration,
	rttMultiplier float64, isSlowWorker, isFastWorker bool) int64 {

	newLimit := currentLimit

	adjustedMultiplier := rttMultiplier
	if isSlowWorker {
		adjustedMultiplier *= 0.8
	} else if isFastWorker {
		adjustedMultiplier *= 1.2
	}

	if rttVar < smoothedRTT/4 {
		newLimit = currentLimit + int64(float64(currentLimit)/(6*adjustedMultiplier))
	} else {
		newLimit = currentLimit + int64(float64(currentLimit)/(12*adjustedMultiplier))
	}

	if smoothedRTT > time.Duration(float64(minRTT)*3) {
		backoffFactor := 2.0 * adjustedMultiplier
		if isSlowWorker {
			backoffFactor *= 0.8 // Less aggressive backoff for slow workers
		}
		newLimit = int64(float64(currentLimit) / backoffFactor)
		if newLimit < 1 {
			newLimit = 1
		}
	}

	return newLimit
}

func (t *Transfer) updateCongestionControl(now time.Time) {
	if now.Sub(t.lastCongestionAdj) <= 100*time.Millisecond {
		return
	}

	smoothedRTT := t.rttStats.GetSmoothedRTT()
	rttVar := t.rttStats.GetRTTVariation()
	minRTT := t.rttStats.GetMinRTT()
	rttMultiplier := t.rttStats.GetAdaptiveRTTMultiplier()
	isLocalNetwork := t.rttStats.IsLocalNetwork()
	isRemoteNetwork := t.rttStats.IsRemoteNetwork()

	currentLimit := t.limiter.LimitNum()
	var newLimit int64 = currentLimit

	if t.inSlowStart {
		newLimit, t.inSlowStart = t.adjustLimitInSlowStart(
			currentLimit, smoothedRTT, minRTT, rttMultiplier, isLocalNetwork, isRemoteNetwork)
	} else {
		isSlowWorker, isFastWorker := t.classifyWorkerType(isLocalNetwork, isRemoteNetwork)

		if isLocalNetwork {
			newLimit = t.adjustLimitForLocalNetwork(currentLimit, rttVar, smoothedRTT, minRTT, isSlowWorker)
		} else if isRemoteNetwork {
			newLimit = t.adjustLimitForRemoteNetwork(currentLimit, rttVar, smoothedRTT, minRTT, isSlowWorker)
		} else {
			newLimit = t.adjustLimitForUnknownNetwork(
				currentLimit, rttVar, smoothedRTT, minRTT, rttMultiplier, isSlowWorker, isFastWorker)
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

func (t *Transfer) updateThroughputMetrics(now time.Time) {
	if t.framesSent%10 != 0 && now.Sub(t.lastMetricTime) <= time.Second {
		return
	}

	elapsedSeconds := now.Sub(t.lastMetricTime).Seconds()
	if elapsedSeconds <= 0 {
		return
	}

	// Calculate bytes per second
	t.currentThroughput = float64(t.bytesTransferred) / elapsedSeconds

	t.updateCongestionControl(now)

	t.lastMetricTime = now
	t.bytesTransferred = 0
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

			now := time.Now()
			t.updateThroughputMetrics(now)

			delete(t.waitAcks, ack.FrameID)
		}
		t.mu.Unlock()

		if ok {
			t.limiter.Release()
		}

		t.ackCh <- ack
	}
}
