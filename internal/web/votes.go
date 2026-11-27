package web

import (
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"time"

	"github.com/jpdagostin0/trawl/internal/feedback"
)

// apiVote records an up/down/clear vote on a result. JSON only: a cross-site
// page can't send application/json without a CORS preflight, which we never
// answer, so other sites can't plant votes.
func (s *Server) apiVote(w http.ResponseWriter, r *http.Request) {
	if s.Votes == nil {
		apiErr(w, http.StatusNotImplemented, "voting is disabled")
		return
	}
	if mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mt != "application/json" {
		apiErr(w, http.StatusUnsupportedMediaType, "send application/json")
		return
	}
	var v feedback.Vote
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&v); err != nil {
		apiErr(w, 400, err.Error())
		return
	}
	v.Time = time.Time{} // server clock, not the client's
	if err := s.Votes.Vote(v); err != nil {
		apiErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// apiVotes lists the current votes, newest first.
func (s *Server) apiVotes(w http.ResponseWriter, r *http.Request) {
	if s.Votes == nil {
		writeJSON(w, 200, map[string]any{"votes": []feedback.Vote{}, "enabled": false})
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{"votes": s.Votes.Store.Votes(), "enabled": true})
}
