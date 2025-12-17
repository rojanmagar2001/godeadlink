package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/rojanmagar2001/godeadlink/internal/app"
)

func main() {
	var (
		startURL      = flag.String("url", "", "Start URL (single page) e.g. https://example.com")
		timeout       = flag.Duration("timeout", 10*time.Second, "HTTP timeout (e.g. 10s)")
		headFirst     = flag.Bool("head-first", true, "Try HEAD before GET (fallback to GET if needed)")
		concurrency   = flag.Int("concurrency", 20, "Number of concurrent links checks")
		maxDepth      = flag.Int("max-depth", 2, "Max crawl depth (0 = only start page)")
		maxPages      = flag.Int("max-pages", 200, "Max number of pages to crawl")
		allowExternal = flag.Bool("allow-external", false, "Also check external links (default: false)")
		checkAssets   = flag.Bool("check-assets", true, "Check asset links (img, script, link)")
		rate          = flag.Int("rate", 10, "Global request rate (req/sec)")
		perHost       = flag.Int("per-host-rate", 2, "Per-host request rate (req/sec)")
	)
	flag.Parse()

	cfg := app.Config{
		StartURL:      *startURL,
		Timeout:       *timeout,
		HeadFirst:     *headFirst,
		Concurrency:   *concurrency,
		MaxDepth:      *maxDepth,
		MaxPages:      *maxPages,
		AllowExternal: *allowExternal,
		CheckAssets:   *checkAssets,
		Rate:          *rate,
		PerHostRate:   *perHost,
	}

	if err := app.Run(context.Background(), cfg, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
