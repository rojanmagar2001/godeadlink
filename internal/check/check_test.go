package check

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestChecker_OKAndDead(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/ok", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/dead", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	ctx := context.Background()
	chk := NewChecker(2*time.Second, true)

	ok := chk.Check(ctx, srv.URL+"/ok")
	if ok.Err != nil || ok.StatusCode != 200 || ok.IsDead() {
		t.Fatalf("expected OK, got %+v", ok)
	}

	dead := chk.Check(ctx, srv.URL+"/dead")
	if dead.Err != nil || dead.StatusCode != 404 || !dead.IsDead() {
		t.Fatalf("expected DEAD, got %+v", dead)
	}
}

func TestChecker_Timeout(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/slow", func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	chk := NewChecker(50*time.Millisecond, false)

	ctx := context.Background()
	res := chk.Check(ctx, srv.URL+"/slow")

	if res.Err == nil {
		t.Fatalf("expected timeout error, got nil")
	}
}

func TestChecker_NetworkError(t *testing.T) {
	// Unused local port to force connection error
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := ln.Addr().String()
	ln.Close()

	chk := NewChecker(1*time.Second, false)

	ctx := context.Background()
	res := chk.Check(ctx, "http://"+addr)

	if res.Err == nil {
		t.Fatalf("expected network error, got nil")
	}
}

func TestChecker_DeadAndRedirectAndOK(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`
			<html><body>
				<a href="/ok">ok</a>
				<a href="/dead">dead</a>
				<a href="/redir">redir</a>
			</body></html>
		`))
	})
	mux.HandleFunc("/ok", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/dead", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("/redir", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/ok", http.StatusMovedPermanently)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	chk := NewChecker(2*time.Second, true)

	okRes := chk.Check(ctx, srv.URL+"/ok")
	if okRes.Err != nil || okRes.StatusCode != 200 {
		t.Fatalf("ok: got err=%v code=%d", okRes.Err, okRes.StatusCode)
	}

	deadRes := chk.Check(ctx, srv.URL+"/dead")
	if deadRes.Err != nil || deadRes.StatusCode != 404 || !deadRes.IsDead() {
		t.Fatalf("dead: got err=%v code=%d isDead=%v", deadRes.Err, deadRes.StatusCode, deadRes.IsDead())
	}

	redirRes := chk.Check(ctx, srv.URL+"/redir")
	// net/http follows redirects by default, so expect 200 here.
	if redirRes.Err != nil || redirRes.StatusCode != 200 {
		t.Fatalf("redir: got err=%v code=%d", redirRes.Err, redirRes.StatusCode)
	}
	if redirRes.IsDead() {
		t.Fatalf("redir should not be dead")
	}
}
