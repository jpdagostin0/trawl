// Package archive stores readable article snapshots in a size-bounded directory.
package archive

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const Capacity int64 = 4_000_000_000 // 4 GB in bytes
const maxHeader = 64 << 10
const suffix = ".page"

type Entry struct {
	ID        string    `json:"id"`
	URL       string    `json:"url"`
	Title     string    `json:"title"`
	FetchedAt time.Time `json:"fetched_at"`
	Size      int64     `json:"size_bytes"`
}

type Store struct {
	dir   string
	limit int64

	mu      sync.Mutex
	used    int64
	entries map[string]Entry
}

func DefaultPath() string {
	if dir := os.Getenv("XDG_DATA_HOME"); dir != "" {
		return filepath.Join(dir, "trawl", "archive")
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".local", "share", "trawl", "archive")
	}
	return ""
}

// Open reads only the metadata of each existing article. The archive directory
// belongs to one server process and should not be shared between instances.
func Open(dir string, limit int64) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	s := &Store{dir: dir, limit: limit, entries: map[string]Entry{}}
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, file := range files {
		if !file.Type().IsRegular() {
			continue
		}
		name := file.Name()
		if strings.HasPrefix(name, ".partial-") {
			if err := os.Remove(filepath.Join(dir, name)); err != nil {
				return nil, err
			}
			continue
		}
		id, ok := strings.CutSuffix(name, suffix)
		if !ok || len(id) != sha256.Size*2 {
			continue
		}
		info, err := file.Info()
		if err != nil {
			return nil, err
		}
		entry, _, err := s.read(id, false)
		if err != nil {
			return nil, fmt.Errorf("read archive entry %s: %w", id, err)
		}
		entry.ID, entry.Size = id, info.Size()
		s.entries[id] = entry
		s.used += entry.Size
	}
	return s, s.makeRoom(0)
}

func (s *Store) Limit() int64 { return s.limit }

// Save keeps one full readable snapshot per final URL. Check the quota before
// writing, including the bytes needed for the temporary file and metadata.
func (s *Store) Save(url, title, markdown string, fetchedAt time.Time) (string, error) {
	key := sha256.Sum256([]byte(url))
	id := hex.EncodeToString(key[:])
	entry := Entry{ID: id, URL: url, Title: title, FetchedAt: fetchedAt}
	header, err := json.Marshal(entry)
	if err != nil {
		return "", err
	}
	entry.Size = int64(len(header)) + 1 + int64(len(markdown))
	if len(header)+1 > maxHeader || entry.Size > s.limit {
		return "", errors.New("snapshot exceeds archive size limit")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if old, exists := s.entries[id]; exists && !fetchedAt.After(old.FetchedAt) {
		return id, nil
	}
	if err := s.makeRoom(entry.Size); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(s.dir, ".partial-")
	if err != nil {
		return "", err
	}
	defer os.Remove(f.Name())
	_, err = f.Write(append(header, '\n'))
	if err == nil {
		_, err = f.WriteString(markdown)
	}
	if err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return "", err
	}
	if err := os.Rename(f.Name(), filepath.Join(s.dir, id+suffix)); err != nil {
		return "", err
	}
	s.used += entry.Size - s.entries[id].Size
	s.entries[id] = entry
	return id, nil
}

func (s *Store) makeRoom(incoming int64) error {
	if s.used <= s.limit-incoming {
		return nil
	}
	entries := s.sorted()
	for i := len(entries) - 1; i >= 0 && s.used > s.limit-incoming; i-- {
		e := entries[i]
		if err := os.Remove(filepath.Join(s.dir, e.ID+suffix)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		delete(s.entries, e.ID)
		s.used -= e.Size
	}
	return nil
}

func (s *Store) sorted() []Entry {
	entries := make([]Entry, 0, len(s.entries))
	for _, entry := range s.entries {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].FetchedAt.Equal(entries[j].FetchedAt) {
			return entries[i].ID < entries[j].ID
		}
		return entries[i].FetchedAt.After(entries[j].FetchedAt)
	})
	return entries
}

func (s *Store) List(offset, count int) ([]Entry, int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries := s.sorted()
	offset = min(max(0, offset), len(entries))
	count = min(max(0, count), len(entries)-offset)
	return entries[offset : offset+count], s.used
}

func (s *Store) Get(id string) (Entry, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.entries[id]
	if !ok {
		return Entry{}, "", fs.ErrNotExist
	}
	_, text, err := s.read(entry.ID, true)
	return entry, text, err
}

func (s *Store) read(id string, withBody bool) (Entry, string, error) {
	f, err := os.Open(filepath.Join(s.dir, id+suffix))
	if err != nil {
		return Entry{}, "", err
	}
	defer f.Close()
	reader := bufio.NewReaderSize(f, maxHeader)
	header, err := reader.ReadSlice('\n')
	if err != nil {
		return Entry{}, "", err
	}
	var entry Entry
	if err := json.Unmarshal(header, &entry); err != nil {
		return Entry{}, "", err
	}
	if !withBody {
		return entry, "", nil
	}
	body, err := io.ReadAll(reader)
	return entry, string(body), err
}
