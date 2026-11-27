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

	"github.com/jpdagostin0/trawl/internal/answer"
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
	Answer   *answer.Answer  `json:"answer,omitempty"` // calculator / unit conversion
	Infobox  *engine.Infobox `json:"infobox,omitempty"`
	Engines  []EngineStatus  `json:"engines"`
	TookMS   int64           `json:"took_ms"`
	Cached   bool            `json:"cached"`
	Done     bool            `json:"done"` // false for the partial snapshots Stream sends while engines are still answering
}

type Searcher struct {
	engines []engine.Engine
	timeout time.Duration
	cache   *lru.LRU[string, *Response]

	// Rerank, if set, personalizes every response after the cache, so votes
	// take effect immediately even on cached queries.
	Rerank Reranker

	mu      sync.Mutex
	breaker map[string]*breaker
}

// Reranker adjusts merged results in place (scores and order).
type Reranker interface {
	Prepare(q string) // called when a search starts; must not block
	Rerank(q engine.Query, cat engine.Category, results []engine.Result)
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

// Search runs the query on every engine in the category and returns the merged
// results once all of them have answered. If only is non-empty, it restricts to
// those engine names.
func (s *Searcher) Search(ctx context.Context, q engine.Query, cat engine.Category, only []string) *Response {
	var last *Response
	s.Stream(ctx, q, cat, only, func(r *Response) { last = r })
	return last
}

// Stream is Search, but calls emit with a merged snapshot as soon as the first
// engine answers and again each time another engine (or the infobox) comes in.
// The last snapshot has Done set. emit is called from the calling goroutine.
func (s *Searcher) Stream(ctx context.Context, q engine.Query, cat engine.Category, only []string, emit func(*Response)) {
	if s.Rerank != nil {
		s.Rerank.Prepare(q.Text)
		raw := emit
		emit = func(r *Response) {
			cp := *r
			cp.Results = slices.Clone(r.Results) // r may be the cached copy
			s.Rerank.Rerank(q, cat, cp.Results)
			raw(&cp)
		}
	}
	key := fmt.Sprintf("%s\x00%s\x00%d\x00%s\x00%s", cat, q.Text, q.Page, q.Lang, strings.Join(only, ","))
	if r, ok := s.cache.Get(key); ok {
		cp := *r
		cp.Cached = true
		emit(&cp)
		return
	}
	start := time.Now()
	var ans *answer.Answer
	if cat == engine.General && q.Page == 1 {
		ans = answer.For(q.Text)
	}
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	// Each goroutine sends exactly one event; nil out means an infobox event.
	type event struct {
		out     *out
		infobox *engine.Infobox
	}
	var (
		outs    []out
		infobox *engine.Infobox
		running int
	)
	events := make(chan event)
	for _, e := range s.engines {
		if e.Category() != cat || (len(only) > 0 && !slices.Contains(only, e.Name())) {
			continue
		}
		if s.suspended(e.Name()) {
			outs = append(outs, out{eng: e, status: EngineStatus{Name: e.Name(), Skipped: true, Error: "suspended after errors"}})
			continue
		}
		running++
		go func(e engine.Engine) {
			t0 := time.Now()
			res, err := e.Search(ctx, q)
			st := EngineStatus{Name: e.Name(), Results: len(res), TookMS: time.Since(t0).Milliseconds()}
			if err != nil {
				st.Error = err.Error()
			}
			s.record(e.Name(), err)
			events <- event{out: &out{e, res, st}}
		}(e)
		if ip, ok := e.(engine.InfoboxProvider); ok && q.Page == 1 && ans == nil {
			running++
			go func() {
				ib, err := ip.Infobox(ctx, q)
				if err != nil {
					ib = nil
				}
				events <- event{infobox: ib}
			}()
		}
	}

	snapshot := func(done bool) *Response {
		return &Response{Query: q.Text, Category: cat, Page: q.Page, Results: merge(outs, q), Answer: ans, Infobox: infobox,
			Engines: statuses(outs), TookMS: time.Since(start).Milliseconds(), Done: done}
	}
	answered := false
	for ; running > 0; running-- {
		ev := <-events
		if ev.out != nil {
			outs = append(outs, *ev.out)
			answered = true
		} else if ev.infobox != nil {
			infobox = ev.infobox
		} else {
			continue
		}
		if answered && running > 1 {
			emit(snapshot(false))
		}
	}
	resp := snapshot(true)
	if len(resp.Results) > 0 {
		s.cache.Add(key, resp)
	}
	emit(resp)
}

type out struct {
	eng     engine.Engine
	results []engine.Result
	status  EngineStatus
}

// merge does weighted reciprocal-rank fusion: results several engines agree on float up.
func merge(outs []out, q engine.Query) []engine.Result {
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
	return results
}

func statuses(outs []out) []EngineStatus {
	st := make([]EngineStatus, 0, len(outs))
	for _, o := range outs {
		st = append(st, o.status)
	}
	sort.Slice(st, func(i, j int) bool { return st[i].Name < st[j].Name })
	return st
}

// snippetJunk is markup and escaped newlines some upstreams leak into snippets.
var snippetJunk = strings.NewReplacer(`\r`, " ", `\n`, " ", "<br>", " ", "<br/>", " ", "<br />", " ", "\r", " ", "\n", " ")

func tidySnippet(s string) string {
	return strings.Join(strings.Fields(snippetJunk.Replace(s)), " ")
}

// Key is the dedupe key results are merged on; feedback matches votes by it.
func Key(raw string) string { return normalize(raw) }

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
