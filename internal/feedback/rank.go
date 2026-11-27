package feedback

import (
	"log"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jpdagostin0/trawl/internal/engine"
)

const (
	// Every vote applies to every search. Query similarity only scales it, from
	// globalReach (unrelated search) up to 1 (the search you voted on).
	globalReach   = 0.5
	pageWeight    = 2.0 // a vote on this exact page: x4 (up) or /4 (down) on the voted search, x2 or /2 everywhere else
	siteWeight    = 0.3 // a vote on another page of the same site
	contentWeight = 1.5 // a vote on a different page whose text embeds close to this one
	maxPage       = 3.0
	maxSite       = 1.0
	maxContent    = 1.5
	// Cosine thresholds, calibrated on embeddinggemma. Queries: paraphrases
	// ~0.99, related 0.74-0.78, unrelated 0.47-0.57. Pages (title + snippet):
	// same topic 0.58-0.77, different topics at most 0.47; 0.6 keeps pages on
	// the voted page's subject and drops unrelated ones.
	querySim = 0.65
	pageSim  = 0.6
)

// Ranker applies stored votes to search results. It implements search.Reranker.
type Ranker struct {
	Store *Store
	Emb   *Embedder // nil: word overlap and exact pages/sites only

	mu           sync.Mutex
	lastBackfill time.Time
}

// Vote records v and embeds its query and page into DuckDB in the background.
func (r *Ranker) Vote(v Vote) error {
	if err := r.Store.Record(v); err != nil {
		return err
	}
	if r.Emb != nil && v.Dir != 0 {
		go r.embed([]string{normQuery(v.Query)}, []pageDoc{{r.Store.key(v.URL), v.Title, v.Snippet}})
	}
	return nil
}

// Backfill embeds voted queries and pages that have no vector for the current
// model (Ollama was down when they were voted, or the model changed). It runs
// at most every 5 minutes.
func (r *Ranker) Backfill() {
	if r.Emb == nil {
		return
	}
	r.mu.Lock()
	if time.Since(r.lastBackfill) < 5*time.Minute {
		r.mu.Unlock()
		return
	}
	r.lastBackfill = time.Now()
	r.mu.Unlock()
	qs, pages, err := r.Store.missingVecs(r.Emb.Model)
	if err != nil {
		log.Printf("feedback: %v", err)
		return
	}
	if len(qs)+len(pages) > 0 {
		r.embed(qs, pages)
	}
}

func (r *Ranker) embed(qs []string, pages []pageDoc) {
	e := r.Emb
	if len(qs) > 0 {
		texts := make([]string, len(qs))
		for i, q := range qs {
			texts[i] = e.queryText(q)
		}
		if vecs, err := e.fetch(e.queries, qs, texts); err == nil {
			for i, q := range qs {
				if err := r.Store.putQueryVec(e.Model, q, vecs[i]); err != nil {
					log.Printf("feedback: %v", err)
				}
			}
		}
	}
	if len(pages) > 0 {
		ids, texts := make([]string, len(pages)), make([]string, len(pages))
		for i, p := range pages {
			ids[i], texts[i] = p.key, e.pageText(p.title, p.snippet)
		}
		if vecs, err := e.fetch(e.pages, ids, texts); err == nil {
			for i, id := range ids {
				if err := r.Store.putPageVec(e.Model, id, vecs[i]); err != nil {
					log.Printf("feedback: %v", err)
				}
			}
		}
	}
}

// Prepare starts fetching q's embedding so it's ready when results arrive.
func (r *Ranker) Prepare(q string) {
	if r.Emb == nil || len(r.Store.snapshot()) == 0 {
		return
	}
	r.Emb.Query(normQuery(q))
	go r.Backfill()
}

// Rerank multiplies each result's score by 2^adj and re-sorts. Every vote counts
// on every search; how similar the vote's query is to q scales it from
// globalReach to 1. adj sums three kinds of votes:
//   - votes on this exact page,
//   - votes on other pages of the same site,
//   - votes on other pages whose embedding is close to this page's (DuckDB vector search).
//
// It sets Vote (the user's vote for this exact query) and Personal (adj) on each result.
func (r *Ranker) Rerank(q engine.Query, _ engine.Category, results []engine.Result) {
	votes := r.Store.snapshot()
	if len(votes) == 0 || len(results) == 0 {
		return
	}
	nq := normQuery(q.Text)
	qsim := r.querySims(nq, votes)
	reach := func(vq string) float64 { return globalReach + (1-globalReach)*qsim[vq] }
	near := r.nearPages(results)
	byPage := map[string][]*vote{}
	for _, v := range votes {
		byPage[v.key] = append(byPage[v.key], v)
	}

	changed := false
	for i := range results {
		res := &results[i]
		key, host := r.Store.key(res.URL), hostOf(res.URL)
		var page, site, content float64
		for _, v := range votes {
			switch {
			case v.key == key:
				page += pageWeight * reach(v.q) * float64(v.Dir)
				if v.q == nq {
					res.Vote = v.Dir
				}
			case host != "" && v.host == host:
				site += siteWeight * reach(v.q) * float64(v.Dir)
			}
		}
		for _, m := range near[key] {
			c := (m.cos - pageSim) / (1 - pageSim)
			for _, v := range byPage[m.key] {
				content += contentWeight * c * reach(v.q) * float64(v.Dir)
			}
		}
		adj := clamp(page, maxPage) + clamp(site, maxSite) + clamp(content, maxContent)
		if adj != 0 {
			res.Score *= math.Exp2(adj)
			res.Personal = math.Round(adj*100) / 100
			changed = true
		}
	}
	if changed {
		sort.SliceStable(results, func(i, j int) bool { return results[i].Score > results[j].Score })
	}
}

// querySims scores each voted query against nq in [0, 1]: the better of word
// overlap (Jaccard) and embedding cosine rescaled from [querySim, 1].
func (r *Ranker) querySims(nq string, votes []*vote) map[string]float64 {
	out := map[string]float64{nq: 1}
	for _, v := range votes {
		if _, ok := out[v.q]; !ok {
			out[v.q] = jaccard(nq, v.q)
		}
	}
	if r.Emb == nil {
		return out
	}
	qv, ok := r.Emb.Query(nq)
	if !ok {
		return out
	}
	cos, err := r.Store.similarQueries(r.Emb.Model, qv, querySim)
	if err != nil {
		log.Printf("feedback: %v", err)
		return out
	}
	for q, c := range cos {
		out[q] = min(max(out[q], (c-querySim)/(1-querySim)), 1)
	}
	return out
}

// nearPages maps result keys to voted pages whose embedding is close to theirs.
func (r *Ranker) nearPages(results []engine.Result) map[string][]pageMatch {
	if r.Emb == nil {
		return nil
	}
	docs := make([]pageDoc, len(results))
	for i, res := range results {
		docs[i] = pageDoc{r.Store.key(res.URL), res.Title, res.Snippet}
	}
	vecs := r.Emb.Pages(docs)
	if len(vecs) == 0 {
		return nil
	}
	keys := make([]string, 0, len(vecs))
	vs := make([][]float32, 0, len(vecs))
	for k, v := range vecs {
		keys, vs = append(keys, k), append(vs, v)
	}
	near, err := r.Store.similarPages(r.Emb.Model, keys, vs, pageSim)
	if err != nil {
		log.Printf("feedback: %v", err)
		return nil
	}
	return near
}

func jaccard(a, b string) float64 {
	set := map[string]int{}
	for _, w := range strings.Fields(a) {
		set[w] |= 1
	}
	for _, w := range strings.Fields(b) {
		set[w] |= 2
	}
	if len(set) == 0 {
		return 0
	}
	both := 0
	for _, m := range set {
		if m == 3 {
			both++
		}
	}
	return float64(both) / float64(len(set))
}

func clamp(x, lim float64) float64 { return min(max(x, -lim), lim) }
