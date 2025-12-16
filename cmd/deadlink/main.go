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
		startURL  = flag.String("url", "", "Start URL (single page) e.g. https://example.com")
		timeout   = flag.Duration("timeout", 10*time.Second, "HTTP timeout (e.g. 10s)")
		headFirst = flag.Bool("head-first", true, "Try HEAD before GET (fallback to GET if needed)")
	)
	flag.Parse()

	cfg := app.Config{
		StartURL:  *startURL,
		Timeout:   *timeout,
		HeadFirst: *headFirst,
	}

	if err := app.Run(context.Background(), cfg, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
