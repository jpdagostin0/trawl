package engine

import (
	"context"
	"encoding/base64"
	"net/url"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// ---- Bing ----

type Bing struct{}

func (*Bing) Name() string       { return "bing" }
func (*Bing) Category() Category { return General }
func (*Bing) Weight() float64    { return 1.0 }

func (b *Bing) Search(ctx context.Context, q Query) ([]Result, error) {
	v := url.Values{"q": {q.Text}}
	if q.Page > 1 {
		v.Set("first", strconv.Itoa((q.Page-1)*10+1))
	}
	req, err := newReq(ctx, "GET", "https://www.bing.com/search?"+v.Encode(), nil)
	if err != nil {
		return nil, err
	}
	d, err := doc(req)
	if err != nil {
		return nil, err
	}
	var out []Result
	d.Find("li.b_algo").Each(func(_ int, s *goquery.Selection) {
		a := s.Find("h2 a").First()
		href, _ := a.Attr("href")
		u := bingUnwrap(href)
		if u == "" {
			return
		}
		snip := s.Find(".b_caption p").First()
		snip.Find("span.algoSlug_icon").Remove()
		out = append(out, Result{URL: u, Title: clean(a.Text()), Snippet: clean(snip.Text())})
	})
	if len(out) == 0 && d.Find("#b_captcha, .captcha").Length() > 0 {
		return nil, ErrBlocked{"bing"}
	}
	return out, nil
}

// bingUnwrap decodes bing.com/ck/a?...&u=a1<base64url> click-tracking links.
func bingUnwrap(href string) string {
	pu, err := url.Parse(href)
	if err != nil {
		return ""
	}
	if !strings.HasSuffix(pu.Host, "bing.com") || !strings.HasPrefix(pu.Path, "/ck/") {
		return href
	}
	enc := pu.Query().Get("u")
	if len(enc) < 3 {
		return ""
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(enc[2:], "="))
	if err != nil {
		return ""
	}
	return string(raw)
}

// ---- DuckDuckGo (HTML endpoint) ----

type DuckDuckGo struct{}

func (*DuckDuckGo) Name() string       { return "duckduckgo" }
func (*DuckDuckGo) Category() Category { return General }
func (*DuckDuckGo) Weight() float64    { return 1.0 }

func (e *DuckDuckGo) Search(ctx context.Context, q Query) ([]Result, error) {
	v := url.Values{"q": {q.Text}, "b": {""}}
	if q.Page > 1 {
		v.Set("s", strconv.Itoa((q.Page-1)*30-5)) // ddg html paginates ~30 per page
		v.Set("dc", strconv.Itoa((q.Page-1)*30-4))
	}
	req, err := newReq(ctx, "POST", "https://html.duckduckgo.com/html/", strings.NewReader(v.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", "https://html.duckduckgo.com/")
	d, err := doc(req)
	if err != nil {
		return nil, err
	}
	var out []Result
	d.Find(".result.results_links").Each(func(_ int, s *goquery.Selection) {
		if s.HasClass("result--ad") {
			return
		}
		a := s.Find("a.result__a").First()
		href, _ := a.Attr("href")
		if strings.HasPrefix(href, "//duckduckgo.com/l/") || strings.Contains(href, "duckduckgo.com/l/?") {
			if pu, err := url.Parse("https:" + strings.TrimPrefix(href, "https:")); err == nil {
				href = pu.Query().Get("uddg")
			}
		}
		if href == "" {
			return
		}
		out = append(out, Result{URL: href, Title: clean(a.Text()), Snippet: clean(s.Find(".result__snippet").Text())})
	})
	if len(out) == 0 && (d.Find(".anomaly-modal__title, #challenge-form").Length() > 0 || d.Find(".result").Length() == 0 && d.Find(".no-results").Length() == 0) {
		return nil, ErrBlocked{"duckduckgo"}
	}
	return out, nil
}

// ---- Mojeek (independent index) ----

type Mojeek struct{}

func (*Mojeek) Name() string       { return "mojeek" }
func (*Mojeek) Category() Category { return General }
func (*Mojeek) Weight() float64    { return 0.7 }

func (e *Mojeek) Search(ctx context.Context, q Query) ([]Result, error) {
	v := url.Values{"q": {q.Text}}
	if q.Page > 1 {
		v.Set("s", strconv.Itoa((q.Page-1)*10+1))
	}
	req, err := newReq(ctx, "GET", "https://www.mojeek.com/search?"+v.Encode(), nil)
	if err != nil {
		return nil, err
	}
	d, err := doc(req)
	if err != nil {
		return nil, err
	}
	if strings.Contains(d.Find("title").Text(), "Captcha") {
		return nil, ErrBlocked{"mojeek"}
	}
	var out []Result
	d.Find("ul.results-standard > li").Each(func(_ int, s *goquery.Selection) {
		a := s.Find("a.title").First()
		href, _ := a.Attr("href")
		if href == "" {
			return
		}
		out = append(out, Result{URL: href, Title: clean(a.Text()), Snippet: clean(s.Find("p.s").Text())})
	})
	return out, nil
}
