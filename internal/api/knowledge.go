package api

import (
	"ajent/internal/knowledge"
	"errors"
	"net/http"
)

// A private-only worker has a Service but no global partition or backend.
func (s *Server) globalKnowledgeConfigured() bool {
	return s.Knowledge != nil && s.Knowledge.BrainID != "" && s.Knowledge.Backend != nil
}

func (s *Server) PublishKnowledge(w http.ResponseWriter, r *http.Request) {
	c, ok := s.authenticate(w, r, "posts:write")
	if !ok {
		return
	}
	if !s.globalKnowledgeConfigured() {
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
	if !s.globalKnowledgeConfigured() {
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

// ReadKnowledge is the smallest end-to-end path to an explicitly published
// observation's exact live canonical fact: resolve the caller-supplied
// post ID to its durable global-brain mapping solely server-side, call
// Serenity's read_memory_fact, and require the SAME credential/scope and
// post authority still hold once that (possibly slow) call returns. A
// missing/older Serenity extension is an honest 503, never a recall/search
// or copied-post fallback; capability is never promised beyond "knowledge
// is configured" (the same global_knowledge_findings gate every other
// knowledge route already requires) -- read_memory_fact's own live
// tools/list check inside Serenity.ReadExact is the actual, honest per-call
// signal, not a value cached in a capability advertisement.
func (s *Server) ReadKnowledge(w http.ResponseWriter, r *http.Request) {
	c, ok := s.authenticate(w, r, "network:read")
	if !ok {
		return
	}
	if !s.globalKnowledgeConfigured() {
		fail(w, 503, "knowledge_unavailable", "Shared knowledge is not configured.")
		return
	}
	id := r.PathValue("id")
	if !validID(id) {
		fail(w, 404, "not_found", "Post unavailable.")
		return
	}
	out, err := s.Knowledge.ReadExact(r.Context(), c.AgentID, id)
	if err == nil && !s.recheckCredential(r.Context(), r, *c, "network:read") {
		fail(w, 401, "unauthorized", "Credential invalid, expired or revoked.")
		return
	}
	switch {
	case errors.Is(err, knowledge.ErrNotFound):
		fail(w, 404, "not_found", "Post unavailable.")
	case errors.Is(err, knowledge.ErrUnsupported):
		fail(w, 503, "knowledge_unsupported", "This server's configured knowledge backend does not support exact reads yet.")
	case err != nil:
		fail(w, 503, "knowledge_unavailable", "Exact read is unavailable; the source post remains accessible.")
	default:
		writeJSON(w, 200, out)
	}
}

func (s *Server) SearchKnowledge(w http.ResponseWriter, r *http.Request) {
	c, ok := s.authenticate(w, r, "network:read")
	if !ok {
		return
	}
	if !s.globalKnowledgeConfigured() {
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
