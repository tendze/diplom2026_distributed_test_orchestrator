package agent

import (
	"context"
	"time"

	"github.com/juju/ratelimit"
)

type RateLimiter struct {
	bucket *ratelimit.Bucket
}

func NewRateLimiter(rps int) *RateLimiter {
	return &RateLimiter{
		bucket: ratelimit.NewBucketWithRate(float64(rps), int64(rps)),
	}
}


func (l *RateLimiter) Wait(ctx context.Context) error {
	wd := l.bucket.Take(1)
	if wd <= 0 {
		return nil
	}

	timer := time.NewTimer(wd)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
