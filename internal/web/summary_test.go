package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jpdagostin0/trawl/internal/engine"
	"github.com/jpdagostin0/trawl/internal/search"
)

type summaryEngine struct{}

func (summaryEngine) Name() string              { return "fixture" }
func (summaryEngine) Category() engine.Category { return engine.General }
func (summaryEngine) Weight() float64           { return 1 }
func (summaryEngine) Search(context.Context, engine.Query) ([]engine.Result, error) {
	return []engine.Result{{URL: "https://example.com/guide", Title: "A helpful guide", Snippet: "Use a wooden spoon."}}, nil
}

func TestSearchStreamsResultsThenSummaryTokens(t *testing.T) {
	requests := make(chan string, 8)
	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model    string
			Think    *bool
			Stream   bool
			Messages []struct{ Content string }
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Messages) != 2 || req.Think == nil || *req.Think || !req.Stream {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		requests <- req.Model + " " + req.Messages[1].Content
		fmt.Fprintln(w, `{"message":{"role":"assistant","content":"Use a"},"done":false}`)
		fmt.Fprintln(w, `{"message":{"role":"assistant","content":" wooden spoon [1]."},"done":false}`)
		fmt.Fprintln(w, `{"message":{"role":"assistant","content":""},"done":true}`)
	}))
	defer ollama.Close()
	t.Setenv("OLLAMA_HOST", ollama.URL)
	s := New(search.New([]engine.Engine{summaryEngine{}}, time.Second), nil, "")
	var err error
	s.Summary, err = NewSummarizer("lfm3:1.7b")
	if err != nil {
		t.Fatal(err)
	}

	get := func(suffix string) []map[string]any {
		t.Helper()
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/search?q=wooden+spoon&stream=1"+suffix, nil))
		var snapshots []map[string]any
		for line := range strings.SplitSeq(strings.TrimSpace(w.Body.String()), "\n") {
			var data map[string]any
			if err := json.Unmarshal([]byte(line), &data); err != nil {
				t.Fatal(err)
			}
			snapshots = append(snapshots, data)
		}
		return snapshots
	}
	if got := get("&summary=1"); len(got) != 1 || got[0]["summary"] != nil || len(requests) != 0 {
		t.Fatalf("disabled summaries called Ollama or changed the response: %v", got)
	}
	s.Summary.Enabled.Store(true)
	if got := get(""); len(got) != 1 || len(requests) != 0 {
		t.Fatalf("existing stream changed without summary opt-in: %v", got)
	}
	got := get("&summary=1")
	if len(got) != 4 || got[0]["done"] != true || got[0]["summary_pending"] != true || got[0]["summary"] != nil {
		t.Fatalf("want final search results before the summary tokens: %v", got)
	}
	if results, ok := got[0]["results"].([]any); !ok || len(results) != 1 {
		t.Fatalf("first frame lost the search results: %v", got[0])
	}
	for i, want := range []string{"Use a", "Use a wooden spoon [1].", "Use a wooden spoon [1]."} {
		frame := got[i+1]
		if frame["summary"] != want || frame["summary_pending"] != (i < 2) || frame["results"] != nil {
			t.Fatalf("unexpected summary frame %d: %v", i, frame)
		}
	}
	if len(requests) != 1 {
		t.Fatalf("got %d Ollama requests", len(requests))
	}
	if req := <-requests; !strings.Contains(req, "lfm3:1.7b") || !strings.Contains(req, "Use a wooden spoon.") {
		t.Fatalf("Ollama request missing model or source snippet: %s", req)
	}
}
