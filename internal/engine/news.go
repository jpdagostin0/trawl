package engine

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ---- Bing News (RSS) ----

type BingNews struct{}

func (*BingNews) Name() string       { return "bing news" }
func (*BingNews) Category() Category { return News }
func (*BingNews) Weight() float64    { return 1.0 }

func (e *BingNews) Search(ctx context.Context, q Query) ([]Result, error) {
	v := url.Values{"q": {q.Text}, "format": {"rss"}, "first": {strconv.Itoa((q.Page-1)*10 + 1)}}
	req, err := newReq(ctx, "GET", "https://www.bing.com/news/search?"+v.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var feed struct {
		Items []struct {
			Title       string `xml:"title"`
			Link        string `xml:"link"`
			Description string `xml:"description"`
			PubDate     string `xml:"pubDate"`
			Src         string `xml:"Source"` // News:Source; the namespace URI varies per query
			Image       string `xml:"Image"`
		} `xml:"channel>item"`
	}
	if err := xml.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&feed); err != nil {
		return nil, err
	}
	out := make([]Result, 0, len(feed.Items))
	for _, it := range feed.Items {
		link := it.Link
		// apiclick.aspx?...&url=<real article>
		if u, err := url.Parse(link); err == nil && strings.HasSuffix(u.Path, "/apiclick.aspx") {
			if real := u.Query().Get("url"); real != "" {
				link = real
			}
		}
		r := Result{URL: link, Title: clean(it.Title), Snippet: stripTags(it.Description), Source: clean(it.Src)}
		if t, err := time.Parse(time.RFC1123, it.PubDate); err == nil {
			r.Published = t.UTC().Format(time.RFC3339)
		}
		if it.Image != "" {
			r.Thumbnail = strings.Replace(it.Image, "http://", "https://", 1) + "&w=160&h=120&c=14"
		}
		out = append(out, r)
	}
	return out, nil
}

// ---- DuckDuckGo News (news.js, needs a vqd token from the HTML page) ----

type DuckDuckGoNews struct {
	mu  sync.Mutex
	vqd map[string]string // query -> token; tokens are per query
}

func (*DuckDuckGoNews) Name() string       { return "duckduckgo news" }
func (*DuckDuckGoNews) Category() Category { return News }
func (*DuckDuckGoNews) Weight() float64    { return 0.9 }

var vqdRe = regexp.MustCompile(`vqd=["']?([0-9-]+)`)

func (e *DuckDuckGoNews) token(ctx context.Context, q string) (string, error) {
	e.mu.Lock()
	tok, ok := e.vqd[q]
	e.mu.Unlock()
	if ok {
		return tok, nil
	}
	req, err := newReq(ctx, "GET", "https://duckduckgo.com/?ia=news&q="+url.QueryEscape(q), nil)
	if err != nil {
		return "", err
	}
	resp, err := do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	m := vqdRe.FindSubmatch(body)
	if m == nil {
		return "", ErrBlocked{"duckduckgo news"}
	}
	e.mu.Lock()
	if e.vqd == nil || len(e.vqd) > 512 {
		e.vqd = map[string]string{}
	}
	e.vqd[q] = string(m[1])
	e.mu.Unlock()
	return string(m[1]), nil
}

func (e *DuckDuckGoNews) Search(ctx context.Context, q Query) ([]Result, error) {
	tok, err := e.token(ctx, q.Text)
	if err != nil {
		return nil, err
	}
	v := url.Values{"l": {"us-en"}, "o": {"json"}, "noamp": {"1"}, "q": {q.Text}, "vqd": {tok}, "p": {"-1"}, "s": {strconv.Itoa((q.Page - 1) * 30)}}
	var r struct {
		Results []struct {
			Date    int64  `json:"date"`
			Excerpt string `json:"excerpt"`
			Image   string `json:"image"`
			Source  string `json:"source"`
			Title   string `json:"title"`
			URL     string `json:"url"`
		} `json:"results"`
	}
	req, err := newReq(ctx, "GET", "https://duckduckgo.com/news.js?"+v.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Referer", "https://duckduckgo.com/")
	resp, err := do(req)
	if err != nil {
		e.mu.Lock()
		delete(e.vqd, q.Text) // token may have expired
		e.mu.Unlock()
		return nil, err
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&r); err != nil {
		return nil, fmt.Errorf("duckduckgo news: %w", err)
	}
	out := make([]Result, 0, len(r.Results))
	for _, x := range r.Results {
		if x.URL == "" {
			continue
		}
		res := Result{URL: x.URL, Title: stripTags(x.Title), Snippet: stripTags(x.Excerpt), Source: clean(x.Source), Thumbnail: x.Image}
		if x.Date > 0 {
			res.Published = time.Unix(x.Date, 0).UTC().Format(time.RFC3339)
		}
		out = append(out, res)
	}
	return out, nil
}
