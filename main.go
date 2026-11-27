// trawl: a fast, private metasearch engine with a built-in reader/scraper.
package main

import (
	"context"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/jpdagostin0/trawl/internal/engine"
	"github.com/jpdagostin0/trawl/internal/fetch"
	"github.com/jpdagostin0/trawl/internal/search"
	"github.com/jpdagostin0/trawl/internal/web"
)

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

type config struct {
	addr, base, disable string
	timeout             time.Duration
	allowPrivate        bool
}

// app owns the running HTTP server so the tray can restart it.
type app struct {
	cfg config
	mu  sync.Mutex
	srv *http.Server
}

// start builds fresh engines, caches and breakers and starts listening.
func (a *app) start() error {
	off := strings.Split(a.cfg.disable, ",")
	var engines []engine.Engine
	for _, e := range engine.All() {
		if !slices.Contains(off, e.Name()) {
			engines = append(engines, e)
		}
	}
	ln, err := net.Listen("tcp", a.cfg.addr)
	if err != nil {
		return err
	}
	srv := &http.Server{
		Handler:           web.New(search.New(engines, a.cfg.timeout), fetch.New(a.cfg.allowPrivate), a.cfg.base).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       90 * time.Second,
	}
	a.mu.Lock()
	a.srv = srv
	a.mu.Unlock()
	log.Printf("trawl listening on http://%s (%d engines)", ln.Addr(), len(engines))
	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()
	return nil
}

func (a *app) stop() {
	a.mu.Lock()
	srv := a.srv
	a.srv = nil
	a.mu.Unlock()
	if srv == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
}

func (a *app) reload() error {
	a.stop()
	return a.start()
}

// port is the TCP port from the listen address, for display.
func (a *app) port() string {
	if _, p, err := net.SplitHostPort(a.cfg.addr); err == nil {
		return p
	}
	return a.cfg.addr
}

func main() {
	var cfg config
	flag.StringVar(&cfg.addr, "addr", env("TRAWL_ADDR", "127.0.0.1:8088"), "listen address")
	flag.StringVar(&cfg.base, "base-url", env("TRAWL_BASE_URL", ""), "public base URL (for OpenSearch); auto-detected if empty")
	flag.StringVar(&cfg.disable, "disable", env("TRAWL_DISABLE", ""), "comma-separated engines to disable")
	flag.DurationVar(&cfg.timeout, "timeout", 3*time.Second, "per-search upstream timeout")
	flag.BoolVar(&cfg.allowPrivate, "allow-private", os.Getenv("TRAWL_ALLOW_PRIVATE") == "1", "allow reader/scrape to fetch private IPs (trusted local use only)")
	flag.Parse()

	a := &app{cfg: cfg}
	if err := a.start(); err != nil {
		log.Fatal(err)
	}
	run(a) // blocks until quit (tray menu or SIGINT/SIGTERM)
	a.stop()
}
