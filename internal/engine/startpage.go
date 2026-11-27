package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// Startpage proxies Google's index (results are display_type "web-google").
// Like SearXNG's startpage engine it needs the per-session "sc" token from the
// home page form; we cache it for a while and refresh on failure.
type Startpage struct {
	mu      sync.Mutex
	sc      string
	scAt    time.Time
	jar     http.CookieJar
	jarOnce sync.Once
}

func (*Startpage) Name() string       { return "startpage" }
func (*Startpage) Category() Category { return General }
func (*Startpage) Weight() float64    { return 1.2 } // Google-quality results

func (e *Startpage) client() *http.Client {
	e.jarOnce.Do(func() { e.jar, _ = cookiejar.New(nil) })
	return &http.Client{Timeout: 8 * time.Second, Jar: e.jar}
}

func (e *Startpage) token(ctx context.Context, refresh bool) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !refresh && e.sc != "" && time.Since(e.scAt) < 30*time.Minute {
		return e.sc, nil
	}
	req, err := newReq(ctx, "GET", "https://www.startpage.com/", nil)
	if err != nil {
		return "", err
	}
	resp, err := e.client().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	d, err := goquery.NewDocumentFromReader(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return "", err
	}
	sc, ok := d.Find(`input[name="sc"]`).First().Attr("value")
	if !ok || sc == "" {
		return "", ErrBlocked{"startpage"}
	}
	e.sc, e.scAt = sc, time.Now()
	return sc, nil
}

type spResult struct {
	Title       string `json:"title"`
	ClickURL    string `json:"clickUrl"`
	Description string `json:"description"`
	// image results
	RawImageURL  string `json:"rawImageUrl"`
	ThumbnailURL string `json:"thumbnailUrl"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	AltClickURL  string `json:"altClickUrl"`
}

func (e *Startpage) Search(ctx context.Context, q Query) ([]Result, error) {
	return e.search(ctx, q, "web", "web-google")
}

func (e *Startpage) search(ctx context.Context, q Query, cat, displayType string) ([]Result, error) {
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		sc, err := e.token(ctx, attempt > 0)
		if err != nil {
			return nil, err
		}
		form := url.Values{"query": {q.Text}, "cat": {cat}, "t": {"device"}, "sc": {sc}, "abp": {"1"}, "abd": {"1"}, "abe": {"1"}}
		if q.Page > 1 {
			form.Set("page", strconv.Itoa(q.Page))
		}
		req, err := newReq(ctx, "POST", "https://www.startpage.com/sp/search", strings.NewReader(form.Encode()))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Origin", "https://www.startpage.com")
		req.Header.Set("Referer", "https://www.startpage.com/")
		resp, err := e.client().Do(req)
		if err != nil {
			return nil, err
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != 200 {
			lastErr = errors.New("startpage status " + strconv.Itoa(resp.StatusCode))
			continue
		}
		res, err := parseStartpage(body, displayType)
		if err == nil {
			return res, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = ErrBlocked{"startpage"}
	}
	return nil, lastErr
}

// parseStartpage pulls the {"display_type":"web-google",...,"results":[...]} object
// out of the inline hydration JSON.
func parseStartpage(body []byte, displayType string) ([]Result, error) {
	marker := []byte(`{"display_type":"` + displayType + `"`)
	i := bytes.Index(body, marker)
	if i < 0 {
		if bytes.Contains(body, []byte("display_type")) {
			return nil, nil // valid page, just no results
		}
		return nil, ErrBlocked{"startpage"}
	}
	var block struct {
		Results []spResult `json:"results"`
	}
	if err := json.NewDecoder(bytes.NewReader(body[i:])).Decode(&block); err != nil {
		return nil, err
	}
	out := make([]Result, 0, len(block.Results))
	for _, r := range block.Results {
		if r.RawImageURL != "" {
			page := r.AltClickURL
			if page == "" {
				page = r.ClickURL
			}
			out = append(out, Result{URL: page, Title: stripTags(r.Title), Image: r.RawImageURL, Thumbnail: r.ThumbnailURL,
				Resolution: res(r.Width, r.Height)})
			continue
		}
		if r.ClickURL == "" {
			continue
		}
		out = append(out, Result{URL: r.ClickURL, Title: stripTags(r.Title), Snippet: stripTags(r.Description)})
	}
	return out, nil
}

func res(w, h int) string {
	if w == 0 || h == 0 {
		return ""
	}
	return strconv.Itoa(w) + "×" + strconv.Itoa(h)
}
