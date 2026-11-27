package feedback

import (
	"math"
	"path/filepath"
	"strings"
	"testing"
	"time"

	lru "github.com/hashicorp/golang-lru/v2/expirable"

	"github.com/jpdagostin0/trawl/internal/engine"
)

func key(u string) string {
	u = strings.TrimPrefix(strings.TrimPrefix(u, "https://"), "http://")
	return strings.TrimSuffix(u, "/")
}

func results(urls ...string) []engine.Result {
	out := make([]engine.Result, len(urls))
	for i, u := range urls {
		out[i] = engine.Result{URL: u, Title: u, Score: 1 / float64(60+i)}
	}
	return out
}

func order(rs []engine.Result) string {
	var s []string
	for _, r := range rs {
		s = append(s, key(r.URL))
	}
	return strings.Join(s, " ")
}

func openStore(t *testing.T, path string) *Store {
	t.Helper()
	st, err := Open(path, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

// offlineEmbedder serves only what's put in its caches; fetches fail as if
// Ollama were down, so tests never touch the network.
func offlineEmbedder() *Embedder {
	return &Embedder{
		Model:     "test",
		queries:   lru.NewLRU[string, []float32](100, nil, time.Hour),
		pages:     lru.NewLRU[string, []float32](100, nil, time.Hour),
		downUntil: time.Now().Add(time.Hour),
	}
}

// unit returns a 2-D unit vector at the given cosine to (1, 0).
func unit(cos float64) []float32 { return []float32{float32(cos), float32(math.Sqrt(1 - cos*cos))} }

func TestVotesRerankAndPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trawl.duckdb")
	st := openStore(t, path)
	rk := &Ranker{Store: st}
	q := engine.Query{Text: "Golang  HTTP server"}
	urls := []string{"https://a.com/1", "https://b.com/1", "https://c.com/1", "https://d.com/1"}
	for _, v := range []Vote{
		{Query: "golang http server", URL: "https://c.com/1", Dir: 1, Engines: []string{"bing", "startpage"}},
		{Query: "golang http server", URL: "https://a.com/1", Dir: -1},
	} {
		if err := rk.Vote(v); err != nil {
			t.Fatal(err)
		}
	}
	rs := results(urls...)
	rk.Rerank(q, engine.General, rs)
	if got := order(rs); got != "c.com/1 b.com/1 d.com/1 a.com/1" {
		t.Fatalf("order = %s", got)
	}
	if rs[0].Vote != 1 || rs[3].Vote != -1 || rs[0].Personal <= 0 {
		t.Errorf("vote fields: %+v %+v", rs[0], rs[3])
	}

	// Reopen: votes come back from DuckDB; clearing a vote removes it.
	st.Close()
	st = openStore(t, path)
	vs := st.Votes()
	if len(vs) != 2 {
		t.Fatalf("reloaded %d votes, want 2", len(vs))
	}
	for _, v := range vs {
		if v.URL == "https://c.com/1" && strings.Join(v.Engines, ",") != "bing,startpage" {
			t.Errorf("engines = %v", v.Engines)
		}
	}
	rk = &Ranker{Store: st}
	rk.Vote(Vote{Query: "golang http server", URL: "https://a.com/1", Dir: 0})
	st.Close()
	rk = &Ranker{Store: openStore(t, path)}
	rs = results(urls...)
	rk.Rerank(q, engine.General, rs)
	if got := order(rs); got != "c.com/1 a.com/1 b.com/1 d.com/1" {
		t.Errorf("after clear, order = %s", got)
	}
	var events int
	rk.Store.db.QueryRow(`SELECT count(*) FROM votes`).Scan(&events)
	if events != 3 {
		t.Errorf("votes table has %d events, want all 3", events)
	}
}

func TestWordOverlapAndSites(t *testing.T) {
	rk := &Ranker{Store: openStore(t, filepath.Join(t.TempDir(), "v.duckdb"))}
	rk.Vote(Vote{Query: "golang http server", URL: "https://c.com/1", Dir: 1})
	rk.Vote(Vote{Query: "cake recipe", URL: "https://spam.com/x", Dir: -1})

	rs := results("https://a.com/1", "https://b.com/1", "https://c.com/1")
	rk.Rerank(engine.Query{Text: "golang http router"}, engine.General, rs)
	if rs[0].URL != "https://c.com/1" || rs[0].Vote != 0 {
		t.Errorf("similar query: %s, vote %d", order(rs), rs[0].Vote)
	}

	// Unrelated query: no page boost, but the site downvote still nudges spam.com down.
	rs = results("https://spam.com/other", "https://b.com/1")
	rk.Rerank(engine.Query{Text: "weather boston"}, engine.General, rs)
	if rs[0].URL != "https://b.com/1" {
		t.Errorf("site vote: %s", order(rs))
	}
}

func TestVectorSearch(t *testing.T) {
	st := openStore(t, filepath.Join(t.TempDir(), "v.duckdb"))
	emb := offlineEmbedder()
	rk := &Ranker{Store: st, Emb: emb}
	rk.Vote(Vote{Query: "golang http server", URL: "https://liked.com/post", Title: "liked", Dir: 1})

	qs, pages, err := st.missingVecs("test")
	if err != nil || len(qs) != 1 || len(pages) != 1 || pages[0].title != "liked" {
		t.Fatalf("missingVecs = %v %v %v", qs, pages, err)
	}
	// The voted query and page sit on the x axis.
	if err := st.putQueryVec("test", "golang http server", unit(1)); err != nil {
		t.Fatal(err)
	}
	if err := st.putPageVec("test", "liked.com/post", unit(1)); err != nil {
		t.Fatal(err)
	}
	if qs, pages, _ := st.missingVecs("test"); len(qs)+len(pages) != 0 {
		t.Errorf("still missing %v %v", qs, pages)
	}

	// No shared words with the voted query, but cosine 0.9: the liked page itself moves up.
	emb.queries.Add("go net/http tutorial", unit(0.9))
	rs := results("https://a.com/", "https://b.com/", "https://liked.com/post")
	rk.Rerank(engine.Query{Text: "go net/http tutorial"}, engine.General, rs)
	if rs[0].URL != "https://liked.com/post" {
		t.Errorf("semantic query match: %s", order(rs))
	}
	if qs, _ := st.similarQueries("test", unit(0.9), querySim); math.Abs(qs["golang http server"]-0.9) > 1e-3 {
		t.Errorf("similarQueries = %v", qs)
	}

	// A different page whose text embedding is close to the liked page moves up;
	// one that's far away doesn't.
	emb.queries.Add("something else", unit(0))
	emb.pages.Add("twin.org/article", unit(0.95))
	emb.pages.Add("far.org/x", unit(0.3))
	rs = results("https://a.com/", "https://far.org/x", "https://twin.org/article")
	rk.Rerank(engine.Query{Text: "something else"}, engine.General, rs)
	if rs[0].URL != "https://twin.org/article" || rs[0].Personal <= 0 {
		t.Errorf("content match: %s %+v", order(rs), rs[0])
	}
	for _, r := range rs {
		if r.URL == "https://far.org/x" && r.Personal != 0 {
			t.Errorf("far page was boosted: %+v", r)
		}
	}
}

func TestRecordValidates(t *testing.T) {
	st := openStore(t, filepath.Join(t.TempDir(), "v.duckdb"))
	for _, v := range []Vote{{Query: "", URL: "https://a.com", Dir: 1}, {Query: "x", URL: "https://a.com", Dir: 2}} {
		if st.Record(v) == nil {
			t.Errorf("accepted %+v", v)
		}
	}
}
