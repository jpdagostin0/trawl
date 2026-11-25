package web

import (
	"context"
	"io"
	"net/http"
	"regexp"
	"time"

	lru "github.com/hashicorp/golang-lru/v2/expirable"

	"github.com/jpdagostin0/trawl/internal/engine"
)

type icon struct {
	ctype string
	data  []byte
}

var (
	iconCache = lru.NewLRU[string, icon](4096, nil, 24*time.Hour)
	hostRe    = regexp.MustCompile(`^[a-z0-9.-]{1,253}$`)
	iconHTTP  = &http.Client{Timeout: 3 * time.Second}
	blankIcon = icon{"image/svg+xml", []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16"><circle cx="8" cy="8" r="6" fill="#8888"/></svg>`)}
)

// favicon proxies site icons so result pages make zero third-party requests from the browser.
func (s *Server) favicon(w http.ResponseWriter, r *http.Request) {
	h := r.URL.Query().Get("h")
	if !hostRe.MatchString(h) {
		http.Error(w, "bad host", 400)
		return
	}
	ic, ok := iconCache.Get(h)
	if !ok {
		ic = blankIcon
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		req, _ := http.NewRequestWithContext(ctx, "GET", "https://icons.duckduckgo.com/ip3/"+h+".ico", nil)
		req.Header.Set("User-Agent", engine.UserAgent)
		if resp, err := iconHTTP.Do(req); err == nil {
			data, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
			resp.Body.Close()
			if resp.StatusCode == 200 && len(data) > 0 {
				ic = icon{http.DetectContentType(data), data}
			}
		}
		iconCache.Add(h, ic)
	}
	w.Header().Set("Content-Type", ic.ctype)
	w.Header().Set("Cache-Control", "public, max-age=604800")
	w.Write(ic.data)
}
