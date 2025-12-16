package app

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/rojanmagar2001/godeadlink/internal/check"
	"github.com/rojanmagar2001/godeadlink/internal/extract"
	"github.com/rojanmagar2001/godeadlink/internal/model"
)

type Config struct {
	StartURL    string
	Timeout     time.Duration
	HeadFirst   bool
	Concurrency int
	UserAgent   string
}

type summary struct {
	Checked   int
	OK        int
	DeadHTTP  int
	Redirects int
	Errors    int
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
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 20
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

	// ---------------------------------------------------------------------
	// Worker pool setup
	//
	// We want to check many links concurrently without spawning an
	// unbounded number of goroutines.
	//
	// Pattern used:
	//   - jobs channel: carries links to be checked
	//   - results channel: carries completed check results
	//   - fixed number of worker goroutines
	//   - WaitGroup to know when all workers are done
	// ---------------------------------------------------------------------
	chk := check.NewChecker(cfg.Timeout, cfg.HeadFirst)

	// jobs is an unbuffered channel of work items.
	// Each item is a single link (string) to check.
	jobs := make(chan string)

	// results carries completed link-check results.
	// We buffer it to the concurrency size so workers are not blocked
	// on send if the collector is momentarily slow.
	results := make(chan model.Result, cfg.Concurrency)

	var wg sync.WaitGroup

	// worker is the function run by each goroutine in the pool.
	// It continuously receives links from the jobs channel until
	// the channel is closed.
	worker := func() {
		// Ensure the WaitGroup counter is decremented when
		// this worker exits.
		defer wg.Done()
		for link := range jobs {
			// IMPORTANT:
			// We create a fresh timeout context *per link*.
			//
			// This avoids the bug where a single shared context
			// times out and causes all remaining work to fail.
			linkCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)

			// Perform the actual HTTP check.
			res := chk.Check(linkCtx, link)

			// Always cancel to free timers/resources associated
			// with the context.
			cancel()

			// Send the result to the collector.
			// This send will block only if the results buffer
			// is full and the collector is not keeping up.
			results <- res
		}
	}

	// ---------------------------------------------------------------------
	// Start worker goroutines
	// ---------------------------------------------------------------------

	// We start a fixed number of workers, equal to cfg.Concurrency.
	// This caps parallelism and protects both the target site
	// and our own machine.
	wg.Add(cfg.Concurrency)
	for i := 0; i < cfg.Concurrency; i++ {
		go worker()
	}

	// ---------------------------------------------------------------------
	// Feed jobs to the workers
	// ---------------------------------------------------------------------

	// This goroutine is responsible for sending all links
	// into the jobs channel.
	//
	// When it finishes sending, it closes the channel to signal
	// to workers that no more work is coming.
	go func() {
		for _, link := range links {
			jobs <- link
		}
		close(jobs)
	}()

	// ---------------------------------------------------------------------
	// Close results channel when all workers are done
	// ---------------------------------------------------------------------

	// We cannot close results immediately, because workers
	// may still be sending to it.
	//
	// Instead, we wait for all workers to finish, then close
	// results so the collector can exit its loop cleanly.
	go func() {
		wg.Wait()
		close(results)
	}()

	// ---------------------------------------------------------------------
	// Collect results
	// ---------------------------------------------------------------------

	// The main goroutine acts as the collector.
	// It ranges over results until the channel is closed.
	all := make([]model.Result, 0, len(links))
	for r := range results {
		all = append(all, r)
	}

	// Stable output: sort by URL before printing
	sort.Slice(all, func(i, j int) bool {
		return all[i].URL < all[j].URL
	})

	// Print dead links and summary
	s := summarize(all)
	for _, r := range all {
		// In Stage 1/2 we still print only "dead" items:
		// - HTTP >= 400
		// - any error (timeout/network/etc)
		if r.IsDead() {
			if r.Err != nil {
				fmt.Fprintf(stdout, "DEAD  %-5s  %s\n", "ERR", r.URL)
				fmt.Fprintf(stdout, "      %v\n", r.Err)
			} else {
				fmt.Fprintf(stdout, "DEAD  %-5d  %s\n", r.StatusCode, r.URL)
			}
		}
	}

	fmt.Fprintf(stdout, "\nChecked %d links. Dead: %d (HTTP: %d, Errors: %d). OK: %d. Redirects: %d\n",
		s.Checked, (s.DeadHTTP + s.Errors), s.DeadHTTP, s.Errors, s.OK, s.Redirects,
	)

	_ = stderr // kept for future stages (logging, warnings)

	// deadCount := 0
	// for _, link := range links {
	// 	// Each link check gets its own timeout budget (avoids “context deadline exceeded” cascade).
	// 	linkCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	// 	res := chk.Check(linkCtx, link)
	// 	cancel()
	//
	// 	if res.IsDead() {
	// 		deadCount++
	// 		if res.Err != nil {
	// 			fmt.Fprintf(stdout, "DEAD  %-5s  %s\n", "ERR", res.URL)
	// 			fmt.Fprintf(stdout, "      %v\n", res.Err)
	// 		} else {
	// 			fmt.Fprintf(stdout, "DEAD  %-5d  %s\n", res.StatusCode, res.URL)
	// 		}
	// 	}
	// }
	//
	// fmt.Fprintf(stdout, "\nChecked %d links. Dead: %d\n", len(links), deadCount)
	return nil
}

func summarize(all []model.Result) summary {
	var s summary
	s.Checked = len(all)
	for _, r := range all {
		if r.Err != nil {
			s.Errors++
			continue
		}
		switch {
		case r.StatusCode >= 200 && r.StatusCode <= 299:
			s.OK++
		case r.StatusCode >= 300 && r.StatusCode <= 399:
			s.Redirects++
		case r.StatusCode >= 400:
			s.DeadHTTP++
		default:
			// status code 0 etc shouldn't happen without error, but keep safe
		}
	}
	return s
}
