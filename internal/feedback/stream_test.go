package feedback

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/jpdagostin0/trawl/internal/engine"
	"github.com/jpdagostin0/trawl/internal/search"
)

type instantEngine struct{}

func (instantEngine) Name() string              { return "test" }
func (instantEngine) Category() engine.Category { return engine.General }
func (instantEngine) Weight() float64           { return 1 }
func (instantEngine) Search(context.Context, engine.Query) ([]engine.Result, error) {
	return []engine.Result{
		{URL: "https://unrelated.example/", Title: "Unrelated"},
		{URL: "https://new.example/", Title: "Similar page"},
	}, nil
}

func TestColdSearchUsesGlobalContentVotesBeforeStreamEnds(t *testing.T) {
	for _, dir := range []int{1, -1} {
		name := "upvote"
		if dir < 0 {
			name = "downvote"
		}
		t.Run(name, func(t *testing.T) {
			// Release the model only once the stream displays its first results.
			ready := make(chan struct{})
			ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				var input struct{ Input []string }
				if err := json.NewDecoder(req.Body).Decode(&input); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				select {
				case <-ready:
				case <-req.Context().Done():
					return
				}
				var vectors [][]float32
				for _, text := range input.Input {
					vec := unit(0)
					switch text {
					case "Similar page\n":
						vec = unit(0.95)
					case "Voted page\n":
						// The voted page can finish indexing after the new results.
						time.Sleep(150 * time.Millisecond)
						vec = unit(1)
					case "original query":
						vec = unit(1)
					}
					vectors = append(vectors, vec)
				}
				json.NewEncoder(w).Encode(map[string]any{"embeddings": vectors})
			}))
			defer ollama.Close()
			t.Setenv("OLLAMA_HOST", ollama.URL)
			st := openStore(t, filepath.Join(t.TempDir(), "trawl.duckdb"))
			emb, err := NewEmbedder("test")
			if err != nil {
				t.Fatal(err)
			}
			ranker := &Ranker{Store: st, Emb: emb}
			if err := ranker.Vote(Vote{Query: "original query", URL: "https://voted.example/", Title: "Voted page", Dir: dir}); err != nil {
				t.Fatal(err)
			}
			s := search.New([]engine.Engine{instantEngine{}}, time.Second)
			s.Rerank = ranker
			var snapshots []*search.Response
			s.Stream(context.Background(), engine.Query{Text: "different topic", Page: 1}, engine.General, nil, func(r *search.Response) {
				if len(snapshots) == 0 {
					close(ready)
				}
				snapshots = append(snapshots, r)
			})
			last := snapshots[len(snapshots)-1]
			for _, r := range last.Results {
				if r.URL == "https://new.example/" && (!last.Done || r.Personal*float64(dir) <= 0) {
					t.Fatalf("first search finished without applying global content vote: %+v", r)
				}
			}
			if len(snapshots) < 2 || snapshots[0].Done || (dir > 0 && last.Results[0].URL != "https://new.example/") {
				t.Fatalf("expected immediate results followed by personalized final results, got %d snapshots", len(snapshots))
			}
		})
	}
}
