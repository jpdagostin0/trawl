package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jpdagostin0/trawl/internal/archive"
	"github.com/jpdagostin0/trawl/internal/engine"
	"github.com/jpdagostin0/trawl/internal/fetch"
	"github.com/jpdagostin0/trawl/internal/search"
)

type summaryEngine struct{ url string }

func (summaryEngine) Name() string              { return "fixture" }
func (summaryEngine) Category() engine.Category { return engine.General }
func (summaryEngine) Weight() float64           { return 1 }
func (e summaryEngine) Search(context.Context, engine.Query) ([]engine.Result, error) {
	return []engine.Result{{URL: e.url + "/guide", Title: "A helpful guide", Snippet: "Use a wooden spoon."},
		{URL: e.url + "/private", Title: "Blocked guide", Snippet: "Snippet fallback."}}, nil
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
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Messages) < 2 || req.Think == nil || *req.Think || !req.Stream {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		var prompt []string
		for _, message := range req.Messages {
			prompt = append(prompt, message.Content)
		}
		requests <- req.Model + " " + strings.Join(prompt, "\n")
		fmt.Fprintln(w, `{"message":{"role":"assistant","content":"Use a"},"done":false}`)
		fmt.Fprintln(w, `{"message":{"role":"assistant","content":" wooden spoon [1]."},"done":false}`)
		fmt.Fprintln(w, `{"message":{"role":"assistant","content":""},"done":true}`)
	}))
	defer ollama.Close()
	t.Setenv("OLLAMA_HOST", ollama.URL)
	article := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/robots.txt":
			fmt.Fprintln(w, "User-agent: *\nDisallow: /private")
		case "/guide":
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprint(w, "Full article says to use a bamboo spoon and avoid metal.")
		default:
			t.Error("disallowed URL requested")
		}
	}))
	defer article.Close()
	s := New(search.New([]engine.Engine{summaryEngine{url: article.URL}}, time.Second), fetch.New(true), "")
	var err error
	s.Archive, err = archive.Open(filepath.Join(t.TempDir(), "archive"), archive.Capacity)
	if err != nil {
		t.Fatal(err)
	}
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
	if results, ok := got[0]["results"].([]any); !ok || len(results) != 2 {
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
	if req := <-requests; !strings.Contains(req, "lfm3:1.7b") || !strings.Contains(req, "Full article says to use a bamboo spoon") || !strings.Contains(req, "Snippet fallback.") {
		t.Fatalf("Ollama request missing model, article or fallback snippet: %s", req)
	}
	session, ok := got[len(got)-1]["summary_session"].(string)
	if !ok || session == "" {
		t.Fatal("completed summary did not offer a follow-up session")
	}
	followUp := func(question string) {
		t.Helper()
		body, err := json.Marshal(map[string]string{"session": session, "question": question})
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest("POST", "/api/summary/follow-up", strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("follow-up = %d %s", w.Code, w.Body.String())
		}
		dec := json.NewDecoder(w.Body)
		for i, want := range []string{"Use a", "Use a wooden spoon [1].", "Use a wooden spoon [1]."} {
			var frame struct {
				Answer string
				Done   bool
			}
			if err := dec.Decode(&frame); err != nil || frame.Answer != want || frame.Done != (i == 2) {
				t.Fatalf("follow-up frame %d = %+v (%v)", i, frame, err)
			}
		}
	}
	followUp("Why avoid metal?")
	if len(requests) != 1 {
		t.Fatal("follow-up did not call Ollama")
	}
	if req := <-requests; !strings.Contains(req, "Full article says to use a bamboo spoon") || !strings.Contains(req, "Why avoid metal?") || !strings.Contains(req, "Use a wooden spoon [1].") {
		t.Fatalf("first follow-up lost the original source or summary: %s", req)
	}
	followUp("Is there another option?")
	if len(requests) != 1 {
		t.Fatal("second follow-up did not call Ollama")
	}
	if req := <-requests; !strings.Contains(req, "Why avoid metal?") || !strings.Contains(req, "Is there another option?") {
		t.Fatalf("second follow-up lost conversation history: %s", req)
	}
	list := httptest.NewRecorder()
	s.Handler().ServeHTTP(list, httptest.NewRequest("GET", "/api/archive", nil))
	var saved struct{ Entries []archive.Entry }
	if err := json.Unmarshal(list.Body.Bytes(), &saved); err != nil || len(saved.Entries) != 1 {
		t.Fatalf("archive list = %s (%v)", list.Body.String(), err)
	}
	view := httptest.NewRecorder()
	s.Handler().ServeHTTP(view, httptest.NewRequest("GET", "/api/archive/"+saved.Entries[0].ID, nil))
	if view.Code != http.StatusOK || !strings.Contains(view.Body.String(), "Full article says to use a bamboo spoon") {
		t.Fatalf("archive view = %d %s", view.Code, view.Body.String())
	}
}
