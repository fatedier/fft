package worker

import (
	"math"
	"sync/atomic"
	"time"
)

type TrafficLimiter struct {
	count          uint64
	maxCountPerDay uint64

	exceedCh            chan struct{}
	exceedLimitCallback func()
	restoreCallback     func()
}

func NewTrafficLimiter(maxCountPerDay uint64, exceedLimitCallback func(), restoreCallback func()) *TrafficLimiter {
	if maxCountPerDay == 0 {
		maxCountPerDay = math.MaxUint64
	}

	return &TrafficLimiter{
		count:          0,
		maxCountPerDay: maxCountPerDay,

		exceedCh:            make(chan struct{}),
		exceedLimitCallback: exceedLimitCallback,
		restoreCallback:     restoreCallback,
	}
}

func (tl *TrafficLimiter) AddCount(count uint64) {
	newCount := atomic.AddUint64(&tl.count, count)
	if newCount-count < tl.maxCountPerDay && newCount >= tl.maxCountPerDay {
		tl.exceedCh <- struct{}{}
	}
}

func (tl *TrafficLimiter) Run() {
	go tl.restoreWorker()
}

// change count to 0 every day
func (tl *TrafficLimiter) restoreWorker() {
	now := time.Now()
	lastRestoreTime := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	exceed := false

	for {
		select {
		case <-tl.exceedCh:
			exceed = true
			tl.exceedLimitCallback()
		case now := <-time.After(5 * time.Second):
			nowDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
			days := int(nowDay.Sub(lastRestoreTime).Hours() / 24)
			if days > 0 {
				atomic.StoreUint64(&tl.count, 0)
				lastRestoreTime = nowDay
				if exceed {
					exceed = false
					tl.restoreCallback()
				}
			}
		}
	}
}
