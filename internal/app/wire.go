package app

import (
	"context"
	"io"
	"time"

	"github.com/rojanmagar2001/godeadlink/internal/infra/extractor"
	"github.com/rojanmagar2001/godeadlink/internal/infra/httpclient"
	"github.com/rojanmagar2001/godeadlink/internal/infra/limiter"
	"github.com/rojanmagar2001/godeadlink/internal/infra/store"
	"github.com/rojanmagar2001/godeadlink/internal/usecase"
)

type Config struct {
	StartURL    string
	Timeout     time.Duration
	HeadFirst   bool
	Concurrency int
	UserAgent   string

	MaxDepth      int
	MaxPages      int
	AllowExternal bool
	CheckAssets   bool

	Rate        int
	PerHostRate int

	ProgressEvery time.Duration
}

func Run(ctx context.Context, cfg Config, stdout io.Writer) error {
	if cfg.UserAgent == "" {
		cfg.UserAgent = "deadlink-learning-bot/0.1"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Second
	}

	httpc := httpclient.New(cfg.Timeout)
	lim := limiter.New(cfg.Rate, cfg.PerHostRate)
	ext := extractor.New()
	st := store.NewMemory()

	crawler := usecase.NewCrawler(httpc, ext, lim, cfg.UserAgent, cfg.Timeout, cfg.MaxDepth, cfg.MaxPages, cfg.CheckAssets)
	checker := usecase.NewLinkChecker(cfg.Timeout, cfg.HeadFirst, lim)

	orch := usecase.NewOrchestrator(crawler, checker, st, cfg.AllowExternal, cfg.Concurrency, cfg.Timeout, cfg.ProgressEvery)
	return orch.Run(ctx, cfg.StartURL, stdout)
}
