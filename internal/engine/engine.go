// Package engine contains the upstream search backends trawl fans out to.
package engine

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// Category groups engines into the tabs shown in the UI.
type Category string

const (
	General Category = "general"
	Images  Category = "images"
	Videos  Category = "videos"
	News    Category = "news"
	Code    Category = "code"
	Forums  Category = "forums"
	Science Category = "science"
)

// Categories is the display order of tabs; tabs without engines are hidden.
var Categories = []Category{General, Images, Videos, News, Code, Forums, Science}

type Query struct {
	Text string
	Page int    // 1-based
	Lang string // e.g. "en"
}

type Result struct {
	URL     string `json:"url"`
	Title   string `json:"title"`
	Snippet string `json:"snippet"`
	Meta    string `json:"meta,omitempty"` // e.g. "123 points · 45 comments"
	// Image/video results
	Thumbnail  string `json:"thumbnail,omitempty"`
	Image      string `json:"image,omitempty"` // full-size image URL
	Resolution string `json:"resolution,omitempty"`
	// News results
	Source    string   `json:"source,omitempty"`    // publisher name
	Published string   `json:"published,omitempty"` // RFC 3339
	Engines   []string `json:"engines"`
	Score     float64  `json:"score"`
}

// Infobox is a "quick answer" panel (Wikipedia summary).
type Infobox struct {
	Title   string `json:"title"`
	Extract string `json:"extract"`
	URL     string `json:"url"`
	Image   string `json:"image,omitempty"`
}

type Engine interface {
	Name() string
	Category() Category
	Weight() float64
	Search(ctx context.Context, q Query) ([]Result, error)
}

// InfoboxProvider is implemented by engines that can also return an infobox.
type InfoboxProvider interface {
	Infobox(ctx context.Context, q Query) (*Infobox, error)
}

const UserAgent = "Mozilla/5.0 (X11; Linux x86_64; rv:130.0) Gecko/20100101 Firefox/130.0"

var client = &http.Client{Timeout: 8 * time.Second}

func newReq(ctx context.Context, method, u string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/json;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.7")
	return req, nil
}

func do(req *http.Request) (*http.Response, error) {
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("upstream status %d", resp.StatusCode)
	}
	return resp, nil
}

func doc(req *http.Request) (*goquery.Document, error) {
	resp, err := do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return goquery.NewDocumentFromReader(io.LimitReader(resp.Body, 4<<20))
}

func clean(s string) string { return strings.Join(strings.Fields(s), " ") }

// ErrBlocked signals a captcha / bot wall; the aggregator suspends the engine longer.
type ErrBlocked struct{ Engine string }

func (e ErrBlocked) Error() string { return e.Engine + ": blocked by captcha/bot wall" }

// All returns every built-in engine.
func All() []Engine {
	return []Engine{
		&Startpage{}, &Bing{}, &DuckDuckGo{}, &Mojeek{}, &Wikipedia{},
		&BingImages{}, &WikiCommons{},
		&BingNews{}, &DuckDuckGoNews{},
		&GitHub{}, &StackOverflow{},
		&HackerNews{},
	}
}
