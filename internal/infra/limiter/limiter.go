package limiter

import (
	"context"
	"net/url"
	"sync"
	"time"

	"github.com/rojanmagar2001/godeadlink/internal/ports"
)

// tokenBucket is a simple rate limiter using a buffered channel.
type tokenBucket struct {
	ch chan struct{}
}

func newTokenBucket(rate int) *tokenBucket {
	tb := &tokenBucket{
		ch: make(chan struct{}, rate),
	}

	// Fill tokens periodically
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		for range ticker.C {
			for i := 0; i < rate; i++ {
				select {
				case tb.ch <- struct{}{}:
				default:
					// bucket full
				}
			}
		}
	}()

	return tb
}

func (t *tokenBucket) Take(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.ch:
		return nil
	}
}

// hostLimiter manages per-host buckets
type PerHost struct {
	global *tokenBucket

	mu   sync.Mutex
	rate int
	host map[string]*tokenBucket
}

func New(globalRate, perHostRate int) ports.Limiter {
	if globalRate <= 0 {
		globalRate = 10
	}
	if perHostRate <= 0 {
		perHostRate = 2
	}
	return &PerHost{
		global: newTokenBucket(globalRate),
		rate:   perHostRate,
		host:   make(map[string]*tokenBucket),
	}
}

func (h *PerHost) Take(ctx context.Context, rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil // invalid URL already handled elsewhere
	}
	host := u.Hostname()
	if host == "" {
		return nil
	}

	h.mu.Lock()
	tb, ok := h.host[host]
	if !ok {
		tb = newTokenBucket(h.rate)
		h.host[host] = tb
	}
	h.mu.Unlock()

	return tb.Take(ctx)
}
