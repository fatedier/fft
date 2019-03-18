package receiver

import (
	"bytes"
	"io"
	"sort"
	"sync"

	"github.com/fatedier/fft/pkg/stream"
)

type Receiver struct {
	fileID      uint32
	nextFrameID uint32
	dst         io.Writer
	frames      []*stream.Frame
	framesIDMap map[uint32]struct{}
	notifyCh    chan struct{}

	mu sync.RWMutex
}

func NewReceiver(fileID uint32, dst io.Writer) *Receiver {
	return &Receiver{
		fileID:      fileID,
		nextFrameID: 0,
		dst:         dst,
		frames:      make([]*stream.Frame, 0),
		framesIDMap: make(map[uint32]struct{}),
		notifyCh:    make(chan struct{}, 1),
	}
}

func (r *Receiver) RecvFrame(frame *stream.Frame) {
	r.mu.Lock()
	if frame.FrameID < r.nextFrameID {
		r.mu.Unlock()
		return
	}

	if _, ok := r.framesIDMap[frame.FrameID]; ok {
		r.mu.Unlock()
		return
	}

	r.frames = append(r.frames, frame)
	r.framesIDMap[frame.FrameID] = struct{}{}
	sort.Slice(r.frames, func(i, j int) bool {
		return r.frames[i].FrameID < r.frames[j].FrameID
	})
	r.mu.Unlock()

	select {
	case r.notifyCh <- struct{}{}:
	default:
	}
}

func (r *Receiver) Run() {
	for {
		_, ok := <-r.notifyCh
		if !ok {
			return
		}

		buffer := bytes.NewBuffer(nil)
		ii := 0
		finished := false
		r.mu.Lock()
		for i, frame := range r.frames {
			if r.nextFrameID == frame.FrameID {
				ii = i + 1
				delete(r.framesIDMap, frame.FrameID)
				// it's last frame
				if len(frame.Buf) == 0 {
					finished = true
					break
				}

				buffer.Write(frame.Buf)
				r.nextFrameID++
			} else {
				ii = i
				break
			}
		}
		r.frames = r.frames[ii:]
		r.mu.Unlock()

		buf := buffer.Bytes()
		if len(buf) != 0 {
			r.dst.Write(buf)
		}

		if finished {
			break
		}
	}
}
