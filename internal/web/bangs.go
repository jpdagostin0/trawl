package web

import (
	"net/url"
	"strings"

	"github.com/jpdagostin0/trawl/internal/engine"
)

// External bangs, DuckDuckGo-style. %s is replaced by the escaped query.
var bangs = map[string]string{
	"g":     "https://www.google.com/search?q=%s",
	"ddg":   "https://duckduckgo.com/?q=%s",
	"b":     "https://www.bing.com/search?q=%s",
	"w":     "https://en.wikipedia.org/w/index.php?search=%s",
	"gh":    "https://github.com/search?type=repositories&q=%s",
	"yt":    "https://www.youtube.com/results?search_query=%s",
	"r":     "https://www.reddit.com/search/?q=%s",
	"so":    "https://stackoverflow.com/search?q=%s",
	"hn":    "https://hn.algolia.com/?q=%s",
	"aw":    "https://wiki.archlinux.org/index.php?search=%s",
	"m":     "https://www.openstreetmap.org/search?query=%s",
	"a":     "https://www.amazon.com/s?k=%s",
	"mdn":   "https://developer.mozilla.org/en-US/search?q=%s",
	"pkg":   "https://pkg.go.dev/search?q=%s",
	"crate": "https://crates.io/search?q=%s",
	"npm":   "https://www.npmjs.com/search?q=%s",
	"pypi":  "https://pypi.org/search/?q=%s",
	"arxiv": "https://arxiv.org/a/%s",
	"wa":    "https://www.wolframalpha.com/input?i=%s",
	"tr":    "https://translate.google.com/?sl=auto&tl=en&text=%s",
}

// Category bangs keep you in trawl but switch tabs.
var catBangs = map[string]engine.Category{
	"code": engine.Code, "c": engine.Code, "forums": engine.Forums, "f": engine.Forums, "web": engine.General,
	"images": engine.Images, "i": engine.Images, "news": engine.News, "n": engine.News, "videos": engine.Videos, "v": engine.Videos,
}

type bangRest struct {
	query string
	cat   engine.Category
}

// bang finds a !bang token anywhere in q. It returns an external redirect target,
// or (for category bangs) the stripped query + category.
func bang(q string) (target string, rest bangRest, ok bool) {
	fields := strings.Fields(q)
	for i, f := range fields {
		if len(f) < 2 || f[0] != '!' {
			continue
		}
		name := strings.ToLower(f[1:])
		remaining := strings.Join(append(append([]string{}, fields[:i]...), fields[i+1:]...), " ")
		if tmpl, found := bangs[name]; found {
			if remaining == "" { // bare "!gh" goes to the site itself
				u, _ := url.Parse(tmpl)
				return u.Scheme + "://" + u.Host, rest, true
			}
			return strings.Replace(tmpl, "%s", url.QueryEscape(remaining), 1), rest, true
		}
		if cat, found := catBangs[name]; found && remaining != "" {
			return "", bangRest{remaining, cat}, true
		}
	}
	return "", rest, false
}
