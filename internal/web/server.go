// Package web is the human-facing UI and the JSON API.
package web

import (
	"context"
	"embed"
	"encoding/json"
	"html/template"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jpdagostin0/trawl/internal/engine"
	"github.com/jpdagostin0/trawl/internal/fetch"
	"github.com/jpdagostin0/trawl/internal/search"
)

// dist is the built React frontend (frontend/ -> `npm run build`).
//
//go:embed all:dist
var distFS embed.FS

type Server struct {
	S       *search.Searcher
	F       *fetch.Fetcher
	BaseURL string // public URL, used for OpenSearch; derived from request if empty
	shell   []byte // dist/index.html
}

func New(s *search.Searcher, f *fetch.Fetcher, baseURL string) *Server {
	shell, err := distFS.ReadFile("dist/index.html")
	if err != nil {
		log.Fatalf("frontend not built: %v (run `npm run build` in frontend/)", err)
	}
	return &Server{S: s, F: f, BaseURL: strings.TrimSuffix(baseURL, "/"), shell: shell}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	static, _ := fs.Sub(distFS, "dist")
	mux.Handle("GET /static/", cacheForever(http.StripPrefix("/static/", http.FileServerFS(static))))
	mux.HandleFunc("GET /{$}", s.app)
	mux.HandleFunc("GET /search", s.search)
	mux.HandleFunc("GET /read", s.read)
	mux.HandleFunc("GET /suggest", s.suggest)
	mux.HandleFunc("GET /favicon", s.favicon)
	mux.HandleFunc("GET /img", s.img)
	mux.HandleFunc("GET /opensearch.xml", s.opensearch)
	mux.HandleFunc("GET /api/config", s.apiConfig)
	mux.HandleFunc("GET /api/search", s.apiSearch)
	mux.HandleFunc("GET /api/scrape", s.apiScrape)
	mux.HandleFunc("GET /api/map", s.apiMap)
	mux.HandleFunc("GET /api/crawl", s.apiCrawl)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, "ok") })
	return secure(logging(mux))
}

func cacheForever(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=86400")
		h.ServeHTTP(w, r)
	})
}

func secure(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		// Every image (thumbnails, infobox, reader) goes through /img, so nothing third-party loads.
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; script-src 'self'; style-src 'self'; frame-ancestors 'none'")
		h.ServeHTTP(w, r)
	})
}

func logging(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t := time.Now()
		h.ServeHTTP(w, r)
		// queries are deliberately not logged
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(t).Round(time.Millisecond))
	})
}

// app serves the React shell; routing happens client-side.
func (s *Server) app(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(s.shell)
}

func parseCategory(c string) engine.Category {
	for _, cat := range engine.Categories {
		if string(cat) == c {
			return cat
		}
	}
	return engine.General
}

// search handles the server-side parts of /search, then hands off to the app:
// !bangs redirect, and "I'm Feeling Lucky" (btnI, as on google.com) jumps to the top result.
func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	qv := r.URL.Query()
	q := strings.TrimSpace(qv.Get("q"))
	if q == "" {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	if target, rest, ok := bang(q); ok {
		if target == "" {
			target = "/search?q=" + url.QueryEscape(rest.query) + "&c=" + string(rest.cat)
		}
		http.Redirect(w, r, target, http.StatusFound)
		return
	}
	if qv.Has("btnI") {
		resp := s.S.Search(r.Context(), engine.Query{Text: q, Page: 1, Lang: "en"}, parseCategory(qv.Get("c")), nil)
		if len(resp.Results) > 0 {
			http.Redirect(w, r, resp.Results[0].URL, http.StatusFound)
			return
		}
		http.Redirect(w, r, "/search?q="+url.QueryEscape(q)+"&c="+qv.Get("c"), http.StatusFound)
		return
	}
	s.app(w, r)
}

func looksLikeURL(q string) bool {
	if strings.ContainsAny(q, " \t") {
		return false
	}
	if strings.HasPrefix(q, "http://") || strings.HasPrefix(q, "https://") {
		return true
	}
	u, err := url.Parse("https://" + q)
	return err == nil && strings.Contains(u.Hostname(), ".") && !strings.HasSuffix(u.Hostname(), ".")
}

// read serves raw markdown for ?format=md; otherwise the app renders the reader.
func (s *Server) read(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("url")
	if raw == "" {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	switch r.URL.Query().Get("format") {
	case "md", "markdown":
	default:
		s.app(w, r)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	p, err := s.F.Scrape(ctx, raw)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	if r.URL.Query().Has("download") {
		w.Header().Set("Content-Disposition", `attachment; filename="`+slug(p.Title)+`.md"`)
	}
	io.WriteString(w, "# "+p.Title+"\n\nSource: "+p.URL+"\n\n"+p.Markdown+"\n")
}

// img proxies remote images (result thumbnails, infobox pictures, reader images).
func (s *Server) img(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	ctype, data, err := s.F.Image(ctx, r.URL.Query().Get("u"))
	if err != nil {
		http.Error(w, "image unavailable", http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	// SVGs opened directly must not run script.
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
	w.Write(data)
}

func slug(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case b.Len() > 0 && !strings.HasSuffix(b.String(), "-"):
			b.WriteByte('-')
		}
		if b.Len() > 60 {
			break
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "page"
	}
	return out
}

// suggest proxies DuckDuckGo autocomplete so the browser never talks to a third party.
func (s *Server) suggest(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	w.Header().Set("Content-Type", "application/x-suggestions+json")
	if strings.TrimSpace(q) == "" || strings.HasPrefix(q, "!") {
		json.NewEncoder(w).Encode([]any{q, []string{}})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://duckduckgo.com/ac/?type=list&q="+url.QueryEscape(q), nil)
	req.Header.Set("User-Agent", engine.UserAgent)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		json.NewEncoder(w).Encode([]any{q, []string{}})
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Cache-Control", "private, max-age=300")
	io.Copy(w, io.LimitReader(resp.Body, 64<<10))
}

func (s *Server) base(r *http.Request) string {
	if s.BaseURL != "" {
		return s.BaseURL
	}
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func (s *Server) opensearch(w http.ResponseWriter, r *http.Request) {
	b := template.HTMLEscapeString(s.base(r))
	w.Header().Set("Content-Type", "application/opensearchdescription+xml")
	io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>
<OpenSearchDescription xmlns="http://a9.com/-/spec/opensearch/1.1/" xmlns:moz="http://www.mozilla.org/2006/browser/search/">
  <ShortName>trawl</ShortName>
  <Description>Private metasearch with reader view</Description>
  <InputEncoding>UTF-8</InputEncoding>
  <Image width="16" height="16" type="image/svg+xml">`+b+`/static/icon.svg</Image>
  <Url type="text/html" method="get" template="`+b+`/search?q={searchTerms}"/>
  <Url type="application/x-suggestions+json" method="get" template="`+b+`/suggest?q={searchTerms}"/>
  <moz:SearchForm>`+b+`/</moz:SearchForm>
</OpenSearchDescription>`)
}

// ---- JSON API (firecrawl-ish) ----

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(v)
}

func apiErr(w http.ResponseWriter, status int, err string) {
	writeJSON(w, status, map[string]string{"error": err})
}

// apiConfig tells the frontend which tabs have engines and which bangs exist.
func (s *Server) apiConfig(w http.ResponseWriter, r *http.Request) {
	type cat struct {
		ID      string   `json:"id"`
		Engines []string `json:"engines"`
	}
	var cats []cat
	for _, c := range engine.Categories {
		var names []string
		for _, e := range s.S.Engines() {
			if e.Category() == c {
				names = append(names, e.Name())
			}
		}
		if len(names) > 0 {
			cats = append(cats, cat{string(c), names})
		}
	}
	bangSites := map[string]string{}
	for name, tmpl := range bangs {
		if u, err := url.Parse(tmpl); err == nil {
			bangSites[name] = strings.TrimPrefix(u.Host, "www.")
		}
	}
	catBangNames := map[string]string{}
	for name, c := range catBangs {
		catBangNames[name] = string(c)
	}
	w.Header().Set("Cache-Control", "public, max-age=300")
	writeJSON(w, 200, map[string]any{"categories": cats, "bangs": bangSites, "tab_bangs": catBangNames})
}

// searchResponse adds UI hints to the search response.
type searchResponse struct {
	*search.Response
	ReaderURL string `json:"reader_url,omitempty"` // the query is a URL: offer the reader
}

func (s *Server) apiSearch(w http.ResponseWriter, r *http.Request) {
	qv := r.URL.Query()
	q := strings.TrimSpace(qv.Get("q"))
	if q == "" {
		apiErr(w, 400, "missing q")
		return
	}
	page, _ := strconv.Atoi(qv.Get("page"))
	var only []string
	if e := qv.Get("engines"); e != "" {
		only = strings.Split(e, ",")
	}
	resp := s.S.Search(r.Context(), engine.Query{Text: q, Page: max(page, 1), Lang: "en"}, parseCategory(qv.Get("category")), only)
	// scrape=N: also extract markdown for the top N results, in parallel
	if n, _ := strconv.Atoi(qv.Get("scrape")); n > 0 {
		n = min(n, 10, len(resp.Results))
		ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
		defer cancel()
		pages := make([]any, n)
		done := make(chan struct{}, n)
		for i := 0; i < n; i++ {
			go func(i int) {
				defer func() { done <- struct{}{} }()
				if p, err := s.F.Scrape(ctx, resp.Results[i].URL); err == nil {
					pages[i] = map[string]any{"url": p.URL, "title": p.Title, "markdown": p.Markdown}
				} else {
					pages[i] = map[string]any{"url": resp.Results[i].URL, "error": err.Error()}
				}
			}(i)
		}
		for i := 0; i < n; i++ {
			<-done
		}
		writeJSON(w, 200, map[string]any{"search": resp, "scraped": pages})
		return
	}
	out := searchResponse{Response: resp}
	if looksLikeURL(q) {
		out.ReaderURL = q
	}
	writeJSON(w, 200, out)
}

func (s *Server) apiScrape(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	p, err := s.F.Scrape(ctx, r.URL.Query().Get("url"))
	if err != nil {
		apiErr(w, 502, err.Error())
		return
	}
	out := *p
	if r.URL.Query().Get("html") != "1" {
		out.HTML = ""
	}
	if r.URL.Query().Get("links") != "1" {
		out.Links = nil
	}
	writeJSON(w, 200, out)
}

func (s *Server) apiMap(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	links, err := s.F.Map(ctx, r.URL.Query().Get("url"))
	if err != nil {
		apiErr(w, 502, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"links": links, "count": len(links)})
}

func (s *Server) apiCrawl(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	depth, err := strconv.Atoi(r.URL.Query().Get("depth"))
	if err != nil {
		depth = 2
	}
	if limit == 0 {
		limit = 10
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	pages, err := s.F.Crawl(ctx, r.URL.Query().Get("url"), limit, min(depth, 5))
	if err != nil {
		apiErr(w, 400, err.Error())
		return
	}
	out := make([]map[string]any, 0, len(pages))
	for _, p := range pages {
		out = append(out, map[string]any{"url": p.URL, "title": p.Title, "markdown": p.Markdown, "word_count": p.WordCount})
	}
	writeJSON(w, 200, map[string]any{"pages": out, "count": len(out)})
}
