package usecase

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/rojanmagar2001/godeadlink/internal/domain"
	"github.com/rojanmagar2001/godeadlink/internal/ports"
)

type Crawler struct {
	client    ports.HTTPClient
	extractor ports.Extractor
	limiter   ports.Limiter

	userAgent string
	timeout   time.Duration

	maxDepth    int
	maxPages    int
	checkAssets bool
}

type PageJob struct {
	URL   string
	Depth int
}

func NewCrawler(
	client ports.HTTPClient,
	extractor ports.Extractor,
	limiter ports.Limiter,
	userAgent string,
	timeout time.Duration,
	maxDepth, maxPages int,
	checkAssets bool,
) *Crawler {
	return &Crawler{
		client:      client,
		extractor:   extractor,
		limiter:     limiter,
		userAgent:   userAgent,
		timeout:     timeout,
		maxDepth:    maxDepth,
		maxPages:    maxPages,
		checkAssets: checkAssets,
	}
}

func (c *Crawler) Crawl(ctx context.Context, startUrl string, store ports.Store) (startHost string, err error) {
	start, err := url.Parse(startUrl)
	if err != nil {
		return "", fmt.Errorf("parse start url: %w", err)
	}

	startHost = strings.ToLower(start.Hostname())

	queue := []PageJob{{URL: startUrl, Depth: 0}}
	crawled := 0

	for len(queue) > 0 && crawled < c.maxPages {
		job := queue[0]
		queue = queue[1:]

		if job.Depth > c.maxDepth {
			continue
		}

		if !store.MarkVisitedPage(job.URL) {
			continue
		}
		crawled++

		_ = c.limiter.Take(ctx, job.URL)

		pageCtx, cancel := context.WithTimeout(ctx, c.timeout)
		req, err := http.NewRequestWithContext(pageCtx, http.MethodGet, job.URL, nil)
		if err != nil {
			cancel()
			store.RecordDiscoveredLink(domain.LinkMeta{
				URL:            job.URL,
				FirstSeenDepth: job.Depth,
				Kind:           domain.LinkKindPage,
			}, job.URL)
			continue
		}
		req.Header.Set("User-Agent", c.userAgent)

		resp, err := c.client.Do(req)
		if err != nil {
			cancel()
			store.RecordDiscoveredLink(domain.LinkMeta{
				URL:            job.URL,
				FirstSeenDepth: job.Depth,
				Kind:           domain.LinkKindPage,
			}, job.URL)
			continue
		}

		ct := strings.ToLower(resp.Header.Get("Content-Type"))
		if !strings.Contains(ct, "text/html") && !strings.Contains(ct, "application/xhtml") {
			_ = resp.Body.Close()

			cancel()
			store.RecordDiscoveredLink(domain.LinkMeta{
				URL:            job.URL,
				FirstSeenDepth: job.Depth,
				Kind:           domain.LinkKindPage,
			}, job.URL)
			continue
		}

		found, exErr := c.extractor.Extract(job.URL, resp.Body)
		_ = resp.Body.Close()
		cancel()
		if exErr != nil {
			store.RecordDiscoveredLink(domain.LinkMeta{
				URL:            job.URL,
				FirstSeenDepth: job.Depth,
				Kind:           domain.LinkKindPage,
			}, job.URL)
			continue

		}

		store.RecordDiscoveredLink(domain.LinkMeta{
			URL:            job.URL,
			FirstSeenDepth: job.Depth,
			Kind:           domain.LinkKindPage,
		}, job.URL)

		for _, fl := range found {
			if fl.SkipReason != "" || fl.URL == "" {
				store.RecordDiscoveredLink(domain.LinkMeta{
					URL:            fl.Raw,
					FirstSeenDepth: job.Depth,
					Kind:           fl.Kind,
					Skipped:        fl.SkipReason,
				}, job.URL)
				continue
			}

			if fl.Kind == domain.LinkKindAsset && !c.checkAssets {
				continue
			}

			store.RecordDiscoveredLink(domain.LinkMeta{
				URL:            fl.URL,
				FirstSeenDepth: job.Depth,
				Kind:           fl.Kind,
			}, job.URL)

			// Only crawl page links (same host)
			if fl.Kind != domain.LinkKindPage {
				continue
			}
			u, err := url.Parse(fl.URL)
			if err != nil {
				continue
			}

			host := strings.ToLower(u.Hostname())
			if host != "" && host != startHost {
				continue
			}
			if job.Depth < c.maxDepth {
				queue = append(queue, PageJob{URL: fl.URL, Depth: job.Depth + 1})
			}
		}

	}

	return startHost, nil
}
