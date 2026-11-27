package feedback

import (
	"context"
	"errors"
	"log"
	"strings"
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru/v2/expirable"
	"github.com/ollama/ollama/api"
	"golang.org/x/sync/singleflight"
)

// Embedder turns queries and result pages into vectors with an Ollama
// embedding model. The search stream shows results while vectors are computed.
type Embedder struct {
	Model  string
	client *api.Client
	gemma  bool // embeddinggemma is trained with task prompts

	queries *lru.LRU[string, []float32] // normalized query -> vector (memory only)
	pages   *lru.LRU[string, []float32] // page key -> vector (memory only; voted pages are also in DuckDB)
	group   singleflight.Group

	mu        sync.Mutex
	downUntil time.Time // Ollama unreachable or model missing: back off
}

// NewEmbedder uses OLLAMA_HOST (default 127.0.0.1:11434).
func NewEmbedder(model string) (*Embedder, error) {
	c, err := api.ClientFromEnvironment()
	if err != nil {
		return nil, err
	}
	return &Embedder{
		Model:   model,
		client:  c,
		gemma:   strings.HasPrefix(model, "embeddinggemma"),
		queries: lru.NewLRU[string, []float32](1024, nil, time.Hour),
		pages:   lru.NewLRU[string, []float32](8192, nil, time.Hour),
	}, nil
}

func (e *Embedder) queryText(q string) string {
	if e.gemma {
		return "task: sentence similarity | query: " + q
	}
	return q
}

func (e *Embedder) pageText(title, snippet string) string {
	if !e.gemma {
		return title + "\n" + snippet
	}
	if title == "" {
		title = "none"
	}
	return "title: " + title + " | text: " + snippet
}

// pending starts only missing work for the final set of merged results.
func (e *Embedder) pending(q string, docs []pageDoc) <-chan struct{} {
	var work sync.WaitGroup
	waiting := false
	if _, ok := e.queries.Get(q); !ok {
		waiting = true
		work.Go(func() { e.fetch(e.queries, []string{q}, []string{e.queryText(q)}) })
	}
	var ids, texts []string
	for _, d := range docs {
		if _, ok := e.pages.Get(d.key); !ok {
			ids = append(ids, d.key)
			texts = append(texts, e.pageText(d.title, d.snippet))
		}
	}
	if len(ids) > 0 {
		waiting = true
		work.Go(func() { e.fetch(e.pages, ids, texts) })
	}
	if !waiting {
		return nil
	}
	done := make(chan struct{})
	go func() {
		work.Wait()
		close(done)
	}()
	return done
}

// fetch embeds texts and caches them under ids. Identical concurrent batches
// (every streamed snapshot asks for the same pages) share one request.
func (e *Embedder) fetch(cache *lru.LRU[string, []float32], ids, texts []string) ([][]float32, error) {
	e.mu.Lock()
	down := time.Now().Before(e.downUntil)
	e.mu.Unlock()
	if down {
		return nil, errDown
	}
	prefix := "query:"
	if cache == e.pages {
		prefix = "page:"
	}
	v, err, _ := e.group.Do(prefix+strings.Join(ids, "\x00"), func() (any, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second) // covers a cold model load
		defer cancel()
		resp, err := e.client.Embed(ctx, &api.EmbedRequest{Model: e.Model, Input: texts})
		if err == nil && len(resp.Embeddings) != len(ids) {
			err = errShape
		}
		if err != nil {
			e.mu.Lock()
			if time.Now().After(e.downUntil) {
				log.Printf("feedback: embeddings unavailable (%s): %v; ranking without vectors for 5m", e.Model, err)
			}
			e.downUntil = time.Now().Add(5 * time.Minute)
			e.mu.Unlock()
			return nil, err
		}
		for i, id := range ids {
			cache.Add(id, resp.Embeddings[i])
		}
		return resp.Embeddings, nil
	})
	if err != nil {
		return nil, err
	}
	return v.([][]float32), nil
}

var (
	errShape = errors.New("embedding count mismatch")
	errDown  = errors.New("embeddings unavailable")
)
