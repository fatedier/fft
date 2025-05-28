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
	currentThroughput float64  // bytes per second
	
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
		id:               id,
		maxBufferCount:   maxBufferCount,
		inSlowStart:      true,
		waitAcks:         make(map[uint32]*SendFrame),
		framesSent:       0,
		bytesTransferred: 0,
		startTime:        now,
		lastMetricTime:   now,
		currentThroughput: 0,
		s:                s,
		limiter:          limit.NewLimiter(int64(1)),
		frameCh:          frameCh,
		ackCh:            ackCh,
		sendShutdown:     shutdown.New(),
		recvShutdown:     shutdown.New(),
	}
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
				n++
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
			go func() {
				t.frameCh <- sf
			}()
			continue
		}

		t.mu.Lock()
		t.waitAcks[sf.FrameID()] = sf
		t.mu.Unlock()

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
