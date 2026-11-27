// Package feedback records the user's up/down votes on results and uses them to
// re-rank later searches. Votes and their embeddings live in DuckDB; similar
// past queries and similar voted pages are found with vector search in SQL.
package feedback

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	_ "github.com/duckdb/duckdb-go/v2" // registers the "duckdb" driver
)

// Vote is one up/down click. The votes table keeps every event (with the
// result's title, snippet and position) so it can be mined later; the
// current_votes view is the latest vote per (query, page).
type Vote struct {
	Time     time.Time `json:"time"`
	Query    string    `json:"query"`
	Category string    `json:"category,omitempty"`
	URL      string    `json:"url"`
	Title    string    `json:"title,omitempty"`
	Snippet  string    `json:"snippet,omitempty"`
	Engines  []string  `json:"engines,omitempty"`
	Position int       `json:"position,omitempty"` // 1-based rank it was shown at
	Dir      int       `json:"dir"`                // +1 up, -1 down, 0 cleared
}

// vote is the in-memory form, with the matching keys precomputed.
type vote struct {
	Vote
	q    string // normQuery(Query)
	key  string // page key (search.Key)
	host string
}

// Embedding columns are FLOAT[] (any length) and tagged with the model, so
// switching -embed-model never compares vectors from different models. Only
// voted queries and voted pages are stored; other searches stay in memory.
const schema = `
CREATE TABLE IF NOT EXISTS votes (
	time     TIMESTAMPTZ NOT NULL,
	query    TEXT NOT NULL,
	q        TEXT NOT NULL,
	category TEXT,
	url      TEXT NOT NULL,
	key      TEXT NOT NULL,
	host     TEXT,
	title    TEXT,
	snippet  TEXT,
	engines  TEXT[],
	position INTEGER,
	dir      TINYINT NOT NULL
);
CREATE OR REPLACE VIEW current_votes AS
	SELECT * EXCLUDE (rn) FROM (
		SELECT *, row_number() OVER (PARTITION BY q, key ORDER BY time DESC) AS rn FROM votes
	) WHERE rn = 1 AND dir <> 0;
CREATE TABLE IF NOT EXISTS query_vecs (
	model TEXT NOT NULL,
	q     TEXT NOT NULL,
	vec   FLOAT[] NOT NULL,
	PRIMARY KEY (model, q)
);
CREATE TABLE IF NOT EXISTS page_vecs (
	model TEXT NOT NULL,
	key   TEXT NOT NULL,
	vec   FLOAT[] NOT NULL,
	PRIMARY KEY (model, key)
);
`

type Store struct {
	db  *sql.DB
	key func(string) string

	mu    sync.RWMutex
	votes map[[2]string]*vote // (q, key) -> latest non-zero vote
}

// Open opens (or creates) the DuckDB database at path. key normalizes result
// URLs so votes match the same page however an engine spells its URL.
func Open(path string, key func(string) string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	db, err := sql.Open("duckdb", path)
	if err != nil {
		return nil, err
	}
	s := &Store{db: db, key: key, votes: map[[2]string]*vote{}}
	if err := s.load(); err != nil {
		db.Close()
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return s, nil
}

func (s *Store) load() error {
	if _, err := s.db.Exec(schema); err != nil {
		return err
	}
	rows, err := s.db.Query(`SELECT time, query, coalesce(category, ''), url, coalesce(title, ''), coalesce(snippet, ''),
		coalesce(array_to_string(engines, ','), ''), coalesce(position, 0), dir FROM current_votes`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var v Vote
		var engines string
		if err := rows.Scan(&v.Time, &v.Query, &v.Category, &v.URL, &v.Title, &v.Snippet, &engines, &v.Position, &v.Dir); err != nil {
			return err
		}
		if engines != "" {
			v.Engines = strings.Split(engines, ",")
		}
		s.apply(v)
	}
	return rows.Err()
}

func (s *Store) Close() error { return s.db.Close() }

// DefaultPath is $XDG_DATA_HOME/trawl/trawl.duckdb (~/.local/share/trawl/trawl.duckdb).
func DefaultPath() string {
	dir := os.Getenv("XDG_DATA_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		dir = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(dir, "trawl", "trawl.duckdb")
}

func (s *Store) apply(v Vote) {
	k := [2]string{normQuery(v.Query), s.key(v.URL)}
	if k[0] == "" || k[1] == "" {
		return
	}
	if v.Dir == 0 {
		delete(s.votes, k)
		return
	}
	s.votes[k] = &vote{Vote: v, q: k[0], key: k[1], host: hostOf(v.URL)}
}

// Record appends v to the votes table and updates the in-memory view.
func (s *Store) Record(v Vote) error {
	if v.Dir > 1 || v.Dir < -1 {
		return fmt.Errorf("dir must be -1, 0 or 1")
	}
	q, key := normQuery(v.Query), s.key(v.URL)
	if q == "" || key == "" {
		return fmt.Errorf("query and an absolute url are required")
	}
	if v.Time.IsZero() {
		v.Time = time.Now().UTC()
	}
	engines := v.Engines
	if engines == nil {
		engines = []string{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.db.Exec(`INSERT INTO votes VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		v.Time, v.Query, q, v.Category, v.URL, key, hostOf(v.URL), v.Title, v.Snippet, engines, v.Position, v.Dir); err != nil {
		return err
	}
	s.apply(v)
	return nil
}

// Votes returns the current (non-cleared) votes, newest first.
func (s *Store) Votes() []Vote {
	s.mu.RLock()
	out := make([]Vote, 0, len(s.votes))
	for _, v := range s.votes {
		out = append(out, v.Vote)
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Time.After(out[j].Time) })
	return out
}

func (s *Store) snapshot() []*vote {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*vote, 0, len(s.votes))
	for _, v := range s.votes {
		out = append(out, v)
	}
	return out
}

func normQuery(q string) string { return strings.Join(strings.Fields(strings.ToLower(q)), " ") }

func hostOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	h := strings.ToLower(u.Hostname())
	h = strings.TrimPrefix(h, "www.")
	return strings.TrimPrefix(h, "m.")
}
