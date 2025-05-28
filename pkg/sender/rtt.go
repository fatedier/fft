package sender

import (
	"math"
	"sync"
	"time"
)

type NetworkEnvironmentType int

const (
	NetworkEnvironmentUnknown NetworkEnvironmentType = iota
	NetworkEnvironmentLocal
	NetworkEnvironmentRemote
)

var RTTThresholds = struct {
	LocalThreshold  time.Duration
	RemoteThreshold time.Duration
}{
	LocalThreshold:  5 * time.Millisecond,
	RemoteThreshold: 50 * time.Millisecond,
}

type RTTStats struct {
	minRTT              time.Duration
	smoothedRTT         time.Duration
	rttVar              time.Duration
	latestRTT           time.Duration
	samples             int
	networkEnvironment  NetworkEnvironmentType
	environmentDetected bool
	lastEnvCheck        time.Time
	mu                  sync.Mutex
}

func NewRTTStats() *RTTStats {
	return &RTTStats{
		minRTT:              time.Hour, // Initialize to a large value
		smoothedRTT:         0,
		rttVar:              0,
		latestRTT:           0,
		samples:             0,
		networkEnvironment:  NetworkEnvironmentUnknown,
		environmentDetected: false,
		lastEnvCheck:        time.Now(),
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

	if !r.environmentDetected || r.samples%10 == 0 {
		r.detectNetworkEnvironment()
	}
}

func (r *RTTStats) detectNetworkEnvironment() {
	if r.samples < 5 {
		return
	}

	if r.smoothedRTT <= RTTThresholds.LocalThreshold {
		r.networkEnvironment = NetworkEnvironmentLocal
	} else if r.smoothedRTT >= RTTThresholds.RemoteThreshold {
		r.networkEnvironment = NetworkEnvironmentRemote
	} else {
		if r.minRTT <= RTTThresholds.LocalThreshold*2 {
			r.networkEnvironment = NetworkEnvironmentLocal
		} else {
			r.networkEnvironment = NetworkEnvironmentRemote
		}
	}

	r.environmentDetected = true
	r.lastEnvCheck = time.Now()
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

func (r *RTTStats) GetNetworkEnvironment() NetworkEnvironmentType {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.networkEnvironment
}

func (r *RTTStats) IsLocalNetwork() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.networkEnvironment == NetworkEnvironmentLocal
}

func (r *RTTStats) IsRemoteNetwork() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.networkEnvironment == NetworkEnvironmentRemote
}

func (r *RTTStats) GetAdaptiveRTTMultiplier() float64 {
	r.mu.Lock()
	defer r.mu.Unlock()

	switch r.networkEnvironment {
	case NetworkEnvironmentLocal:
		return 0.5 // More aggressive for local networks
	case NetworkEnvironmentRemote:
		return 2.0 // More conservative for remote networks
	default:
		return 1.0 // Default multiplier
	}
}
