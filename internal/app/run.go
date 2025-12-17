package app

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
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

	MaxDepth      int
	MaxPages      int
	AllowExternal bool

	CheckAssets   bool
	ProgressEvery time.Duration // e.g. 1s

	Rate        int
	PerHostRate int
}

// linkMeta tracks where a link was found + at what crawl depth it first appeared.
type linkMeta struct {
	URL            string
	FirstSeenDepth int
	Sources        map[string]struct{} // set of source page URLs
}

type pageJob struct {
	URL   string
	Depth int
}

type summary struct {
	CrawledPages int
	Discovered   int
	Checked      int
	SkippedExt   int

	OK        int
	DeadHTTP  int
	Redirects int
	Errors    int
}

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
	if cfg.MaxDepth < 0 {
		cfg.MaxDepth = 0
	}
	if cfg.MaxPages <= 0 {
		cfg.MaxPages = 200
	}
	if cfg.ProgressEvery <= 0 {
		cfg.ProgressEvery = 1 * time.Second
	}
	if !cfg.CheckAssets {
		cfg.CheckAssets = true
	}
	if cfg.Rate <= 0 {
		cfg.Rate = 10
	}
	if cfg.PerHostRate <= 0 {
		cfg.PerHostRate = 2
	}

	crawlProgress := newProgressLogger(cfg.ProgressEvery)

	start, err := url.Parse(cfg.StartURL)
	if err != nil {
		return fmt.Errorf("parse start url: %w", err)
	}
	startHost := strings.ToLower(start.Hostname())

	globalLimiter := newTokenBucket(cfg.Rate)
	hostLimiter := newHostLimiter(cfg.PerHostRate)

	// ------------------------------------------------------------
	// Stage 3: Crawl pages (same-host only) up to MaxDepth/MaxPages
	// ------------------------------------------------------------
	//
	// We perform crawling sequentially for now (simpler and easier to learn).
	// Concurrency is used for *link checking* (Stage 2 worker pool).
	//
	// We maintain:
	// - visited pages set (avoid loops)
	// - queue of page jobs (BFS-ish)
	// - link index: link URL -> metadata (sources, firstSeenDepth)
	//
	client := &http.Client{Timeout: cfg.Timeout}
	visitedPages := make(map[string]struct{})
	linkIndex := make(map[string]*linkMeta)

	queue := []pageJob{{URL: cfg.StartURL, Depth: 0}}

	crawledPages := 0

	skippedCounts := make(map[model.SkipReason]int)

	for len(queue) > 0 && crawledPages < cfg.MaxPages {
		job := queue[0]
		queue = queue[1:]

		// Depth limit: do not fetch pages deeper than MaxDepth.
		if job.Depth > cfg.MaxDepth {
			continue
		}

		// Deduplicate pages.
		pageURL := normalizeForKey(job.URL)
		if _, seen := visitedPages[pageURL]; seen {
			continue
		}
		visitedPages[pageURL] = struct{}{}
		crawledPages++

		// Global rate limit
		if err := globalLimiter.Take(ctx); err != nil {
			return err
		}

		// Per-host rate limit
		if err := hostLimiter.Take(ctx, job.URL); err != nil {
			return err
		}

		// Fetch page with its own timeout context.
		pageCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
		req, err := http.NewRequestWithContext(pageCtx, http.MethodGet, job.URL, nil)
		if err != nil {
			cancel()
			// If the page URL itself is malformed, record it as a “link” error with source unknown.
			recordLink(linkIndex, job.URL, job.URL, job.Depth)
			continue
		}
		req.Header.Set("User-Agent", cfg.UserAgent)

		resp, err := client.Do(req)
		if err != nil {
			cancel()
			// If fetching this page fails, we still record the page as a link found on itself,
			// so it will show up in results.
			recordLink(linkIndex, job.URL, job.URL, job.Depth)
			continue
		}

		// Only parse HTML pages. If not HTML, treat as leaf.
		ct := strings.ToLower(resp.Header.Get("Content-Type"))
		if !strings.Contains(ct, "text/html") && !strings.Contains(ct, "application/xhtml") {
			_ = resp.Body.Close()
			cancel()
			// Record page itself as a checkable link.
			recordLink(linkIndex, job.URL, job.URL, job.Depth)
			continue
		}

		// Extract <a href> links from this page.
		found, err := extract.ExtractLinks(job.URL, resp.Body)
		_ = resp.Body.Close()
		cancel()
		if err != nil {
			// If parsing fails, still record the page itself.
			recordLink(linkIndex, job.URL, job.URL, job.Depth)
			continue
		}

		// Record the page itself as a link (useful to catch broken pages too).
		recordLink(linkIndex, job.URL, job.URL, job.Depth)

		for _, fl := range found {
			// If skipped, record as discovered (optional) but don’t crawl/check.
			if fl.SkipReason != "" || fl.URL == "" {
				skippedCounts[fl.SkipReason]++
				// You can choose to record skipped links too, but it may be noisy.
				// For learning, we record them as discovered with a marker in sources later (Stage 8).
				continue
			}

			// If assets are disabled, skip asset links entirely
			if fl.Kind == model.LinkKindAsset && !cfg.CheckAssets {
				continue
			}

			link := fl.URL

			// Track source relationship: link was found on job.URL
			recordLink(linkIndex, link, job.URL, job.Depth)

			// Only crawl "page" links (anchors). Assets are checked but not crawled.
			if fl.Kind != model.LinkKindPage {
				continue
			}

			// Decide whether to enqueue this link as another page to crawl.
			u, err := url.Parse(link)
			if err != nil {
				continue
			}
			host := strings.ToLower(u.Hostname())

			// Crawl scope: same-host only.
			if host != "" && host != startHost {
				continue
			}

			// Only enqueue if we still have depth budget.
			if job.Depth < cfg.MaxDepth {
				queue = append(queue, pageJob{URL: link, Depth: job.Depth + 1})
			}
		}

		if crawlProgress.ShouldLog() {
			fmt.Fprintf(stdout,
				"[crawl] pages=%d queue=%d discoveredLinks=%d\n",
				crawledPages,
				len(queue),
				len(linkIndex),
			)
		}
	}

	// ------------------------------------------------------------
	// Build the set of links we will actually check
	// ------------------------------------------------------------
	linksToCheck := make([]*linkMeta, 0, len(linkIndex))
	skippedExternal := 0

	for _, meta := range linkIndex {
		u, err := url.Parse(meta.URL)
		if err != nil {
			// Still check malformed URLs via the checker; it will return error.
			linksToCheck = append(linksToCheck, meta)
			continue
		}

		host := strings.ToLower(u.Hostname())
		isExternal := host != "" && host != startHost

		// If we’re not allowing external checks, skip them (but keep them discovered).
		if isExternal && !cfg.AllowExternal {
			skippedCounts[model.SkipExternal]++
			skippedExternal++
			continue
		}
		linksToCheck = append(linksToCheck, meta)
	}

	// Stable ordering of checks (helps consistent testability and output).
	sort.Slice(linksToCheck, func(i, j int) bool {
		return linksToCheck[i].URL < linksToCheck[j].URL
	})

	// ------------------------------------------------------------
	// Stage 2 worker pool: Concurrently check links
	// ------------------------------------------------------------
	chk := check.NewChecker(cfg.Timeout, cfg.HeadFirst)

	jobs := make(chan *linkMeta)
	results := make(chan model.Result, cfg.Concurrency)

	var wg sync.WaitGroup

	worker := func() {
		defer wg.Done()

		for meta := range jobs {
			// Global rate limit
			if err := globalLimiter.Take(ctx); err != nil {
				return
			}

			// Per-host rate limit
			if err := hostLimiter.Take(ctx, meta.URL); err != nil {
				return
			}
			// Per-link timeout context. Critical for preventing cascading timeouts.
			linkCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
			res := chk.Check(linkCtx, meta.URL)
			cancel()

			results <- res
		}
	}

	wg.Add(cfg.Concurrency)
	for i := 0; i < cfg.Concurrency; i++ {
		go worker()
	}

	go func() {
		for _, meta := range linksToCheck {
			jobs <- meta
		}
		close(jobs)
	}()

	go func() {
		wg.Wait()
		close(results)
	}()

	checkProgress := newProgressLogger(cfg.ProgressEvery)
	checked := 0
	dead := 0
	errors := 0

	// Collect results into slice so we can sort and print stably.
	all := make([]model.Result, 0, len(linksToCheck))
	for r := range results {
		all = append(all, r)
		checked++

		if r.Err != nil {
			errors++
		} else if r.StatusCode >= 400 {
			dead++
		}

		if checkProgress.ShouldLog() {
			fmt.Fprintf(stdout,
				"[check] checked=%d/%d dead=%d errors=%d\n",
				checked,
				len(linksToCheck),
				dead,
				errors,
			)
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].URL < all[j].URL })

	// Summarize statuses.
	s := summarize(all)
	s.CrawledPages = crawledPages
	s.Discovered = len(linkIndex)
	s.Checked = len(linksToCheck)
	s.SkippedExt = skippedExternal

	// Print dead items with sources.
	// (We print sources to satisfy Stage 3 requirement: link→source tracking.)
	for _, r := range all {
		if r.IsDead() {
			meta := linkIndex[r.URL]
			srcList := sourcesAsSortedList(meta)

			if r.Err != nil {
				fmt.Fprintf(stdout, "DEAD  %-5s  %s\n", "ERR", r.URL)
				fmt.Fprintf(stdout, "      %v\n", r.Err)
			} else {
				fmt.Fprintf(stdout, "DEAD  %-5d  %s\n", r.StatusCode, r.URL)
			}

			// Print where it was found.
			if len(srcList) > 0 {
				// Show first source + count (avoid huge spam, but still informative).
				if len(srcList) == 1 {
					fmt.Fprintf(stdout, "      found on: %s\n", srcList[0])
				} else {
					fmt.Fprintf(stdout, "      found on: %s (+%d more)\n", srcList[0], len(srcList)-1)
				}
			}
		}
	}

	fmt.Fprintf(stdout,
		"\nCrawled pages: %d (max-pages=%d, max-depth=%d)\nDiscovered links: %d\nChecked links: %d\nSkipped external: %d (allow-external=%v)\nOK: %d  Redirects: %d  DeadHTTP: %d  Errors: %d\n",
		s.CrawledPages, cfg.MaxPages, cfg.MaxDepth,
		s.Discovered,
		s.Checked,
		s.SkippedExt, cfg.AllowExternal,
		s.OK, s.Redirects, s.DeadHTTP, s.Errors,
	)

	if len(skippedCounts) > 0 {
		fmt.Fprintln(stdout, "\nSkipped links:")
		keys := make([]string, 0, len(skippedCounts))
		for k := range skippedCounts {
			keys = append(keys, string(k))
		}
		sort.Strings(keys)

		for _, k := range keys {
			fmt.Fprintf(stdout, "  %-20s %d\n", k+":", skippedCounts[model.SkipReason(k)])
		}
	}

	_ = stderr // reserved for later structured logging/warnings
	return nil
}

// recordLink updates (or creates) linkMeta for a discovered link and tracks the source page.
func recordLink(index map[string]*linkMeta, linkURL, sourcePage string, depth int) {
	key := normalizeForKey(linkURL)

	m, ok := index[key]
	if !ok {
		m = &linkMeta{
			URL:            key,
			FirstSeenDepth: depth,
			Sources:        make(map[string]struct{}),
		}
		index[key] = m
	}

	// Keep earliest depth seen.
	if depth < m.FirstSeenDepth {
		m.FirstSeenDepth = depth
	}

	if sourcePage != "" {
		m.Sources[normalizeForKey(sourcePage)] = struct{}{}
	}
}

// normalizeForKey is a small normalization to improve deduping:
// - strip fragment
// - lowercase hostname
// (More robust normalization will come later.)
func normalizeForKey(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	u.Fragment = ""
	if u.Host != "" {
		// url.URL doesn't have Hostname setter, so normalize via Host field.
		// Keep port if present.
		host := strings.ToLower(u.Hostname())
		if port := u.Port(); port != "" {
			u.Host = host + ":" + port
		} else {
			u.Host = host
		}
	}
	return u.String()
}

func sourcesAsSortedList(meta *linkMeta) []string {
	if meta == nil || len(meta.Sources) == 0 {
		return nil
	}
	out := make([]string, 0, len(meta.Sources))
	for s := range meta.Sources {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func summarize(all []model.Result) summary {
	var s summary
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
		}
	}
	return s
}

type progressLogger struct {
	last  time.Time
	every time.Duration
}

func newProgressLogger(every time.Duration) *progressLogger {
	if every <= 0 {
		every = time.Second
	}
	return &progressLogger{
		last:  time.Now(),
		every: every,
	}
}

func (p *progressLogger) ShouldLog() bool {
	now := time.Now()
	if now.Sub(p.last) >= p.every {
		p.last = now
		return true
	}
	return false
}
