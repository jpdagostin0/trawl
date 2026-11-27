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

func TestCleanURLClearURLsRules(t *testing.T) {
	for in, want := range map[string]string{
		"https://www.google.com/url?q=https://example.com/a%3Fb%3D1&sa=U&ved=2ah": "https://example.com/a?b=1",
		"https://www.youtube.com/watch?v=abc&feature=share&si=xyz":                "https://www.youtube.com/watch?v=abc",
		"https://example.com/post?id=5&utm_source=x&fbclid=abc#intro":             "https://example.com/post?id=5#intro",
		"https://en.wikipedia.org/wiki/Go_(programming_language)":                 "https://en.wikipedia.org/wiki/Go_(programming_language)",
	} {
		if got := CleanURL(in); got != want {
			t.Errorf("CleanURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTidySnippet(t *testing.T) {
	if got := tidySnippet(`Hello!\r<br>\r<br>Lyrics  Hi`); got != "Hello! Lyrics Hi" {
		t.Errorf("got %q", got)
	}
}
