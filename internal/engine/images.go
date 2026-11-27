package engine

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// ---- Bing Images ----

type BingImages struct{}

func (*BingImages) Name() string       { return "bing images" }
func (*BingImages) Category() Category { return Images }
func (*BingImages) Weight() float64    { return 1.0 }

func (e *BingImages) Search(ctx context.Context, q Query) ([]Result, error) {
	v := url.Values{"q": {q.Text}, "first": {strconv.Itoa((q.Page-1)*35 + 1)}, "count": {"35"}, "mmasync": {"1"}}
	req, err := newReq(ctx, "GET", "https://www.bing.com/images/async?"+v.Encode(), nil)
	if err != nil {
		return nil, err
	}
	d, err := doc(req)
	if err != nil {
		return nil, err
	}
	var out []Result
	d.Find("a.iusc").Each(func(_ int, s *goquery.Selection) {
		var m struct {
			Purl, Murl, Turl, T string
		}
		raw, _ := s.Attr("m")
		if json.Unmarshal([]byte(raw), &m) != nil || m.Murl == "" {
			return
		}
		r := Result{URL: m.Purl, Title: clean(m.T), Image: m.Murl, Thumbnail: m.Turl}
		// dimensions live in the sibling info panel: "1024 x 683 · jpeg"
		info := clean(s.Closest("li, .iuscp").Find(".img_info .nowrap").First().Text())
		if info != "" {
			r.Resolution = strings.Replace(strings.SplitN(info, "·", 2)[0], " x ", "×", 1)
			r.Resolution = strings.TrimSpace(r.Resolution)
		}
		out = append(out, r)
	})
	return out, nil
}

// ---- Wikimedia Commons ----

type WikiCommons struct{}

func (*WikiCommons) Name() string       { return "wikicommons.images" }
func (*WikiCommons) Category() Category { return Images }
func (*WikiCommons) Weight() float64    { return 0.7 }

func (e *WikiCommons) Search(ctx context.Context, q Query) ([]Result, error) {
	v := url.Values{"action": {"query"}, "format": {"json"}, "generator": {"search"}, "gsrsearch": {q.Text + " filetype:bitmap|drawing"},
		"gsrnamespace": {"6"}, "gsrlimit": {"20"}, "gsroffset": {strconv.Itoa((q.Page - 1) * 20)},
		"prop": {"imageinfo"}, "iiprop": {"url|size"}, "iiurlwidth": {"400"}}
	var r struct {
		Query struct {
			Pages map[string]struct {
				Title     string `json:"title"`
				Index     int    `json:"index"`
				ImageInfo []struct {
					URL            string `json:"url"`
					ThumbURL       string `json:"thumburl"`
					DescriptionURL string `json:"descriptionurl"`
					Width          int    `json:"width"`
					Height         int    `json:"height"`
				} `json:"imageinfo"`
			} `json:"pages"`
		} `json:"query"`
	}
	if err := getJSON(ctx, "https://commons.wikimedia.org/w/api.php?"+v.Encode(), &r); err != nil {
		return nil, err
	}
	out := make([]Result, len(r.Query.Pages))
	n := 0
	for _, p := range r.Query.Pages {
		if len(p.ImageInfo) == 0 || p.Index < 1 || p.Index > len(out) {
			continue
		}
		ii := p.ImageInfo[0]
		title := strings.TrimPrefix(p.Title, "File:")
		if i := strings.LastIndex(title, "."); i > 0 {
			title = title[:i]
		}
		out[p.Index-1] = Result{URL: ii.DescriptionURL, Title: title, Image: ii.URL, Thumbnail: ii.ThumbURL, Resolution: res(ii.Width, ii.Height)}
		n++
	}
	// keep API rank order, drop holes
	final := out[:0]
	for _, r := range out {
		if r.Image != "" {
			final = append(final, r)
		}
	}
	return final, nil
}
