package web

import (
	"context"
	"errors"
	"net/http"
	"time"

	lru "github.com/hashicorp/golang-lru/v2/expirable"

	"github.com/jpdagostin0/trawl/internal/engine"
)

var wikiCache = lru.NewLRU[string, *engine.WikiArticle](256, nil, 6*time.Hour)

// apiWiki returns the expanded Wikipedia panel for an article URL from the results.
func (s *Server) apiWiki(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("url")
	lang, title, err := engine.WikiTitle(raw)
	if err != nil {
		apiErr(w, http.StatusBadRequest, err.Error())
		return
	}
	key := lang + ":" + title
	a, ok := wikiCache.Get(key)
	if !ok {
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		a, err = engine.FetchWikiArticle(ctx, raw)
		if errors.Is(err, engine.ErrNotWikipedia) {
			apiErr(w, http.StatusNotFound, "no article panel for this page")
			return
		}
		if err != nil {
			apiErr(w, http.StatusBadGateway, err.Error())
			return
		}
		wikiCache.Add(key, a)
	}
	w.Header().Set("Cache-Control", "private, max-age=3600")
	writeJSON(w, http.StatusOK, a)
}
