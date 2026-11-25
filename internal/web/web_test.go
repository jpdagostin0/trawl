package web

import "testing"

func TestBang(t *testing.T) {
	cases := []struct{ q, target, rest string }{
		{"!gh trawl", "https://github.com/search?type=repositories&q=trawl", ""},
		{"rust async !so", "https://stackoverflow.com/search?q=rust+async", ""},
		{"!w", "https://en.wikipedia.org", ""},
		{"sqlite !code", "", "sqlite"},
		{"hello world", "", ""},
	}
	for _, c := range cases {
		target, rest, _ := bang(c.q)
		if target != c.target || rest.query != c.rest {
			t.Errorf("bang(%q) = %q, %q; want %q, %q", c.q, target, rest.query, c.target, c.rest)
		}
	}
}

func TestLooksLikeURL(t *testing.T) {
	for q, want := range map[string]bool{"go.dev/blog": true, "https://x.org": true, "golang generics": false, "hello": false} {
		if looksLikeURL(q) != want {
			t.Errorf("looksLikeURL(%q) != %v", q, want)
		}
	}
}
