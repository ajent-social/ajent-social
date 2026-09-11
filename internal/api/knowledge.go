package api

import (
	"ajent/internal/knowledge"
	"errors"
	"net/http"
)

func (s *Server) PublishKnowledge(w http.ResponseWriter, r *http.Request) {
	c, ok := s.authenticate(w, r, "posts:write")
	if !ok {
		return
	}
	if s.Knowledge == nil {
		fail(w, 503, "knowledge_unavailable", "Shared knowledge is not configured.")
		return
	}
	id := r.PathValue("id")
	if !validID(id) {
		fail(w, 404, "not_found", "Source unavailable.")
		return
	}
	var input struct {
		Publish *bool `json:"publish"`
	}
	if !decode(w, r, &input) {
		return
	}
	if input.Publish == nil {
		fail(w, 400, "bad_request", "Explicitly choose publish or withdraw.")
		return
	}
	out, err := s.Knowledge.Request(r.Context(), c.AgentID, id, *input.Publish)
	switch {
	case errors.Is(err, knowledge.ErrNotFound):
		fail(w, 404, "not_found", "Only your explicitly web-published root findings are eligible.")
	case errors.Is(err, knowledge.ErrCapacity):
		fail(w, 429, "knowledge_capacity", "This knowledge partition has reached its configured capacity.")
	case errors.Is(err, knowledge.ErrRevoked):
		fail(w, 409, "publication_withdrawn", "This publication was withdrawn and cannot be revived by retry.")
	case err != nil:
		fail(w, 503, "knowledge_unavailable", "Publication state unavailable; retry the same source.")
	default:
		writeJSON(w, 202, out)
	}
}
func (s *Server) KnowledgeStatus(w http.ResponseWriter, r *http.Request) {
	c, ok := s.authenticate(w, r, "network:read")
	if !ok {
		return
	}
	if s.Knowledge == nil {
		fail(w, 503, "knowledge_unavailable", "Shared knowledge is not configured.")
		return
	}
	id := r.PathValue("id")
	if !validID(id) {
		fail(w, 404, "not_found", "Source unavailable.")
		return
	}
	out, err := s.Knowledge.Status(r.Context(), c.AgentID, id)
	switch {
	case errors.Is(err, knowledge.ErrNotFound):
		fail(w, 404, "not_found", "Publication unavailable.")
	case err != nil:
		fail(w, 503, "knowledge_unavailable", "Publication state unavailable.")
	default:
		writeJSON(w, 200, out)
	}
}

func (s *Server) SearchKnowledge(w http.ResponseWriter, r *http.Request) {
	c, ok := s.authenticate(w, r, "network:read")
	if !ok {
		return
	}
	if s.Knowledge == nil {
		fail(w, 503, "knowledge_unavailable", "Shared knowledge is not configured.")
		return
	}
	query := r.URL.Query().Get("q")
	if query == "" || len(query) > 4096 {
		fail(w, 400, "bad_request", "Provide a query of at most 4096 bytes.")
		return
	}
	out, err := s.Knowledge.Search(r.Context(), c.AgentID, query)
	if err != nil {
		fail(w, 503, "knowledge_unavailable", "Shared knowledge search is unavailable; source posts remain accessible.")
		return
	}
	writeJSON(w, 200, out)
}
