package sender

import (
	"math"
	"sync"
	"time"
)

type RTTStats struct {
	minRTT      time.Duration
	smoothedRTT time.Duration
	rttVar      time.Duration
	latestRTT   time.Duration
	samples     int
	mu          sync.Mutex
}

func NewRTTStats() *RTTStats {
	return &RTTStats{
		minRTT:      time.Hour, // Initialize to a large value
		smoothedRTT: 0,
		rttVar:      0,
		latestRTT:   0,
		samples:     0,
	}
}

func (r *RTTStats) UpdateRTT(sendTime time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()

	rtt := time.Since(sendTime)
	r.latestRTT = rtt

	if rtt < r.minRTT {
		r.minRTT = rtt
	}

	if r.samples == 0 {
		r.smoothedRTT = rtt
		r.rttVar = rtt / 2
	} else {
		rttDelta := time.Duration(math.Abs(float64(r.smoothedRTT - rtt)))
		r.rttVar = r.rttVar*3/4 + rttDelta/4

		r.smoothedRTT = r.smoothedRTT*7/8 + rtt/8
	}

	r.samples++
}

func (r *RTTStats) GetSmoothedRTT() time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	if r.samples == 0 {
		return time.Millisecond * 100 // Default value if no samples
	}
	return r.smoothedRTT
}

func (r *RTTStats) GetRTTVariation() time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	if r.samples == 0 {
		return time.Millisecond * 50 // Default value if no samples
	}
	return r.rttVar
}

func (r *RTTStats) GetMinRTT() time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	if r.minRTT == time.Hour {
		return time.Millisecond * 50 // Default value if no valid minimum
	}
	return r.minRTT
}

func (r *RTTStats) GetLatestRTT() time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.latestRTT
}
