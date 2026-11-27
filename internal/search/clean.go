package search

import (
	"log"
	"sync"
	"time"

	"github.com/ddlsmurf/clearurls-go/clearurls"
	lru "github.com/hashicorp/golang-lru/v2/expirable"
)

// ClearURLs rules (https://docs.clearurls.xyz), compiled into the binary.
// github.com/ddlsmurf/clearurls-go is replaced by the jbisfun fork in go.mod
// (nil-regexp panic fix, rawRules and referralMarketing support).
var (
	clearOnce  sync.Once
	clearRules []clearurls.RunnableProvider
	cleanCache = lru.NewLRU[string, string](8192, nil, time.Hour)
)

func rules() []clearurls.RunnableProvider {
	clearOnce.Do(func() {
		p, err := clearurls.MustHaveHardcodedProviders()
		if err != nil {
			log.Printf("clearurls: %v (URLs will not be cleaned)", err)
		}
		clearRules = p
	})
	return clearRules
}

// CleanURL removes tracking parameters and unwraps redirect links using the
// ClearURLs rule set. It returns raw unchanged if cleaning fails, and also when
// ClearURLs would block the URL outright, because the result should still be shown.
func CleanURL(raw string) string {
	if c, ok := cleanCache.Get(raw); ok {
		return c
	}
	out := raw
	if c, err := clearurls.ClearURL(rules(), raw, false); err == nil && c != "" {
		out = c
	}
	cleanCache.Add(raw, out)
	return out
}
