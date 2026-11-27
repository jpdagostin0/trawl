package web

import (
	"errors"
	"io/fs"
	"net/http"
	"strconv"
)

const archivePageSize = 40

func (s *Server) apiArchive(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if s.Archive == nil {
		apiErr(w, http.StatusServiceUnavailable, "the archive is disabled or unavailable")
		return
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	entries, used := s.Archive.List(offset, archivePageSize+1)
	more := len(entries) > archivePageSize
	if more {
		entries = entries[:archivePageSize]
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"entries": entries, "used_bytes": used, "limit_bytes": s.Archive.Limit(), "has_more": more,
	})
}

func (s *Server) apiArchiveEntry(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if s.Archive == nil {
		apiErr(w, http.StatusServiceUnavailable, "the archive is disabled or unavailable")
		return
	}
	entry, text, err := s.Archive.Get(r.PathValue("id"))
	if errors.Is(err, fs.ErrNotExist) {
		apiErr(w, http.StatusNotFound, "the archived page was not found or has been purged")
		return
	}
	if err != nil {
		apiErr(w, http.StatusInternalServerError, "could not read the archived page")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"entry": entry, "markdown": text})
}
