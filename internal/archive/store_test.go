package archive

import (
	"errors"
	"io/fs"
	"strings"
	"testing"
	"time"
)

func TestStoreEvictsOldestAndKeepsFullContentsAcrossReopen(t *testing.T) {
	dir := t.TempDir()
	const limit = 700
	s, err := Open(dir, limit)
	if err != nil {
		t.Fatal(err)
	}
	body := strings.Repeat("Full content. ", 12)
	var ids []string
	for i, url := range []string{"https://x.test/a", "https://x.test/b", "https://x.test/c"} {
		id, err := s.Save(url, "Example", body, time.Unix(int64(i+1), 0).UTC())
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	s, err = Open(dir, limit)
	if err != nil {
		t.Fatal(err)
	}
	entries, used := s.List(0, 10)
	if len(entries) != 2 || entries[0].ID != ids[2] || entries[1].ID != ids[1] || used > limit {
		t.Fatalf("wrong entries or quota: %v (%d bytes)", entries, used)
	}
	if _, _, err := s.Get(ids[0]); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("oldest article still exists: %v", err)
	}
	if _, got, err := s.Get(ids[2]); err != nil || got != body {
		t.Fatalf("archived content was truncated or lost: %q (%v)", got, err)
	}
	if _, err := s.Save("https://x.test/large", "Large", strings.Repeat("a", 800), time.Now()); err == nil {
		t.Fatal("stored article larger than quota")
	}
	if entries, _ := s.List(0, 10); len(entries) != 2 {
		t.Fatal("oversized article evicted valid entries")
	}
}
