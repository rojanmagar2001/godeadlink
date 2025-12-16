package app

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/rojanmagar2001/godeadlink/internal/check"
	"github.com/rojanmagar2001/godeadlink/internal/extract"
)

type Config struct {
	StartURL  string
	Timeout   time.Duration
	HeadFirst bool
	UserAgent string
}

// Run executes the Stage-1 pipeline:
// fetch start page -> extract <a href> -> check links -> print dead links.
func Run(ctx context.Context, cfg Config, stdout, stderr io.Writer) error {
	if cfg.StartURL == "" {
		return fmt.Errorf("start url is required")
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Second
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = "deadlink-learning-bot/0.1"
	}

	// Start page fetch gets its own timeout budget.
	pageCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(pageCtx, http.MethodGet, cfg.StartURL, nil)
	if err != nil {
		return fmt.Errorf("build start request: %w", err)
	}
	req.Header.Set("User-Agent", cfg.UserAgent)

	client := &http.Client{Timeout: cfg.Timeout}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("fetch start page: %w", err)
	}
	defer resp.Body.Close()

	links, err := extract.ExtractLinks(cfg.StartURL, resp.Body)
	if err != nil {
		return fmt.Errorf("extract links: %w", err)
	}

	chk := check.NewChecker(cfg.Timeout, cfg.HeadFirst)

	deadCount := 0
	for _, link := range links {
		// Each link check gets its own timeout budget (avoids “context deadline exceeded” cascade).
		linkCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
		res := chk.Check(linkCtx, link)
		cancel()

		if res.IsDead() {
			deadCount++
			if res.Err != nil {
				fmt.Fprintf(stdout, "DEAD  %-5s  %s\n", "ERR", res.URL)
				fmt.Fprintf(stdout, "      %v\n", res.Err)
			} else {
				fmt.Fprintf(stdout, "DEAD  %-5d  %s\n", res.StatusCode, res.URL)
			}
		}
	}

	fmt.Fprintf(stdout, "\nChecked %d links. Dead: %d\n", len(links), deadCount)
	return nil
}
