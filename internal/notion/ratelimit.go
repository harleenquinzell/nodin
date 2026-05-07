package notion

import (
	"context"

	"golang.org/x/time/rate"
)

type rateLimiter struct {
	l *rate.Limiter
}

func newRateLimiter(rps int) *rateLimiter {
	return &rateLimiter{l: rate.NewLimiter(rate.Limit(rps), rps)}
}

func (r *rateLimiter) Wait(ctx context.Context) error {
	return r.l.Wait(ctx)
}
