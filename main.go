// trawl: a fast, private metasearch engine with a built-in reader/scraper.
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"
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

func main() {
	addr := flag.String("addr", env("TRAWL_ADDR", "127.0.0.1:8088"), "listen address")
	base := flag.String("base-url", env("TRAWL_BASE_URL", ""), "public base URL (for OpenSearch); auto-detected if empty")
	disable := flag.String("disable", env("TRAWL_DISABLE", ""), "comma-separated engines to disable")
	timeout := flag.Duration("timeout", 3*time.Second, "per-search upstream timeout")
	allowPrivate := flag.Bool("allow-private", os.Getenv("TRAWL_ALLOW_PRIVATE") == "1", "allow reader/scrape to fetch private IPs (trusted local use only)")
	flag.Parse()

	off := strings.Split(*disable, ",")
	var engines []engine.Engine
	for _, e := range engine.All() {
		if !slices.Contains(off, e.Name()) {
			engines = append(engines, e)
		}
	}

	srv := &http.Server{
		Addr:              *addr,
		Handler:           web.New(search.New(engines, *timeout), fetch.New(*allowPrivate), *base).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       90 * time.Second,
	}
	go func() {
		log.Printf("trawl listening on http://%s (%d engines)", *addr, len(engines))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Shutdown(sctx)
}
