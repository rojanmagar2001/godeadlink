package usecase

import (
	"context"
	"time"

	"github.com/rojanmagar2001/godeadlink/internal/check"
	"github.com/rojanmagar2001/godeadlink/internal/domain"
	"github.com/rojanmagar2001/godeadlink/internal/ports"
)

type LinkCheckerService struct {
	chk     *check.Checker
	limiter ports.Limiter
	timeout time.Duration
}

func NewLinkChecker(timeout time.Duration, headFirst bool, limiter ports.Limiter) *LinkCheckerService {
	return &LinkCheckerService{
		chk:     check.NewChecker(timeout, headFirst),
		limiter: limiter,
		timeout: timeout,
	}
}

func (s *LinkCheckerService) Check(ctx context.Context, url string) domain.Result {
	// Limiting happens before network call
	_ = s.limiter.Take(ctx, url)

	// Per-link timeout
	linkCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	r := s.chk.Check(linkCtx, url)
	return domain.Result{
		URL:        r.URL,
		StatusCode: r.StatusCode,
		Err:        r.Err,
		Elapsed:    r.Elapsed,
	}
}
