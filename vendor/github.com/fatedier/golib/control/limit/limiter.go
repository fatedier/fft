package limit

import (
	"errors"
	"sync/atomic"
	"time"

	gerr "github.com/fatedier/golib/errors"
)

var (
	ErrTimeout = errors.New("limiter acquire timeout")
	ErrClosed  = errors.New("limiter closed")
)

// Limiter support update limit number dynamically
type Limiter struct {
	poolCh    chan struct{}
	releaseCh chan struct{}
	limitCh   chan int64

	current int64
	waiting int64
	limit   int64
	closed  int64
}

func NewLimiter(initLimit int64) (l *Limiter) {
	l = &Limiter{
		poolCh:    make(chan struct{}),
		releaseCh: make(chan struct{}),
		limitCh:   make(chan int64),
		limit:     initLimit,
	}
	go l.manager()
	return
}

func (l *Limiter) manager() {
	var err error
	for {
		if l.current < l.limit {
			err = gerr.PanicToError(func() {
				select {
				case l.poolCh <- struct{}{}:
					atomic.AddInt64(&l.current, 1)
				case <-l.releaseCh:
					if l.current > 0 {
						atomic.AddInt64(&l.current, -1)
					}
				case newLimit := <-l.limitCh:
					atomic.StoreInt64(&l.limit, newLimit)
				}
			})
			if err != nil {
				// closed
				close(l.releaseCh)
				close(l.limitCh)
				break
			}
			continue
		}

		select {
		case <-l.releaseCh:
			atomic.AddInt64(&l.current, -1)
		case newLimit := <-l.limitCh:
			atomic.StoreInt64(&l.limit, newLimit)
		}
	}
}

func (l *Limiter) LimitNum() int64 {
	return atomic.LoadInt64(&l.limit)
}

func (l *Limiter) RunningNum() int64 {
	return atomic.LoadInt64(&l.current)
}

func (l *Limiter) WaitingNum() int64 {
	return atomic.LoadInt64(&l.waiting)
}

// Acquire will wait for an available resource.
// timeout eq 0 means no timeout limit.
// Return ErrTimeout if no resource available after timeout duration.
// Return ErrClosed if this Limiter is closed.
func (l *Limiter) Acquire(timeout time.Duration) (err error) {
	atomic.AddInt64(&l.waiting, 1)

	defer func() {
		atomic.AddInt64(&l.waiting, -1)
	}()

	if timeout == 0 {
		// no timeout limit
		select {
		case _, ok := <-l.poolCh:
			if !ok {
				err = ErrClosed
			}
		}
	} else {
		select {
		case <-time.After(timeout):
			err = ErrTimeout
		case _, ok := <-l.poolCh:
			if !ok {
				err = ErrClosed
			}
		}
	}
	return
}

// Release resources.
func (l *Limiter) Release() {
	if err := gerr.PanicToError(func() {
		l.releaseCh <- struct{}{}
	}); err != nil {
		atomic.AddInt64(&l.current, -1)
	}

}

func (l *Limiter) SetLimit(num int64) {
	gerr.PanicToError(func() {
		l.limitCh <- num
	})
}

func (l *Limiter) Close() {
	closed := !atomic.CompareAndSwapInt64(&l.closed, 0, 1)
	if !closed {
		close(l.poolCh)
	}
}
