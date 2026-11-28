package engine

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func TestWikiTitle(t *testing.T) {
	for raw, want := range map[string]string{
		"https://en.wikipedia.org/wiki/Alan_Turing":           "en:Alan_Turing",
		"https://de.m.wikipedia.org/wiki/C%2B%2B":             "de:C++",
		"https://en.wikipedia.org/wiki/Star_Wars:_Episode_IV": "en:Star_Wars:_Episode_IV",
		"https://en.wikipedia.org/wiki/User_talk:Example":     "",
		"https://en.wikipedia.org/wiki/Talk:Alan_Turing":      "",
		"https://en.wikipedia.org/w/index.php?title=Foo":      "",
		"https://wikipedia.org.evil.example/wiki/Alan_Turing": "",
		"https://example.com/wiki/Alan_Turing":                "",
	} {
		lang, title, err := WikiTitle(raw)
		got := ""
		if err == nil {
			got = lang + ":" + title
		}
		if got != want {
			t.Errorf("WikiTitle(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestWikiImageSizes(t *testing.T) {
	photo := wikiSummary{
		Thumbnail: wikiImage{"https://upload.wikimedia.org/a/b/X.jpg/330px-X.jpg", 330, 440},
		Original:  wikiImage{"https://upload.wikimedia.org/a/b/X.jpg", 2000, 2666},
	}
	if src, w, h := photo.image(); !strings.Contains(src, "/500px-X.jpg") || w != 500 || h != 666 {
		t.Errorf("photo: %s %dx%d", src, w, h)
	}
	logo := wikiSummary{
		Thumbnail: wikiImage{"https://upload.wikimedia.org/a/b/L.svg/330px-L.svg.png", 330, 330},
		Original:  wikiImage{"https://upload.wikimedia.org/a/b/L.svg/120px-L.svg.png", 110, 110},
	}
	if src, w, _ := logo.image(); !strings.Contains(src, "/330px-L.svg.png") || w != 330 {
		t.Errorf("logo: %s %d", src, w)
	}
	small := wikiSummary{Original: wikiImage{"https://upload.wikimedia.org/a/b/S.jpg", 200, 100}}
	if src, w, _ := small.image(); src != small.Original.Source || w != 200 {
		t.Errorf("small: %s %d", src, w)
	}
}

func TestWikiLeadAndFacts(t *testing.T) {
	d, err := goquery.NewDocumentFromReader(strings.NewReader(`<div class="mw-parser-output">
<table class="infobox"><tbody>
<tr><th colspan="2" class="infobox-above">Alan Turing</th></tr>
<tr><td colspan="2" class="infobox-image"><img src="x.jpg"></td></tr>
<tr><th class="infobox-label">Born</th><td class="infobox-data">Alan Mathison Turing<br><span style="display:none">(1912-06-23)</span>23 June 1912<br>London</td></tr>
<tr><th class="infobox-label">Known for</th><td class="infobox-data"><ul><li>Enigma</li><li>Turing machine</li></ul></td></tr>
</tbody></table>
<p class="mw-empty-elt"></p>
<p><b>Alan Turing</b> (23 June 1912 – 7 June 1954) was an English mathematician and computer scientist.<sup class="reference">[1]</sup></p>
<p><span class="error mw-ext-cite-error">Cite error: missing reflist.</span></p>
<p>He worked at Bletchley Park during the Second World War, breaking German ciphers [citation needed].</p>
</div>`))
	if err != nil {
		t.Fatal(err)
	}
	paras := leadParagraphs(d)
	if len(paras) != 2 || strings.Contains(paras[0], "[1]") || strings.Contains(paras[1], "citation needed") {
		t.Fatalf("paragraphs: %q", paras)
	}
	facts := infoboxFacts(d)
	want := []WikiFact{{"Born", "Alan Mathison Turing · 23 June 1912 · London"}, {"Known for", "Enigma · Turing machine"}}
	if len(facts) != len(want) || facts[0] != want[0] || facts[1] != want[1] {
		t.Fatalf("facts: %q", facts)
	}
}
