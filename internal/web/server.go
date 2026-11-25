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

//go:embed templates/*.html
var tmplFS embed.FS

//go:embed static
var staticFS embed.FS

type Server struct {
	S       *search.Searcher
	F       *fetch.Fetcher
	BaseURL string // public URL, used for OpenSearch; derived from request if empty
	pages   map[string]*template.Template
}

var funcs = template.FuncMap{
	"host": func(raw string) string {
		u, err := url.Parse(raw)
		if err != nil {
			return raw
		}
		return strings.TrimPrefix(u.Hostname(), "www.")
	},
	"crumbs": func(raw string) string {
		u, err := url.Parse(raw)
		if err != nil {
			return ""
		}
		parts := strings.FieldsFunc(u.Path, func(r rune) bool { return r == '/' })
		if len(parts) > 3 {
			parts = append(parts[:3], "…")
		}
		if len(parts) == 0 {
			return ""
		}
		return " › " + strings.Join(parts, " › ")
	},
	"safe":  func(s string) template.HTML { return template.HTML(s) }, // only used on bluemonday-sanitized HTML
	"add":   func(a, b int) int { return a + b },
	"qesc":  url.QueryEscape,
	"title": func(s string) string { return strings.ToUpper(s[:1]) + s[1:] },
}

func New(s *search.Searcher, f *fetch.Fetcher, baseURL string) *Server {
	srv := &Server{S: s, F: f, BaseURL: strings.TrimSuffix(baseURL, "/"), pages: map[string]*template.Template{}}
	for _, p := range []string{"home", "results", "reader", "error"} {
		srv.pages[p] = template.Must(template.New("").Funcs(funcs).ParseFS(tmplFS, "templates/base.html", "templates/"+p+".html"))
	}
	return srv
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	static, _ := fs.Sub(staticFS, "static")
	mux.Handle("GET /static/", cacheForever(http.StripPrefix("/static/", http.FileServerFS(static))))
	mux.HandleFunc("GET /{$}", s.home)
	mux.HandleFunc("GET /search", s.search)
	mux.HandleFunc("GET /read", s.read)
	mux.HandleFunc("GET /suggest", s.suggest)
	mux.HandleFunc("GET /favicon", s.favicon)
	mux.HandleFunc("GET /opensearch.xml", s.opensearch)
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
		// Reader view shows remote images; everything else is same-origin. No third-party scripts, ever.
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src * data:; script-src 'self'; style-src 'self'; frame-ancestors 'none'")
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

func (s *Server) render(w http.ResponseWriter, page string, status int, data map[string]any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := s.pages[page].ExecuteTemplate(w, "base", data); err != nil {
		log.Printf("template %s: %v", page, err)
	}
}

func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	s.render(w, "home", 200, map[string]any{"Home": true})
}

func parseCategory(c string) engine.Category {
	for _, cat := range engine.Categories {
		if string(cat) == c {
			return cat
		}
	}
	return engine.General
}

func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	cat := parseCategory(r.URL.Query().Get("c"))
	// !bangs: external redirect, or switch category
	if target, rest, ok := bang(q); ok {
		if target != "" {
			http.Redirect(w, r, target, http.StatusFound)
			return
		}
		q, cat = rest.query, rest.cat
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("p"))
	page = max(page, 1)
	resp := s.S.Search(r.Context(), engine.Query{Text: q, Page: page, Lang: "en"}, cat, nil)

	var readerURL string
	if looksLikeURL(q) {
		readerURL = q
	}
	allFailed := len(resp.Results) == 0
	for _, e := range resp.Engines {
		if e.Error == "" {
			allFailed = false
		}
	}
	s.render(w, "results", 200, map[string]any{
		"Q": q, "R": resp, "Cat": string(cat), "Cats": engine.Categories, "Page": page,
		"ReaderURL": readerURL, "AllFailed": allFailed && len(resp.Engines) > 0,
	})
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

func (s *Server) read(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("url")
	if raw == "" {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	p, err := s.F.Scrape(ctx, raw)
	if err != nil {
		s.render(w, "error", http.StatusBadGateway, map[string]any{"Msg": "Couldn't open this page in reader view.", "Detail": err.Error(), "URL": raw})
		return
	}
	switch r.URL.Query().Get("format") {
	case "md", "markdown":
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		if r.URL.Query().Has("download") {
			w.Header().Set("Content-Disposition", `attachment; filename="`+slug(p.Title)+`.md"`)
		}
		io.WriteString(w, "# "+p.Title+"\n\nSource: "+p.URL+"\n\n"+p.Markdown+"\n")
		return
	}
	s.render(w, "reader", 200, map[string]any{"P": p, "Reader": true})
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
	writeJSON(w, 200, resp)
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
