package api

import (
	"ajent/internal/improvements"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
)

func (s *Server) GetImprovementControl(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.improvementOperator(w, r); !ok {
		return
	}
	repo := r.URL.Query().Get("repository")
	c, err := improvements.ReadControl(r.Context(), s.DB.Pool, repo)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "not_found", "Improvement control not found.")
		return
	}
	if err != nil {
		fail(w, 400, "bad_request", "Valid repository required.")
		return
	}
	writeJSON(w, 200, c)
}

func (s *Server) PutImprovementControl(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.improvementOperator(w, r); !ok {
		return
	}
	var c improvements.Control
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&c); err != nil {
		fail(w, 400, "bad_request", "JSON body required.")
		return
	}
	if err := improvements.EnsureControl(r.Context(), s.DB.Pool, c.Repository); err != nil {
		fail(w, 400, "bad_request", "Valid repository required.")
		return
	}
	out, err := improvements.UpdateControl(r.Context(), s.DB.Pool, c)
	if errors.Is(err, improvements.ErrConflict) {
		fail(w, 409, "conflict", "Improvement control changed.")
		return
	}
	if err != nil {
		fail(w, 400, "bad_request", "Invalid improvement control.")
		return
	}
	writeJSON(w, 200, out)
}

func (s *Server) GetImprovementWorkerHealth(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.improvementOperator(w, r); !ok {
		return
	}
	h, err := improvements.WorkerHealth(r.Context(), s.DB.Pool, r.URL.Query().Get("repository"))
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "not_found", "Improvement control not found.")
		return
	}
	if err != nil {
		fail(w, 400, "bad_request", "Valid repository required.")
		return
	}
	writeJSON(w, 200, h)
}

func (s *Server) ListImprovementDecisions(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.improvementOperator(w, r); !ok {
		return
	}
	page, err := improvements.ListDecisions(r.Context(), s.DB.Pool, r.URL.Query().Get("repository"), r.URL.Query().Get("cursor"))
	if errors.Is(err, improvements.ErrInvalidDecision) {
		fail(w, 400, "bad_request", "Valid repository and cursor required.")
		return
	}
	if err != nil {
		fail(w, 503, "unavailable", "Decision review unavailable.")
		return
	}
	writeJSON(w, 200, page)
}

func (s *Server) ReviewImprovementDecision(w http.ResponseWriter, r *http.Request) {
	c, ok := s.improvementOperator(w, r)
	if !ok {
		return
	}
	var review improvements.DecisionReview
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&review); err != nil {
		fail(w, 400, "bad_request", "JSON body required.")
		return
	}
	out, err := improvements.ReviewDecision(r.Context(), s.DB.Pool, r.URL.Query().Get("repository"), r.PathValue("id"), c.AgentID, review)
	if errors.Is(err, improvements.ErrDecisionConflict) {
		fail(w, 409, "conflict", "Decision changed.")
		return
	}
	if errors.Is(err, improvements.ErrInvalidDecision) {
		fail(w, 400, "bad_request", "Invalid decision review.")
		return
	}
	if errors.Is(err, improvements.ErrDecisionNotFound) {
		fail(w, 404, "not_found", "Decision not found.")
		return
	}
	if err != nil {
		fail(w, 503, "unavailable", "Decision review unavailable.")
		return
	}
	writeJSON(w, 200, out)
}

func (s *Server) GetImprovementTriageMetrics(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.improvementOperator(w, r); !ok {
		return
	}
	m, err := improvements.TriageMetrics(r.Context(), s.DB.Pool, r.URL.Query().Get("repository"))
	if errors.Is(err, improvements.ErrInvalidDecision) {
		fail(w, 400, "bad_request", "Valid repository required.")
		return
	}
	if err != nil {
		fail(w, 503, "unavailable", "Triage metrics unavailable.")
		return
	}
	writeJSON(w, 200, m)
}
