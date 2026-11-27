package fetch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/jpdagostin0/trawl/internal/engine"
)

const maxImage = 10 << 20

// Image hosts (upload.wikimedia.org in particular) answer bursts with 429, and a
// results page asks for dozens of thumbnails at once: cap parallel fetches per host.
const perHostImages = 6

var (
	imgSemMu sync.Mutex
	imgSem   = map[string]chan struct{}{}
)

// Wikimedia's User-Agent policy rate-limits generic browser UAs coming from
// servers (HTTP 429) but serves clients that identify themselves.
const wikimediaUA = "trawl/1.0 (+https://github.com/jpdagostin0/trawl; image proxy)"

func imageUA(host string) string {
	if strings.HasSuffix(host, ".wikimedia.org") || strings.HasSuffix(host, ".wikipedia.org") {
		return wikimediaUA
	}
	return engine.UserAgent
}

func hostSlot(host string) chan struct{} {
	imgSemMu.Lock()
	defer imgSemMu.Unlock()
	c, ok := imgSem[host]
	if !ok {
		if len(imgSem) > 4096 { // forget idle hosts now and then
			imgSem = map[string]chan struct{}{}
		}
		c = make(chan struct{}, perHostImages)
		imgSem[host] = c
	}
	return c
}

// Image fetches a remote image through the same SSRF-guarded client the reader
// uses, so the browser never has to contact image hosts itself. It returns the
// content type and bytes, and refuses anything that isn't an image.
func (f *Fetcher) Image(ctx context.Context, raw string) (string, []byte, error) {
	u, err := validate(raw)
	if err != nil {
		return "", nil, err
	}
	slot := hostSlot(u.Hostname())
	select {
	case slot <- struct{}{}:
		defer func() { <-slot }()
	case <-ctx.Done():
		return "", nil, ctx.Err()
	}
	var resp *http.Response
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
		if err != nil {
			return "", nil, err
		}
		req.Header.Set("User-Agent", imageUA(u.Hostname()))
		req.Header.Set("Accept", "image/avif,image/webp,image/png,image/svg+xml,image/*;q=0.8")
		resp, err = f.client.Do(req)
		if err != nil {
			return "", nil, err
		}
		if resp.StatusCode != http.StatusTooManyRequests || attempt == 1 {
			break
		}
		resp.Body.Close()
		select {
		case <-time.After(400 * time.Millisecond):
		case <-ctx.Done():
			return "", nil, ctx.Err()
		}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("%s returned HTTP %d", u.Host, resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxImage+1))
	if err != nil {
		return "", nil, err
	}
	if len(data) > maxImage {
		return "", nil, errors.New("image too large")
	}
	// Prefer the sniffed type; fall back to the upstream header for formats Go
	// can't sniff (SVG, AVIF). Anything else is refused.
	ctype := http.DetectContentType(data)
	if !strings.HasPrefix(ctype, "image/") {
		hdr, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
		if !strings.HasPrefix(hdr, "image/") {
			return "", nil, fmt.Errorf("not an image (%s)", ctype)
		}
		ctype = hdr
	}
	return ctype, data, nil
}
