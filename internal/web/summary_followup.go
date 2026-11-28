package web

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"mime"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/ollama/ollama/api"
)

const maxFollowUpQuestion = 1500
const followUpHistory = 8 // four recent question/answer pairs, plus the original summary and sources

type summarySession struct {
	base    []api.Message
	mu      sync.Mutex
	history []api.Message
}

func (s *Summarizer) remember(messages []api.Message) string {
	id := rand.Text()
	s.sessions.Add(id, &summarySession{base: messages})
	return id
}

type followUpFrame struct {
	Answer string `json:"answer"`
	Done   bool   `json:"done"`
	Error  string `json:"error,omitempty"`
}

func (s *Server) apiSummaryFollowUp(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	model := s.Summary
	if model == nil || !model.Enabled.Load() {
		apiErr(w, http.StatusServiceUnavailable, "local summaries are disabled")
		return
	}
	if media, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); media != "application/json" {
		apiErr(w, http.StatusUnsupportedMediaType, "expected application/json")
		return
	}
	var input struct {
		Session  string `json:"session"`
		Question string `json:"question"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 12<<10)).Decode(&input); err != nil {
		apiErr(w, http.StatusBadRequest, "invalid follow-up request")
		return
	}
	question := strings.TrimSpace(input.Question)
	if question == "" || utf8.RuneCountInString(question) > maxFollowUpQuestion {
		apiErr(w, http.StatusBadRequest, "enter a question of up to 1500 characters")
		return
	}
	session, ok := model.sessions.Get(input.Session)
	if !ok {
		apiErr(w, http.StatusGone, "this summary session has expired; run the search again")
		return
	}
	if !session.mu.TryLock() {
		apiErr(w, http.StatusConflict, "a follow-up is already running")
		return
	}
	defer session.mu.Unlock()

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("X-Accel-Buffering", "no")
	enc, rc := json.NewEncoder(w), http.NewResponseController(w)
	rc.SetWriteDeadline(time.Time{}) // queued and uncapped: may outlast the server's WriteTimeout
	write := func(frame followUpFrame) error {
		if err := enc.Encode(frame); err != nil {
			return err
		}
		return rc.Flush()
	}
	msg := api.Message{Role: "user", Content: question}
	messages := slices.Concat(session.base, session.history, []api.Message{msg})
	answer, err := model.streamChat(r.Context(), messages, func(text string) error {
		if !model.Enabled.Load() {
			return context.Canceled
		}
		return write(followUpFrame{Answer: text})
	})
	if r.Context().Err() != nil {
		return
	}
	out := followUpFrame{Answer: answer, Done: true}
	if err != nil || !model.Enabled.Load() {
		out.Error = "the local model could not finish answering; try again"
	} else {
		session.history = append(session.history, msg, api.Message{Role: "assistant", Content: answer})
		if n := len(session.history); n > followUpHistory {
			session.history = slices.Clone(session.history[n-followUpHistory:])
		}
	}
	write(out)
}
