package feedback

// Vector search over the embeddings of voted queries and voted pages. It's an
// exact scan with DuckDB's list_cosine_similarity, which is fine at the size of
// one person's votes and needs no extension.

type pageDoc struct{ key, title, snippet string }

type pageMatch struct {
	key string // the voted page
	cos float64
}

func (s *Store) putQueryVec(model, q string, vec []float32) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`INSERT INTO query_vecs VALUES (?, ?, ?) ON CONFLICT (model, q) DO UPDATE SET vec = excluded.vec`, model, q, vec)
	return err
}

func (s *Store) putPageVec(model, key string, vec []float32) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`INSERT INTO page_vecs VALUES (?, ?, ?) ON CONFLICT (model, key) DO UPDATE SET vec = excluded.vec`, model, key, vec)
	return err
}

// missingVecs lists voted queries and pages with no embedding for model yet
// (Ollama was down when they were voted, or the model changed).
func (s *Store) missingVecs(model string) (qs []string, pages []pageDoc, err error) {
	rows, err := s.db.Query(`SELECT DISTINCT q FROM current_votes WHERE q NOT IN (SELECT q FROM query_vecs WHERE model = ?)`, model)
	if err != nil {
		return nil, nil, err
	}
	for rows.Next() {
		var q string
		if err := rows.Scan(&q); err != nil {
			rows.Close()
			return nil, nil, err
		}
		qs = append(qs, q)
	}
	rows.Close()
	rows, err = s.db.Query(`SELECT key, coalesce(arg_max(title, time), ''), coalesce(arg_max(snippet, time), '') FROM current_votes
		WHERE key NOT IN (SELECT key FROM page_vecs WHERE model = ?) GROUP BY key`, model)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var p pageDoc
		if err := rows.Scan(&p.key, &p.title, &p.snippet); err != nil {
			return nil, nil, err
		}
		pages = append(pages, p)
	}
	return qs, pages, rows.Err()
}

// similarQueries returns voted queries whose embedding has cosine > min with qvec.
func (s *Store) similarQueries(model string, qvec []float32, min float64) (map[string]float64, error) {
	rows, err := s.db.Query(`SELECT q, list_cosine_similarity(vec, ?::FLOAT[]) AS cos FROM query_vecs
		WHERE model = ? AND q IN (SELECT q FROM current_votes) AND cos > ?`, qvec, model, min)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]float64{}
	for rows.Next() {
		var q string
		var c float64
		if err := rows.Scan(&q, &c); err != nil {
			return nil, err
		}
		out[q] = c
	}
	return out, rows.Err()
}

// similarPages finds, for each result (keys[i] with vecs[i]), the voted pages
// other than itself whose embedding has cosine > min with it.
func (s *Store) similarPages(model string, keys []string, vecs [][]float32, min float64) (map[string][]pageMatch, error) {
	rows, err := s.db.Query(`SELECT r.k, p.key, list_cosine_similarity(r.v, p.vec) AS cos
		FROM (SELECT unnest(?::TEXT[]) AS k, unnest(?::FLOAT[][]) AS v) r
		JOIN page_vecs p ON p.model = ? AND p.key <> r.k
		WHERE p.key IN (SELECT key FROM current_votes) AND cos > ?`, keys, vecs, model, min)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]pageMatch{}
	for rows.Next() {
		var k string
		var m pageMatch
		if err := rows.Scan(&k, &m.key, &m.cos); err != nil {
			return nil, err
		}
		out[k] = append(out[k], m)
	}
	return out, rows.Err()
}
