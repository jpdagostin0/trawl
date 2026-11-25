package search

import "testing"

func TestNormalizeDedupes(t *testing.T) {
	same := []string{
		"https://www.example.com/a/b/",
		"http://example.com/a/b",
		"https://example.com/a/b?utm_source=x&fbclid=y",
		"https://example.com/a/b#section",
	}
	want := normalize(same[0])
	for _, u := range same[1:] {
		if got := normalize(u); got != want {
			t.Errorf("normalize(%q) = %q, want %q", u, got, want)
		}
	}
	if normalize("https://example.com/a/b?id=1") == want {
		t.Error("meaningful query params must not be stripped")
	}
}

func TestCleanURL(t *testing.T) {
	if got := CleanURL("https://x.com/p?utm_medium=a&id=3"); got != "https://x.com/p?id=3" {
		t.Errorf("got %q", got)
	}
}
