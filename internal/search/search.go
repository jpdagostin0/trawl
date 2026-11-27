// Package search fans a query out to engines concurrently and merges the results.
package search

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru/v2/expirable"

	"github.com/jpdagostin0/trawl/internal/engine"
)

type EngineStatus struct {
	Name    string `json:"name"`
	Results int    `json:"results"`
	TookMS  int64  `json:"took_ms"`
	Error   string `json:"error,omitempty"`
	Skipped bool   `json:"skipped,omitempty"` // suspended after repeated failures
}

type Response struct {
	Query    string          `json:"query"`
	Category engine.Category `json:"category"`
	Page     int             `json:"page"`
	Results  []engine.Result `json:"results"`
	Infobox  *engine.Infobox `json:"infobox,omitempty"`
	Engines  []EngineStatus  `json:"engines"`
	TookMS   int64           `json:"took_ms"`
	Cached   bool            `json:"cached"`
}

type Searcher struct {
	engines []engine.Engine
	timeout time.Duration
	cache   *lru.LRU[string, *Response]

	mu      sync.Mutex
	breaker map[string]*breaker
}

// breaker suspends an engine after consecutive failures, like SearXNG's suspended_time,
// so one rate-limited backend doesn't add latency to every search.
type breaker struct {
	fails int
	until time.Time
}

func New(engines []engine.Engine, timeout time.Duration) *Searcher {
	return &Searcher{
		engines: engines,
		timeout: timeout,
		cache:   lru.NewLRU[string, *Response](2048, nil, 10*time.Minute),
		breaker: map[string]*breaker{},
	}
}

func (s *Searcher) Engines() []engine.Engine { return s.engines }

func (s *Searcher) suspended(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	b := s.breaker[name]
	return b != nil && time.Now().Before(b.until)
}

func (s *Searcher) record(name string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b := s.breaker[name]
	if b == nil {
		b = &breaker{}
		s.breaker[name] = b
	}
	if err == nil {
		b.fails = 0
		return
	}
	b.fails++
	var blocked engine.ErrBlocked
	switch {
	case errors.As(err, &blocked):
		b.until = time.Now().Add(10 * time.Minute)
	case b.fails >= 3:
		b.until = time.Now().Add(time.Duration(min(b.fails, 10)) * time.Minute)
	}
}

// Search runs the query on every engine in the category. If only is non-empty,
// it restricts to those engine names.
func (s *Searcher) Search(ctx context.Context, q engine.Query, cat engine.Category, only []string) *Response {
	key := fmt.Sprintf("%s\x00%s\x00%d\x00%s\x00%s", cat, q.Text, q.Page, q.Lang, strings.Join(only, ","))
	if r, ok := s.cache.Get(key); ok {
		cp := *r
		cp.Cached = true
		return &cp
	}
	start := time.Now()
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	type out struct {
		eng     engine.Engine
		results []engine.Result
		status  EngineStatus
	}
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		outs    []out
		infobox *engine.Infobox
	)
	for _, e := range s.engines {
		if e.Category() != cat || (len(only) > 0 && !slices.Contains(only, e.Name())) {
			continue
		}
		if s.suspended(e.Name()) {
			mu.Lock()
			outs = append(outs, out{eng: e, status: EngineStatus{Name: e.Name(), Skipped: true, Error: "suspended after errors"}})
			mu.Unlock()
			continue
		}
		wg.Add(1)
		go func(e engine.Engine) {
			defer wg.Done()
			t0 := time.Now()
			res, err := e.Search(ctx, q)
			st := EngineStatus{Name: e.Name(), Results: len(res), TookMS: time.Since(t0).Milliseconds()}
			if err != nil {
				st.Error = err.Error()
			}
			s.record(e.Name(), err)
			mu.Lock()
			outs = append(outs, out{e, res, st})
			mu.Unlock()
		}(e)
		if ip, ok := e.(engine.InfoboxProvider); ok && q.Page == 1 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if ib, err := ip.Infobox(ctx, q); err == nil && ib != nil {
					mu.Lock()
					infobox = ib
					mu.Unlock()
				}
			}()
		}
	}
	wg.Wait()

	// Weighted reciprocal-rank fusion: results several engines agree on float up.
	merged := map[string]*engine.Result{}
	var order []string
	for _, o := range outs {
		for rank, r := range o.results {
			r.URL = CleanURL(r.URL)
			r.Snippet = tidySnippet(r.Snippet)
			k := normalize(r.URL)
			if k == "" {
				continue
			}
			sc := o.eng.Weight() / float64(60+rank)
			if m, ok := merged[k]; ok {
				m.Score += sc
				m.Engines = append(m.Engines, o.eng.Name())
				if len(r.Snippet) > len(m.Snippet) {
					m.Snippet = r.Snippet
				}
				if m.Meta == "" {
					m.Meta = r.Meta
				}
				for _, f := range [][2]*string{{&m.Thumbnail, &r.Thumbnail}, {&m.Image, &r.Image}, {&m.Resolution, &r.Resolution}, {&m.Source, &r.Source}, {&m.Published, &r.Published}} {
					if *f[0] == "" {
						*f[0] = *f[1]
					}
				}
				continue
			}
			r := r
			r.Score = sc
			r.Engines = []string{o.eng.Name()}
			merged[k] = &r
			order = append(order, k)
		}
	}
	results := make([]engine.Result, 0, len(merged))
	terms := strings.Fields(strings.ToLower(q.Text))
	for _, k := range order {
		r := merged[k]
		// Light lexical re-rank: scraped engines sometimes return generic pages
		// that ignore half the query. Boost results that mention the query terms.
		if len(terms) > 0 {
			hay := strings.ToLower(r.Title + " " + r.Snippet + " " + r.URL)
			hit := 0
			for _, t := range terms {
				if strings.Contains(hay, t) {
					hit++
				}
			}
			r.Score *= 1 + 0.5*float64(hit)/float64(len(terms))
		}
		results = append(results, *r)
	}
	sort.SliceStable(results, func(i, j int) bool { return results[i].Score > results[j].Score })

	statuses := make([]EngineStatus, 0, len(outs))
	for _, o := range outs {
		statuses = append(statuses, o.status)
	}
	sort.Slice(statuses, func(i, j int) bool { return statuses[i].Name < statuses[j].Name })

	resp := &Response{Query: q.Text, Category: cat, Page: q.Page, Results: results, Infobox: infobox,
		Engines: statuses, TookMS: time.Since(start).Milliseconds()}
	if len(results) > 0 {
		s.cache.Add(key, resp)
	}
	return resp
}

// snippetJunk is markup and escaped newlines some upstreams leak into snippets.
var snippetJunk = strings.NewReplacer(`\r`, " ", `\n`, " ", "<br>", " ", "<br/>", " ", "<br />", " ", "\r", " ", "\n", " ")

func tidySnippet(s string) string {
	return strings.Join(strings.Fields(snippetJunk.Replace(s)), " ")
}

// normalize builds a dedupe key: scheme-less, www-less, no fragment, no tracking
// params (via CleanURL).
func normalize(raw string) string {
	u, err := url.Parse(CleanURL(raw))
	if err != nil || u.Host == "" {
		return ""
	}
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	host = strings.TrimPrefix(host, "m.")
	key := host + strings.TrimSuffix(u.EscapedPath(), "/")
	if u.RawQuery != "" {
		key += "?" + u.RawQuery
	}
	return key
}
