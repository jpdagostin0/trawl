package fetch

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestScrapeAllowedChecksRedirectTargets(t *testing.T) {
	var denied atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			fmt.Fprintln(w, "User-agent: trawl\nDisallow: /private")
			return
		}
		denied.Add(1)
		fmt.Fprintln(w, "must not be fetched")
	}))
	defer destination.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/robots.txt":
			fmt.Fprintln(w, "User-agent: trawl\nDisallow: /blocked")
		case "/redirect":
			http.Redirect(w, r, destination.URL+"/private", http.StatusFound)
		case "/blocked":
			denied.Add(1)
		default:
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprint(w, "Article content")
		}
	}))
	defer origin.Close()
	f := New(true)
	for _, path := range []string{"/blocked", "/redirect"} {
		if _, err := f.ScrapeAllowed(context.Background(), origin.URL+path); err == nil {
			t.Errorf("expected robots.txt to block %s", path)
		}
	}
	if denied.Load() != 0 {
		t.Fatal("a disallowed page was requested")
	}
	p, err := f.ScrapeAllowed(context.Background(), origin.URL+"/article")
	if err != nil || p.Text != "Article content" {
		t.Fatalf("allowed article: %v, %v", p, err)
	}
}

func TestScrapeAllowedFailsClosedWhenRobotsUnavailable(t *testing.T) {
	var requested atomic.Bool
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			http.Error(w, "down", http.StatusServiceUnavailable)
			return
		}
		requested.Store(true)
	}))
	defer s.Close()
	if _, err := New(true).ScrapeAllowed(context.Background(), s.URL+"/article"); err == nil || requested.Load() {
		t.Fatalf("unreachable robots.txt must prevent article fetch: %v", err)
	}
}
