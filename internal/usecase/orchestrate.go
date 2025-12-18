package usecase

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rojanmagar2001/godeadlink/internal/domain"
	"github.com/rojanmagar2001/godeadlink/internal/ports"
)

type Orchestrator struct {
	crawler *Crawler
	checker *LinkCheckerService
	store   ports.Store

	allowExternal bool
	concurrency   int
	timeout       time.Duration
	progressEvery time.Duration
}

type Config struct {
	StartURL      string
	AllowExternal bool
	Concurrency   int
	ProgressEvery time.Duration
}

func NewOrchestrator(c *Crawler, chk *LinkCheckerService, st ports.Store, allowExternal bool, concurrency int, timeout, progressEvery time.Duration) *Orchestrator {
	if concurrency <= 0 {
		concurrency = 20
	}
	if progressEvery <= 0 {
		progressEvery = time.Second
	}

	return &Orchestrator{
		crawler:       c,
		checker:       chk,
		store:         st,
		allowExternal: allowExternal,
		concurrency:   concurrency,
		timeout:       timeout,
		progressEvery: progressEvery,
	}
}

func (o *Orchestrator) Run(ctx context.Context, startURL string, stdout io.Writer) error {
	startHost, err := o.crawler.Crawl(ctx, startURL, o.store)
	if err != nil {
		return err
	}

	discovered := o.store.AllDiscovered()

	// Decide what to check (skip externals unless allowed; skip skipped entries)
	toCheck := make([]*domain.LinkMeta, 0, len(discovered))
	skippedCounts := map[domain.SkipReason]int{}

	for _, m := range discovered {
		if m.Skipped != "" {
			skippedCounts[m.Skipped]++
			continue
		}

		u, err := url.Parse(m.URL)
		if err != nil {
			toCheck = append(toCheck, m)
			continue
		}

		host := strings.ToLower(u.Hostname())
		isExternal := host != "" && host != startHost
		if isExternal && !o.allowExternal {
			skippedCounts[domain.SkipExternal]++
			continue
		}
		toCheck = append(toCheck, m)
	}

	sort.Slice(toCheck, func(i, j int) bool { return toCheck[i].URL < toCheck[j].URL })

	// Worker pool
	jobs := make(chan *domain.LinkMeta)
	results := make(chan domain.Result, o.concurrency)

	var wg sync.WaitGroup
	worker := func() {
		defer wg.Done()
		for m := range jobs {
			results <- o.checker.Check(ctx, m.URL)
		}
	}

	wg.Add(o.concurrency)
	for i := 0; i < o.concurrency; i++ {
		go worker()
	}

	go func() {
		for _, m := range toCheck {
			jobs <- m
		}
		close(jobs)
	}()

	// collect
	all := make([]domain.Result, 0, len(toCheck))
	for r := range results {
		all = append(all, r)
	}

	sort.Slice(all, func(i, j int) bool { return all[i].URL < all[j].URL })
	for _, r := range all {
		if r.IsDead() {
			fmt.Fprintf(stdout, "DEAD %-5s %s\n", codeOrErr(r), r.URL)
			if r.Err != nil {
				fmt.Fprintf(stdout, "      %v\n", r.Err)
			}

			// Find sources (store already has meta keyed by normalized URL).
			// For simplicity, scan discovered list here (O(n)). We'll optimize later if needed.
			if src := firstSourceFor(discovered, r.URL); src != "" {
				fmt.Fprintf(stdout, "       found on : %s\n", src)
			}
		}
	}

	// summary
	ok, redir, deadHTTP, errs := summarize(all)
	fmt.Fprintf(stdout,
		"\nCrawled pages: %d (max-pages=%d, max-depth=%d)\nDiscovered links: %d\nChecked links: %d\nOK: %d  Redirects: %d  DeadHTTP: %d  Errors: %d\n",
		o.store.VisitedCount(), len(discovered), len(toCheck),
		ok, redir, deadHTTP, errs,
	)

	if len(skippedCounts) > 0 {
		fmt.Println(stdout, "\nSkipped links:")
		keys := make([]string, 0, len(skippedCounts))
		for k := range skippedCounts {
			keys = append(keys, string(k))
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(stdout, "  %-20s %d\n", k+":", skippedCounts[domain.SkipReason(k)])
		}
	}

	return nil
}

func codeOrErr(r domain.Result) string {
	if r.Err != nil {
		return "ERR"
	}
	return fmt.Sprintf("%d", r.StatusCode)
}

func summarize(all []domain.Result) (ok, redir, deadHTTP, errs int) {
	for _, r := range all {
		if r.Err != nil {
			errs++
			continue
		}
		switch {
		case r.StatusCode >= 200 && r.StatusCode <= 299:
			ok++
		case r.StatusCode >= 300 && r.StatusCode <= 399:
			redir++
		case r.StatusCode >= 400:
			deadHTTP++
		}
	}
	return
}

func firstSourceFor(discovered []*domain.LinkMeta, url string) string {
	for _, m := range discovered {
		if m.URL == url {
			for s := range m.Sources {
				return s
			}
		}
	}
	return ""
}
