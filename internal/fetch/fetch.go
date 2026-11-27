// Package fetch turns web pages into clean, readable content (the "firecrawl" half).
package fetch

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"syscall"
	"time"

	"codeberg.org/readeck/go-readability/v2"
	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
	"github.com/JohannesKaufmann/html-to-markdown/v2/converter"
	"github.com/PuerkitoBio/goquery"
	lru "github.com/hashicorp/golang-lru/v2/expirable"
	"github.com/microcosm-cc/bluemonday"
	"github.com/temoto/robotstxt"
	"golang.org/x/sync/errgroup"

	"github.com/jpdagostin0/trawl/internal/engine"
)

const maxBody = 8 << 20

var ErrPrivate = errors.New("refusing to fetch private/internal address")

type Page struct {
	URL         string    `json:"url"`
	Title       string    `json:"title"`
	Byline      string    `json:"byline,omitempty"`
	SiteName    string    `json:"site_name,omitempty"`
	Excerpt     string    `json:"excerpt,omitempty"`
	Image       string    `json:"image,omitempty"`
	Language    string    `json:"language,omitempty"`
	Published   string    `json:"published,omitempty"`
	Markdown    string    `json:"markdown"`
	HTML        string    `json:"html,omitempty"` // sanitized article HTML
	Text        string    `json:"-"`
	Links       []string  `json:"links,omitempty"`
	WordCount   int       `json:"word_count"`
	ReadMinutes int       `json:"read_minutes"`
	FetchedAt   time.Time `json:"fetched_at"`
}

type Fetcher struct {
	client    *http.Client
	cache     *lru.LRU[string, *Page]
	robots    *lru.LRU[string, *robotstxt.RobotsData]
	sanitizer *bluemonday.Policy
}

// New returns a Fetcher. allowPrivate disables the SSRF guard (only for trusted local use).
func New(allowPrivate bool) *Fetcher {
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	if !allowPrivate {
		// Checked at connect time, after DNS resolution, so DNS rebinding can't bypass it.
		dialer.Control = func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			ip := net.ParseIP(host)
			if ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
				ip.IsUnspecified() || ip.IsMulticast() || ip.IsInterfaceLocalMulticast() {
				return ErrPrivate
			}
			return nil
		}
	}
	tr := &http.Transport{
		DialContext:           dialer.DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConnsPerHost:   4,
		IdleConnTimeout:       60 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
	}
	p := bluemonday.UGCPolicy()
	p.AddTargetBlankToFullyQualifiedLinks(true)
	p.RequireNoReferrerOnLinks(true)
	return &Fetcher{
		client: &http.Client{Transport: tr, Timeout: 20 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 8 {
				return errors.New("too many redirects")
			}
			return nil
		}},
		cache:     lru.NewLRU[string, *Page](512, nil, 30*time.Minute),
		robots:    lru.NewLRU[string, *robotstxt.RobotsData](1024, nil, 6*time.Hour),
		sanitizer: p,
	}
}

func validate(raw string) (*url.URL, error) {
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("unsupported scheme %q", u.Scheme)
	}
	if u.Host == "" {
		return nil, errors.New("missing host")
	}
	u.Fragment = ""
	return u, nil
}

func get(ctx context.Context, client *http.Client, u, agent string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", agent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml;q=0.9,text/plain;q=0.8,*/*;q=0.5")
	req.Header.Set("Accept-Language", "en-US,en;q=0.7")
	return client.Do(req)
}

// Scrape fetches a URL and extracts the main content as markdown + sanitized HTML.
func (f *Fetcher) Scrape(ctx context.Context, raw string) (*Page, error) {
	return f.scrape(ctx, raw, false)
}

// ScrapeAllowed follows robots.txt at every hop. Use it for automatic fetches.
func (f *Fetcher) ScrapeAllowed(ctx context.Context, raw string) (*Page, error) {
	return f.scrape(ctx, raw, true)
}

func (f *Fetcher) scrape(ctx context.Context, raw string, obeyRobots bool) (*Page, error) {
	u, err := validate(raw)
	if err != nil {
		return nil, err
	}
	client, agent, key := f.client, engine.UserAgent, u.String()
	if obeyRobots {
		if u.User != nil || !f.Allowed(ctx, u) {
			return nil, ErrRobots
		}
		client, agent, key = f.robotsClient(), robotsAgent, "robots:"+key
	}
	if p, ok := f.cache.Get(key); ok {
		if obeyRobots {
			final, err := url.Parse(p.URL)
			if err != nil || !f.Allowed(ctx, final) {
				return nil, ErrRobots
			}
		}
		return p, nil
	}
	resp, err := get(ctx, client, u.String(), agent)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || (obeyRobots && resp.StatusCode == http.StatusPartialContent) {
		return nil, fmt.Errorf("%s returned HTTP %d", u.Host, resp.StatusCode)
	}
	limit := int64(maxBody)
	if obeyRobots {
		limit++
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	if err != nil {
		return nil, err
	}
	if len(body) > maxBody {
		return nil, errors.New("article exceeds fetch size limit")
	}
	final := resp.Request.URL // after redirects
	ct, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if ct == "text/plain" || ct == "text/markdown" {
		txt := string(body)
		return f.store(key, &Page{URL: final.String(), Title: final.Path, Markdown: txt, Text: txt,
			HTML: "<pre>" + bluemonday.StrictPolicy().Sanitize(txt) + "</pre>"}), nil
	}
	if ct != "" && ct != "text/html" && ct != "application/xhtml+xml" {
		return nil, fmt.Errorf("unsupported content type %s", ct)
	}

	links := extractLinks(body, final)
	art, err := readability.FromReader(bytes.NewReader(body), final)
	if err != nil || art.Node == nil {
		return nil, fmt.Errorf("could not extract readable content: %v", err)
	}
	var hb, tb bytes.Buffer
	if err := art.RenderHTML(&hb); err != nil {
		return nil, err
	}
	_ = art.RenderText(&tb)
	md, err := htmltomarkdown.ConvertString(hb.String(), converter.WithDomain(final.Scheme+"://"+final.Host))
	if err != nil {
		return nil, err
	}
	words := len(strings.Fields(tb.String()))
	p := &Page{
		URL: final.String(), Title: art.Title(), Byline: art.Byline(), SiteName: art.SiteName(),
		Excerpt: art.Excerpt(), Image: art.ImageURL(), Language: art.Language(),
		Markdown: strings.TrimSpace(md), HTML: f.sanitizer.Sanitize(hb.String()), Text: tb.String(),
		Links: links, WordCount: words, ReadMinutes: max(1, (words+230)/238),
	}
	if t, err := art.PublishedTime(); err == nil {
		p.Published = t.Format("Jan 2, 2006")
	}
	if p.Title == "" {
		p.Title = final.Host
	}
	return f.store(key, p), nil
}

func (f *Fetcher) store(key string, p *Page) *Page {
	p.FetchedAt = time.Now().UTC()
	f.cache.Add(key, p)
	return p
}

func extractLinks(body []byte, base *url.URL) []string {
	d, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	d.Find("a[href]").Each(func(_ int, s *goquery.Selection) {
		h, _ := s.Attr("href")
		ref, err := url.Parse(strings.TrimSpace(h))
		if err != nil {
			return
		}
		abs := base.ResolveReference(ref)
		if abs.Scheme != "http" && abs.Scheme != "https" {
			return
		}
		abs.Fragment = ""
		k := abs.String()
		if !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	})
	return out
}

// Map returns same-site links discovered on a page (firecrawl /map equivalent).
func (f *Fetcher) Map(ctx context.Context, raw string) ([]string, error) {
	p, err := f.Scrape(ctx, raw)
	if err != nil {
		return nil, err
	}
	base, _ := url.Parse(p.URL)
	var out []string
	for _, l := range p.Links {
		if lu, err := url.Parse(l); err == nil && sameSite(lu, base) {
			out = append(out, l)
		}
	}
	return out, nil
}

func sameSite(a, b *url.URL) bool {
	return strings.TrimPrefix(a.Hostname(), "www.") == strings.TrimPrefix(b.Hostname(), "www.")
}

// Crawl does a bounded, concurrent, robots-respecting BFS of a site under the start path.
func (f *Fetcher) Crawl(ctx context.Context, raw string, limit, depth int) ([]*Page, error) {
	start, err := validate(raw)
	if err != nil {
		return nil, err
	}
	limit = min(max(limit, 1), 100)
	prefix := start.Path
	if i := strings.LastIndex(prefix, "/"); i >= 0 {
		prefix = prefix[:i+1]
	}
	var (
		mu       sync.Mutex
		seen     = map[string]bool{start.String(): true}
		pages    []*Page
		frontier = []string{start.String()}
	)
	for d := 0; d <= depth && len(frontier) > 0; d++ {
		var next []string
		g, gctx := errgroup.WithContext(ctx)
		g.SetLimit(6)
		for _, link := range frontier {
			g.Go(func() error {
				mu.Lock()
				if len(pages) >= limit {
					mu.Unlock()
					return nil
				}
				mu.Unlock()
				p, err := f.ScrapeAllowed(gctx, link)
				if err != nil {
					return nil // skip broken pages, keep crawling
				}
				mu.Lock()
				defer mu.Unlock()
				if len(pages) >= limit {
					return nil
				}
				pages = append(pages, p)
				for _, l := range p.Links {
					cu, err := url.Parse(l)
					if err != nil || !sameSite(cu, start) || !strings.HasPrefix(cu.Path, prefix) {
						continue
					}
					cu.RawQuery = ""
					k := cu.String()
					if !seen[k] {
						seen[k] = true
						next = append(next, k)
					}
				}
				return nil
			})
		}
		_ = g.Wait()
		frontier = next
	}
	return pages, nil
}
