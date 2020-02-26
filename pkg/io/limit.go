package io

import (
	"context"
	"io"

	"golang.org/x/time/rate"
)

type RateReader struct {
	underlying io.Reader
	limiter    *rate.Limiter
}

func NewRateReader(r io.Reader, limiter *rate.Limiter) *RateReader {
	return &RateReader{
		underlying: r,
		limiter:    limiter,
	}
}

func (rr *RateReader) Read(p []byte) (n int, err error) {
	burst := rr.limiter.Burst()
	if len(p) > burst {
		p = p[:burst]
	}

	n, err = rr.underlying.Read(p)
	if err != nil {
		return
	}

	err = rr.limiter.WaitN(context.Background(), n)
	if err != nil {
		return
	}
	return
}
