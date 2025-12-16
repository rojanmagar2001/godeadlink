package app

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRun_CrawlDepthAndSources(t *testing.T) {
	mux := http.NewServeMux()

	// / links to /p1 and external
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`
			<html><body>
				<a href="/p1">p1</a>
				<a href="https://external.example/x">ext</a>
			</body></html>
		`))
	})

	// /p1 links to /p2
	mux.HandleFunc("/p1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`
			<html><body>
				<a href="/p2">p2</a>
				<a href="/dead">dead</a>
			</body></html>
		`))
	})

	// /p2 exists but should only be crawled if max-depth >= 2
	mux.HandleFunc("/p2", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<html><body><a href="/ok">ok</a></body></html>`))
	})

	mux.HandleFunc("/ok", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("/dead", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotFound) })

	srv := httptest.NewServer(mux)
	defer srv.Close()

	cfg := Config{
		StartURL:      srv.URL + "/",
		Timeout:       2 * time.Second,
		HeadFirst:     false,
		Concurrency:   8,
		MaxDepth:      1, // should crawl / and /p1, but not /p2
		MaxPages:      50,
		AllowExternal: false, // external should be skipped from checking
		UserAgent:     "deadlink-test/0.1",
	}

	var out bytes.Buffer
	var errOut bytes.Buffer

	if err := Run(context.Background(), cfg, &out, &errOut); err != nil {
		t.Fatalf("Run error: %v", err)
	}

	got := out.String()

	// Must have crawled at least 2 pages ("/" and "/p1").
	if !strings.Contains(got, "Crawled pages: 2") {
		t.Fatalf("expected to crawl 2 pages, got:\n%s", got)
	}

	// External should be skipped.
	if !strings.Contains(got, "Skipped external: 1") {
		t.Fatalf("expected 1 skipped external, got:\n%s", got)
	}

	// Dead link should be reported and show it was found on /p1.
	if !strings.Contains(got, "/dead") {
		t.Fatalf("expected /dead in output, got:\n%s", got)
	}
	if !strings.Contains(got, "found on: "+srv.URL+"/p1") {
		t.Fatalf("expected source /p1 in output, got:\n%s", got)
	}

	// With max-depth=1, /p2 should not have been crawled.
	// That means its link "/ok" should not be discovered from /p2.
	// (We still might see /ok if it appears elsewhere; it does not.)
	if strings.Contains(got, "/ok") {
		t.Fatalf("did not expect /ok to be discovered with max-depth=1, got:\n%s", got)
	}
}
