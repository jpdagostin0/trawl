package web

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ollama/ollama/api"
)

func TestStreamChatRunsOneAtATime(t *testing.T) {
	var running, peak atomic.Int32
	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := running.Add(1)
		defer running.Add(-1)
		for p := peak.Load(); n > p && !peak.CompareAndSwap(p, n); p = peak.Load() {
		}
		time.Sleep(30 * time.Millisecond)
		fmt.Fprintln(w, `{"message":{"role":"assistant","content":"ok"},"done":true}`)
	}))
	defer ollama.Close()
	t.Setenv("OLLAMA_HOST", ollama.URL)
	s, err := NewSummarizer("test")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			if _, err := s.streamChat(context.Background(), []api.Message{{Role: "user", Content: "hi"}}, func(string) error { return nil }); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if p := peak.Load(); p != 1 {
		t.Fatalf("want one chat at a time, got %d concurrent", p)
	}
}

func TestStreamChatLeavesQueueWhenCanceled(t *testing.T) {
	s, err := NewSummarizer("test")
	if err != nil {
		t.Fatal(err)
	}
	s.queue.Acquire(context.Background(), 1)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := s.streamChat(ctx, nil, func(string) error { return nil }); err == nil {
		t.Fatal("want an error once the waiting request is canceled")
	}
}
