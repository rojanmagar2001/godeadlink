package check

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/rojanmagar2001/godeadlink/internal/model"
)

type Checker struct {
	Client      *http.Client
	HeadFirst   bool
	MaxBodyRead int64
}

func NewChecker(timeout time.Duration, headFirst bool) *Checker {
	return &Checker{
		Client: &http.Client{
			Timeout: timeout,
		},
		HeadFirst:   headFirst,
		MaxBodyRead: 1 << 20, // 1MB safety cap
	}
}

func (c *Checker) Check(ctx context.Context, link string) model.Result {
	// Try HEAD first if enabled
	if c.HeadFirst {
		res := c.do(ctx, http.MethodHead, link)
		// Some servers reject HEAD; fall back to GET
		if res.Err == nil && (res.StatusCode == http.StatusMethodNotAllowed || res.StatusCode == http.StatusBadRequest) {
			res = c.do(ctx, http.MethodGet, link)
		}
		if res.Err != nil {
			// If HEAD failed due to a method/specific issue, try GET once.
			// Otherwise keep the error
			var he *http.ProtocolError
			if errors.As(res.Err, &he) {
				return c.do(ctx, http.MethodGet, link)
			}
		}
		return res
	}

	return c.do(ctx, http.MethodGet, link)
}

func (c *Checker) do(ctx context.Context, method, link string) model.Result {
	req, err := http.NewRequestWithContext(ctx, method, link, nil)
	if err != nil {
		return model.Result{URL: link, Err: fmt.Errorf("new request: %w", err), Elapsed: 0}
	}
	req.Header.Set("User-Agent", "deadlink-learning-bot/0.1")

	start := time.Now()
	resp, err := c.Client.Do(req)
	elapsed := time.Since(start)

	if err != nil {
		return model.Result{URL: link, Err: fmt.Errorf("%s request: %w", method, err), Elapsed: elapsed}
	}
	defer resp.Body.Close()

	// Drain a little body on GET to avoid some servers misbehaving / keepalive issues.
	if method == http.MethodGet {
		_, _ = io.CopyN(io.Discard, resp.Body, c.MaxBodyRead)
	}

	return model.Result{URL: link, StatusCode: resp.StatusCode, Err: nil, Elapsed: elapsed}
}
