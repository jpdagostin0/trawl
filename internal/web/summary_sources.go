package web

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/jpdagostin0/trawl/internal/engine"
)

const maxArticlePromptBytes = 12_000

type summarySource struct {
	Title   string `json:"title"`
	Snippet string `json:"snippet,omitempty"`
	Article string `json:"article,omitempty"`
	Clipped bool   `json:"article_truncated,omitempty"`
}

// summarySources preserves search order (so [1] still points at result one),
// fetching readable content concurrently and falling back to snippets.
func (s *Server) summarySources(ctx context.Context, results []engine.Result) []summarySource {
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	f := s.SummaryFetch
	if f == nil {
		f = s.F
	}
	sources := make([]summarySource, min(len(results), summaryResults))
	var work sync.WaitGroup
	for i := range sources {
		r := results[i]
		source := &sources[i]
		source.Title = clipText(r.Title, 240)
		source.Snippet = clipText(r.Snippet, 600)
		if f == nil {
			continue
		}
		work.Go(func() {
			page, err := f.ScrapeAllowed(ctx, r.URL)
			if err != nil {
				return
			}
			text := strings.TrimSpace(page.Text)
			if text == "" {
				text = strings.TrimSpace(page.Markdown)
			}
			if text == "" {
				return
			}
			source.Article, source.Clipped = articleExcerpt(text)
			source.Snippet = ""
			if s.Archive != nil && ctx.Err() == nil {
				body := page.Markdown
				if body == "" {
					body = text
				}
				if _, err := s.Archive.Save(page.URL, page.Title, body, page.FetchedAt); err != nil {
					log.Printf("archive: %v", err)
				}
			}
		})
	}
	work.Wait()
	return sources
}

func articleExcerpt(text string) (string, bool) {
	if len(text) <= maxArticlePromptBytes {
		return text, false
	}
	end := maxArticlePromptBytes
	for !utf8.RuneStart(text[end]) {
		end--
	}
	return text[:end], true
}
