package app

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func BenchmarkRun_Stage1(b *testing.B) {
	// Build a predictable local site:
	// - start page links to many endpoints
	// - mixture of ok, dead, redirect
	mux := http.NewServeMux()

	// Create endpoints
	okCount := 60
	deadCount := 20
	redirCount := 10

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)

		// Build HTML with many links
		var html bytes.Buffer
		html.WriteString("<html><body>\n")
		for i := 0; i < okCount; i++ {
			fmt.Fprintf(&html, `<a href="/ok/%d">ok</a>`+"\n", i)
		}
		for i := 0; i < deadCount; i++ {
			fmt.Fprintf(&html, `<a href="/dead/%d">dead</a>`+"\n", i)
		}
		for i := 0; i < redirCount; i++ {
			fmt.Fprintf(&html, `<a href="/redir/%d">redir</a>`+"\n", i)
		}
		html.WriteString("</body></html>\n")
		_, _ = w.Write(html.Bytes())
	})

	mux.HandleFunc("/ok/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/dead/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("/redir/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/ok/0", http.StatusMovedPermanently)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	cfg := Config{
		StartURL:  srv.URL + "/",
		Timeout:   2 * time.Second,
		HeadFirst: true,
		UserAgent: "deadlink-bench/0.1",
	}

	ctx := context.Background()

	// Avoid measuring benchmark output overhead: discard by writing to buffers.
	var out bytes.Buffer
	var errOut bytes.Buffer

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		out.Reset()
		errOut.Reset()

		if err := Run(ctx, cfg, &out, &errOut); err != nil {
			b.Fatalf("Run error: %v", err)
		}
	}
}
