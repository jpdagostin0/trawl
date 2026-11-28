package engine

import (
	"context"
	"errors"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/sync/errgroup"
)

// WikiArticle is the expanded Wikipedia panel beside the results: the summary's
// short description and a large picture, the lead section's paragraphs, the
// infobox's facts and the article's top-level sections.
type WikiArticle struct {
	Title       string        `json:"title"`
	Description string        `json:"description,omitempty"`
	Paragraphs  []string      `json:"paragraphs"`
	URL         string        `json:"url"`
	Image       string        `json:"image,omitempty"`
	ImageWidth  int           `json:"image_width,omitempty"`
	ImageHeight int           `json:"image_height,omitempty"`
	Facts       []WikiFact    `json:"facts,omitempty"`
	Sections    []WikiSection `json:"sections,omitempty"`
}

type WikiFact struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type WikiSection struct {
	Title  string `json:"title"`
	Anchor string `json:"anchor"`
}

const (
	// Wikimedia only renders thumbnails at its standard widths.
	wikiImageWidth = 500
	wikiLogoWidth  = 330
	maxParagraphs  = 5
	maxLeadChars   = 3000
	maxFacts       = 14
	maxFactChars   = 240
	maxSections    = 16
)

var (
	wikiHostRe = regexp.MustCompile(`^([a-z][a-z-]{1,11})\.(?:m\.)?wikipedia\.org$`)
	thumbPxRe  = regexp.MustCompile(`/\d+px-`)
	// The lead's pronunciation/citation clutter: "[1]", "[citation needed]", "(listen)".
	refRe = regexp.MustCompile(`\[(?:\d+|[a-z]|note \d+|citation needed|nb \d+)\]`)
)

// ErrNotWikipedia means the URL isn't a Wikipedia article.
var ErrNotWikipedia = errors.New("not a Wikipedia article URL")

// WikiTitle returns the language and title of a Wikipedia article URL.
func WikiTitle(raw string) (lang, title string, err error) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return "", "", ErrNotWikipedia
	}
	m := wikiHostRe.FindStringSubmatch(strings.ToLower(u.Hostname()))
	t, ok := strings.CutPrefix(u.Path, "/wiki/")
	if m == nil || !ok || t == "" || otherNamespace(t) {
		return "", "", ErrNotWikipedia
	}
	return m[1], t, nil
}

var namespaces = map[string]bool{"talk": true, "user": true, "wikipedia": true, "wp": true, "file": true, "image": true,
	"mediawiki": true, "template": true, "help": true, "category": true, "portal": true, "draft": true, "module": true,
	"timedtext": true, "special": true, "media": true, "book": true, "education_program": true, "gadget": true}

// otherNamespace reports whether a title is a talk page, file, category and so
// on rather than an article ("Star Wars: Episode IV" is an article).
func otherNamespace(title string) bool {
	ns, _, ok := strings.Cut(title, ":")
	if !ok {
		return false
	}
	ns = strings.ToLower(strings.ReplaceAll(ns, " ", "_"))
	return namespaces[strings.TrimSuffix(ns, "_talk")]
}

// FetchWikiArticle loads an article's panel from the Wikipedia APIs. The summary
// is required; the lead section and the section list fill in what they can.
func FetchWikiArticle(ctx context.Context, raw string) (*WikiArticle, error) {
	lang, title, err := WikiTitle(raw)
	if err != nil {
		return nil, err
	}
	base := "https://" + lang + ".wikipedia.org"
	var (
		sum      wikiSummary
		lead     *goquery.Document
		sections []WikiSection
	)
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		return getJSON(gctx, base+"/api/rest_v1/page/summary/"+url.PathEscape(title), &sum)
	})
	g.Go(func() error {
		lead, _ = wikiLead(gctx, base, title)
		return nil
	})
	g.Go(func() error {
		sections, _ = wikiSections(gctx, base, title)
		return nil
	})
	if err := g.Wait(); err != nil {
		return nil, err
	}
	if sum.Type == "disambiguation" || sum.Title == "" {
		return nil, ErrNotWikipedia
	}
	a := &WikiArticle{Title: sum.Title, Description: sum.Description, URL: sum.ContentURLs.Desktop.Page, Sections: sections}
	if a.URL == "" {
		a.URL = base + "/wiki/" + title
	}
	a.Image, a.ImageWidth, a.ImageHeight = sum.image()
	if lead != nil {
		a.Paragraphs = leadParagraphs(lead)
		a.Facts = infoboxFacts(lead)
	}
	if len(a.Paragraphs) == 0 && sum.Extract != "" {
		a.Paragraphs = []string{sum.Extract}
	}
	return a, nil
}

type wikiImage struct {
	Source        string `json:"source"`
	Width, Height int
}

type wikiSummary struct {
	Type, Title, Description, Extract string
	Thumbnail                         wikiImage `json:"thumbnail"`
	Original                          wikiImage `json:"originalimage"`
	ContentURLs                       struct {
		Desktop struct{ Page string } `json:"desktop"`
	} `json:"content_urls"`
}

// image picks a picture about wikiImageWidth wide (logos and other SVGs at
// wikiLogoWidth): the original when it's no bigger, otherwise the thumbnail at
// that width.
func (s *wikiSummary) image() (string, int, int) {
	o, t := s.Original, s.Thumbnail
	// An SVG's "original" is a small PNG rendering; ask for a bigger one.
	svg := strings.Contains(strings.ToLower(o.Source+t.Source), ".svg")
	want := wikiImageWidth
	if svg {
		want = wikiLogoWidth
	}
	switch {
	case !svg && o.Source != "" && o.Width > 0 && o.Width <= want:
		return o.Source, o.Width, o.Height
	case t.Source != "" && t.Width > 0 && thumbPxRe.MatchString(t.Source):
		return thumbPxRe.ReplaceAllString(t.Source, "/"+strconv.Itoa(want)+"px-"), want, t.Height * want / t.Width
	case t.Source != "":
		return t.Source, t.Width, t.Height
	}
	return o.Source, o.Width, o.Height
}

func wikiLead(ctx context.Context, base, title string) (*goquery.Document, error) {
	v := url.Values{"action": {"parse"}, "page": {title}, "prop": {"text"}, "section": {"0"},
		"redirects": {"1"}, "disableeditsection": {"1"}, "format": {"json"}, "formatversion": {"2"}}
	var r struct {
		Parse struct{ Text string } `json:"parse"`
	}
	if err := getJSON(ctx, base+"/w/api.php?"+v.Encode(), &r); err != nil {
		return nil, err
	}
	return goquery.NewDocumentFromReader(strings.NewReader(r.Parse.Text))
}

func wikiSections(ctx context.Context, base, title string) ([]WikiSection, error) {
	v := url.Values{"action": {"parse"}, "page": {title}, "prop": {"sections"}, "redirects": {"1"},
		"format": {"json"}, "formatversion": {"2"}}
	var r struct {
		Parse struct {
			Sections []struct {
				Line, Anchor string
				TocLevel     int `json:"toclevel"`
			} `json:"sections"`
		} `json:"parse"`
	}
	if err := getJSON(ctx, base+"/w/api.php?"+v.Encode(), &r); err != nil {
		return nil, err
	}
	var out []WikiSection
	for _, s := range r.Parse.Sections {
		if s.TocLevel != 1 {
			continue
		}
		switch s.Anchor {
		case "References", "Notes", "Citations", "Sources", "External_links", "Further_reading", "Bibliography", "Footnotes", "Notes_and_references":
			continue
		}
		out = append(out, WikiSection{Title: stripTags(s.Line), Anchor: s.Anchor})
		if len(out) == maxSections {
			break
		}
	}
	return out, nil
}

// text is a node's readable text without citations, hidden bits or styles.
func text(s *goquery.Selection) string {
	s = s.Clone()
	s.Find(`sup.reference, .reference, .noprint, .mw-editsection, style, .metadata, .ext-phonos, .mw-ext-cite-error, .error, [style*="display:none"]`).Remove()
	s.Find("br").ReplaceWithHtml(" · ")
	s.Find("li").Each(func(i int, li *goquery.Selection) {
		if i > 0 {
			li.PrependHtml(" · ")
		}
	})
	t := clean(refRe.ReplaceAllString(s.Text(), ""))
	t = strings.ReplaceAll(t, " ,", ",")
	t = strings.ReplaceAll(t, "( ", "(")
	t = strings.ReplaceAll(t, " )", ")")
	return strings.Trim(t, " ·")
}

func leadParagraphs(d *goquery.Document) []string {
	var out []string
	total := 0
	d.Find(".mw-parser-output > p").EachWithBreak(func(_ int, p *goquery.Selection) bool {
		if p.HasClass("mw-empty-elt") {
			return true
		}
		t := text(p)
		if len(t) < 40 {
			return true
		}
		out = append(out, t)
		total += len(t)
		return len(out) < maxParagraphs && total < maxLeadChars
	})
	return out
}

func infoboxFacts(d *goquery.Document) []WikiFact {
	var out []WikiFact
	seen := map[string]bool{}
	d.Find("table.infobox").First().Find("tr").EachWithBreak(func(_ int, tr *goquery.Selection) bool {
		label := text(tr.ChildrenFiltered("th.infobox-label, th"))
		value := text(tr.ChildrenFiltered("td.infobox-data, td"))
		if label == "" || value == "" || seen[label] || tr.Find("td.infobox-image, td.infobox-full-data").Length() > 0 {
			return true
		}
		seen[label] = true
		if r := []rune(value); len(r) > maxFactChars {
			value = strings.TrimRight(string(r[:maxFactChars-1]), " ·,;") + "…"
		}
		out = append(out, WikiFact{Label: label, Value: value})
		return len(out) < maxFacts
	})
	return out
}
