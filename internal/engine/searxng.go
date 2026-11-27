package engine

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

// SearXNG loads every engine of a SearXNG instance as an individual trawl engine.
// trawl asks the instance for one engine at a time (?engines=<name>&format=json), so
// per-engine ranking, timings and the circuit breaker keep working, while SearXNG's
// ~250 Python engine modules (Google, Yandex, YouTube, arXiv, ...) do the scraping.
type SearXNG struct {
	base   string
	name   string
	cat    Category
	weight float64
}

func (e *SearXNG) Name() string       { return e.name }
func (e *SearXNG) Category() Category { return e.cat }
func (e *SearXNG) Weight() float64    { return e.weight }

// SearXNG categories -> trawl tabs (first match wins, so "bing images" [images, web] -> images).
var sxCategoryMap = []struct {
	sx  []string
	cat Category
}{
	{[]string{"images", "stock images"}, Images},
	{[]string{"videos"}, Videos},
	{[]string{"news"}, News},
	{[]string{"it", "packages", "repos", "q&a", "software wikis"}, Code},
	{[]string{"social media"}, Forums},
	{[]string{"science", "scientific publications"}, Science},
	{[]string{"general", "web"}, General},
}

// sxWeights boosts engines with strong indexes.
var sxWeights = map[string]float64{"google": 1.2, "brave": 1.1, "google images": 1.1, "youtube": 1.1}

// LoadSearXNG registers engines from base's /config.
// which: "enabled" (instance defaults), "all", or a comma-separated list of names.
// Engines whose name collides with a native trawl engine are skipped.
func LoadSearXNG(ctx context.Context, base, which string, native []Engine) ([]Engine, error) {
	base = strings.TrimSuffix(base, "/")
	var cfg struct {
		Engines []struct {
			Name       string   `json:"name"`
			Categories []string `json:"categories"`
			Enabled    bool     `json:"enabled"`
		} `json:"engines"`
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := getJSON(ctx, base+"/config", &cfg); err != nil {
		return nil, fmt.Errorf("searxng %s/config: %w", base, err)
	}
	taken := map[string]bool{}
	for _, e := range native {
		taken[e.Name()] = true
	}
	var list []string
	if which != "" && which != "enabled" && which != "all" {
		list = strings.Split(which, ",")
	}
	var out []Engine
	for _, se := range cfg.Engines {
		switch {
		case taken[se.Name]:
			continue
		case list != nil && !slices.Contains(list, se.Name):
			continue
		case list == nil && which != "all" && !se.Enabled:
			continue
		}
		cat, ok := mapCategory(se.Categories)
		if !ok {
			continue // music, files, maps, ... have no tab
		}
		w := sxWeights[se.Name]
		if w == 0 {
			w = 0.8
		}
		out = append(out, &SearXNG{base: base, name: se.Name, cat: cat, weight: w})
	}
	log.Printf("searxng: loaded %d engines from %s", len(out), base)
	return out, nil
}

func mapCategory(cats []string) (Category, bool) {
	for _, m := range sxCategoryMap {
		for _, c := range cats {
			if slices.Contains(m.sx, c) {
				return m.cat, true
			}
		}
	}
	return "", false
}

func (e *SearXNG) Search(ctx context.Context, q Query) ([]Result, error) {
	v := url.Values{"q": {q.Text}, "format": {"json"}, "engines": {e.name}, "pageno": {strconv.Itoa(q.Page)}}
	if q.Lang != "" {
		v.Set("language", q.Lang)
	}
	var r struct {
		Results []struct {
			URL          string   `json:"url"`
			Title        string   `json:"title"`
			Content      string   `json:"content"`
			Thumbnail    string   `json:"thumbnail"`
			ThumbnailSrc string   `json:"thumbnail_src"`
			ImgSrc       string   `json:"img_src"`
			Resolution   string   `json:"resolution"`
			Published    string   `json:"publishedDate"`
			Author       string   `json:"author"`
			Template     string   `json:"template"`
		} `json:"results"`
		Unresponsive [][]string `json:"unresponsive_engines"`
	}
	if err := getJSON(ctx, e.base+"/search?"+v.Encode(), &r); err != nil {
		return nil, err
	}
	if len(r.Results) == 0 && len(r.Unresponsive) > 0 && len(r.Unresponsive[0]) > 1 {
		reason := r.Unresponsive[0][1]
		if strings.Contains(reason, "denied") || strings.Contains(reason, "CAPTCHA") || strings.Contains(reason, "too many") {
			return nil, ErrBlocked{e.name}
		}
		return nil, fmt.Errorf("%s", reason)
	}
	out := make([]Result, 0, len(r.Results))
	for _, x := range r.Results {
		res := Result{URL: x.URL, Title: clean(x.Title), Snippet: truncate(clean(x.Content), 320), Resolution: x.Resolution}
		thumb := x.ThumbnailSrc
		if thumb == "" {
			thumb = x.Thumbnail
		}
		if strings.HasPrefix(thumb, "//") {
			thumb = "https:" + thumb
		}
		res.Thumbnail = thumb
		if x.Template == "images.html" && x.ImgSrc != "" {
			res.Image = x.ImgSrc
			if res.Thumbnail == "" {
				res.Thumbnail = x.ImgSrc
			}
		}
		var meta []string
		if x.Author != "" {
			meta = append(meta, x.Author)
		}
		if t, err := time.Parse("2006-01-02T15:04:05", strings.SplitN(x.Published, ".", 2)[0]); err == nil {
			meta = append(meta, t.Format("Jan 2, 2006"))
		}
		res.Meta = strings.Join(meta, " · ")
		out = append(out, res)
	}
	return out, nil
}
