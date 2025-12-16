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

func TestRun_ConcurrencyStableAndComplete(t *testing.T) {
	mux := http.NewServeMux()

	// Start page includes links in "unsorted" order on purpose.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`
			<html><body>
				<a href="/z-dead">z</a>
				<a href="/a-dead">a</a>
				<a href="/ok">ok</a>
			</body></html>
		`))
	})

	mux.HandleFunc("/ok", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/a-dead", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("/z-dead", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	cfg := Config{
		StartURL:    srv.URL + "/",
		Timeout:     2 * time.Second,
		HeadFirst:   false, // keep deterministic for test
		Concurrency: 8,
		UserAgent:   "deadlink-test/0.1",
	}

	var out bytes.Buffer
	var errOut bytes.Buffer

	if err := Run(context.Background(), cfg, &out, &errOut); err != nil {
		t.Fatalf("Run error: %v", err)
	}

	got := out.String()

	// Must say checked 3 links.
	if !strings.Contains(got, "Checked 3 links") {
		t.Fatalf("expected summary for 3 links, got:\n%s", got)
	}

	// Dead links should appear sorted by URL (a-dead before z-dead).
	aIdx := strings.Index(got, "/a-dead")
	zIdx := strings.Index(got, "/z-dead")
	if aIdx == -1 || zIdx == -1 {
		t.Fatalf("expected both dead links in output, got:\n%s", got)
	}
	if aIdx > zIdx {
		t.Fatalf("expected sorted output (a-dead before z-dead), got:\n%s", got)
	}
}
