package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func getJSON(ctx context.Context, u string, v any) error {
	req, err := newReq(ctx, "GET", u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(v)
}

var tagRe = regexp.MustCompile(`<[^>]+>`)

func stripTags(s string) string { return clean(html.UnescapeString(tagRe.ReplaceAllString(s, ""))) }

func wikiLang(q Query) string {
	if q.Lang != "" && len(q.Lang) <= 3 {
		return q.Lang
	}
	return "en"
}

// ---- Wikipedia ----

type Wikipedia struct{}

func (*Wikipedia) Name() string       { return "wikipedia" }
func (*Wikipedia) Category() Category { return General }
func (*Wikipedia) Weight() float64    { return 0.8 }

func (e *Wikipedia) Search(ctx context.Context, q Query) ([]Result, error) {
	v := url.Values{"action": {"query"}, "list": {"search"}, "format": {"json"}, "srsearch": {q.Text},
		"srlimit": {"5"}, "sroffset": {strconv.Itoa((q.Page - 1) * 5)}}
	var r struct {
		Query struct {
			Search []struct{ Title, Snippet string } `json:"search"`
		} `json:"query"`
	}
	lang := wikiLang(q)
	if err := getJSON(ctx, "https://"+lang+".wikipedia.org/w/api.php?"+v.Encode(), &r); err != nil {
		return nil, err
	}
	out := make([]Result, 0, len(r.Query.Search))
	for _, s := range r.Query.Search {
		out = append(out, Result{
			URL:     "https://" + lang + ".wikipedia.org/wiki/" + url.PathEscape(strings.ReplaceAll(s.Title, " ", "_")),
			Title:   s.Title + " - Wikipedia",
			Snippet: stripTags(s.Snippet),
		})
	}
	return out, nil
}

func (e *Wikipedia) Infobox(ctx context.Context, q Query) (*Infobox, error) {
	lang := wikiLang(q)
	// opensearch resolves the best-matching title (handles redirects/typos better than exact lookup)
	var os []any
	v := url.Values{"action": {"opensearch"}, "search": {q.Text}, "limit": {"1"}, "namespace": {"0"}, "format": {"json"}}
	if err := getJSON(ctx, "https://"+lang+".wikipedia.org/w/api.php?"+v.Encode(), &os); err != nil {
		return nil, err
	}
	if len(os) < 2 {
		return nil, nil
	}
	titles, _ := os[1].([]any)
	if len(titles) == 0 {
		return nil, nil
	}
	title, _ := titles[0].(string)
	var s struct {
		Type, Title, Extract string
		Thumbnail            struct{ Source string } `json:"thumbnail"`
		ContentURLs          struct {
			Desktop struct{ Page string } `json:"desktop"`
		} `json:"content_urls"`
	}
	if err := getJSON(ctx, "https://"+lang+".wikipedia.org/api/rest_v1/page/summary/"+url.PathEscape(strings.ReplaceAll(title, " ", "_")), &s); err != nil {
		return nil, err
	}
	if s.Type == "disambiguation" || s.Extract == "" {
		return nil, nil
	}
	return &Infobox{Title: s.Title, Extract: s.Extract, URL: s.ContentURLs.Desktop.Page, Image: s.Thumbnail.Source}, nil
}

// ---- GitHub repositories ----

type GitHub struct{}

func (*GitHub) Name() string       { return "github" }
func (*GitHub) Category() Category { return Code }
func (*GitHub) Weight() float64    { return 1.0 }

func (e *GitHub) Search(ctx context.Context, q Query) ([]Result, error) {
	v := url.Values{"q": {q.Text}, "per_page": {"10"}, "page": {strconv.Itoa(q.Page)}}
	var r struct {
		Items []struct {
			FullName    string `json:"full_name"`
			HTMLURL     string `json:"html_url"`
			Description string `json:"description"`
			Stars       int    `json:"stargazers_count"`
			Language    string `json:"language"`
			PushedAt    string `json:"pushed_at"`
		} `json:"items"`
	}
	if err := getJSON(ctx, "https://api.github.com/search/repositories?"+v.Encode(), &r); err != nil {
		return nil, err
	}
	out := make([]Result, 0, len(r.Items))
	for _, it := range r.Items {
		meta := fmt.Sprintf("★ %s", humanInt(it.Stars))
		if it.Language != "" {
			meta += " · " + it.Language
		}
		if t, err := time.Parse(time.RFC3339, it.PushedAt); err == nil {
			meta += " · updated " + t.Format("Jan 2006")
		}
		out = append(out, Result{URL: it.HTMLURL, Title: it.FullName, Snippet: it.Description, Meta: meta})
	}
	return out, nil
}

// ---- Stack Overflow ----

type StackOverflow struct{}

func (*StackOverflow) Name() string       { return "stackoverflow" }
func (*StackOverflow) Category() Category { return Code }
func (*StackOverflow) Weight() float64    { return 1.0 }

func (e *StackOverflow) Search(ctx context.Context, q Query) ([]Result, error) {
	// /search/excerpts is the real full-text relevance search (search/advanced is loose).
	v := url.Values{"q": {q.Text}, "site": {"stackoverflow"}, "order": {"desc"}, "sort": {"relevance"},
		"pagesize": {"12"}, "page": {strconv.Itoa(q.Page)}}
	var r struct {
		Items []struct {
			Title       string `json:"title"`
			QuestionID  int    `json:"question_id"`
			Excerpt     string `json:"excerpt"`
			Score       int    `json:"question_score"`
			AnswerCount int    `json:"answer_count"`
			Accepted    bool   `json:"has_accepted_answer"`
		} `json:"items"`
	}
	if err := getJSON(ctx, "https://api.stackexchange.com/2.3/search/excerpts?"+v.Encode(), &r); err != nil {
		return nil, err
	}
	seen := map[int]bool{}
	out := make([]Result, 0, len(r.Items))
	for _, it := range r.Items {
		if seen[it.QuestionID] {
			continue
		}
		seen[it.QuestionID] = true
		meta := plural(it.Score, "vote") + " · " + plural(it.AnswerCount, "answer")
		if it.Accepted {
			meta += " · ✓ accepted"
		}
		out = append(out, Result{URL: fmt.Sprintf("https://stackoverflow.com/questions/%d", it.QuestionID),
			Title: html.UnescapeString(it.Title), Snippet: truncate(stripTags(it.Excerpt), 300), Meta: meta})
	}
	return out, nil
}

// ---- Hacker News (Algolia) ----

type HackerNews struct{}

func (*HackerNews) Name() string       { return "hackernews" }
func (*HackerNews) Category() Category { return Forums }
func (*HackerNews) Weight() float64    { return 1.0 }

func (e *HackerNews) Search(ctx context.Context, q Query) ([]Result, error) {
	v := url.Values{"query": {q.Text}, "tags": {"story"}, "hitsPerPage": {"15"}, "page": {strconv.Itoa(q.Page - 1)}}
	var r struct {
		Hits []struct {
			ObjectID    string `json:"objectID"`
			Title       string `json:"title"`
			URL         string `json:"url"`
			Points      int    `json:"points"`
			NumComments int    `json:"num_comments"`
			CreatedAt   string `json:"created_at"`
			StoryText   string `json:"story_text"`
		} `json:"hits"`
	}
	if err := getJSON(ctx, "https://hn.algolia.com/api/v1/search?"+v.Encode(), &r); err != nil {
		return nil, err
	}
	out := make([]Result, 0, len(r.Hits))
	for _, h := range r.Hits {
		disc := "https://news.ycombinator.com/item?id=" + h.ObjectID
		meta := fmt.Sprintf("%d points · %d comments", h.Points, h.NumComments)
		if t, err := time.Parse(time.RFC3339, h.CreatedAt); err == nil {
			meta += " · " + t.Format("Jan 2, 2006")
		}
		snip := truncate(stripTags(h.StoryText), 240)
		u := h.URL
		if u == "" {
			u = disc
		} else {
			if snip == "" {
				snip = "Discussion: " + disc
			}
		}
		out = append(out, Result{URL: u, Title: h.Title, Snippet: snip, Meta: meta})
	}
	return out, nil
}

func plural(n int, w string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, w)
	}
	return fmt.Sprintf("%d %ss", n, w)
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return strings.TrimSpace(string(r[:n])) + "…"
}

func humanInt(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1e6)
	case n >= 1000:
		return fmt.Sprintf("%.1fk", float64(n)/1e3)
	}
	return strconv.Itoa(n)
}
