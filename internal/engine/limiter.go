package engine

import (
	"context"
	"time"

	"github.com/juju/ratelimit"
)

// RateLimiter управляет интенсивностью генерации запросов (RPS).
// Реализован на основе алгоритма Token Bucket.
type RateLimiter struct {
	bucket *ratelimit.Bucket
}

func NewRateLimiter(rps int) *RateLimiter {
	return &RateLimiter{
		bucket: ratelimit.NewBucketWithRate(float64(rps), int64(rps)),
	}
}

// Wait блокирует выполнение до получения разрешения от лимитера или отмены контекста.
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

// GetBucket вовзращает объект bucket структуры RateLimiter
func (l *RateLimiter) GetBucket() *ratelimit.Bucket {
	return l.bucket
}

func (l *RateLimiter) Drain() {
	l.bucket.TakeAvailable(l.GetBucket().Available())
}
