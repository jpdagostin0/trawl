package fetch

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"

	"github.com/temoto/robotstxt"
)

const robotsAgent = "trawl"
const maxRobots = 512 << 10

var ErrRobots = errors.New("robots.txt disallows fetching or could not be checked")

func (f *Fetcher) robotsClient() *http.Client {
	client := *f.client
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if err := f.client.CheckRedirect(req, via); err != nil {
			return err
		}
		if req.URL.User != nil || (req.URL.Scheme != "http" && req.URL.Scheme != "https") || !f.Allowed(req.Context(), req.URL) {
			return ErrRobots
		}
		return nil
	}
	return &client
}

// Allowed checks robots.txt before an automated fetch. An absent file permits
// fetching; temporary or ambiguous failures do not and are not cached.
func (f *Fetcher) Allowed(ctx context.Context, u *url.URL) bool {
	origin := u.Scheme + "://" + u.Host
	rules, ok := f.robots.Get(origin)
	if !ok {
		// Fetch the policy through the normal redirect-limited client, not the
		// robots-aware one (which would recursively request robots.txt).
		resp, err := get(ctx, f.client, origin+"/robots.txt", robotsAgent)
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		switch {
		case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone:
			rules = &robotstxt.RobotsData{}
		case resp.StatusCode >= 200 && resp.StatusCode < 300 && resp.StatusCode != http.StatusPartialContent:
			body, err := io.ReadAll(io.LimitReader(resp.Body, maxRobots))
			if err != nil {
				return false
			}
			rules, err = robotstxt.FromBytes(body)
			if err != nil {
				return false
			}
		default:
			return false
		}
		f.robots.Add(origin, rules)
	}
	return rules.TestAgent(u.RequestURI(), robotsAgent)
}
