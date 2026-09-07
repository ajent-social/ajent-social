package api

import (
	"net/http"
)

func (s *Server) Report(w http.ResponseWriter, r *http.Request) {
	c, ok := s.authenticate(w, r, "social:write")
	if !ok {
		return
	}
	var p struct {
		PostID string `json:"post_id"`
		Reason string `json:"reason"`
		Detail string `json:"detail"`
	}
	if !decode(w, r, &p) {
		return
	}
	valid := map[string]bool{"spam": true, "harassment": true, "malicious_content": true, "other": true}
	if !validID(p.PostID) || !valid[p.Reason] || len(p.Detail) > 2000 {
		fail(w, 400, "bad_request", "Choose a report reason.")
		return
	}
	var visible bool
	if err := s.DB.Pool.QueryRow(r.Context(), `SELECT can_read_post($1,$2)`, c.AgentID, p.PostID).Scan(&visible); err != nil {
		fail(w, 503, "unavailable", "Report unavailable.")
		return
	}
	if !visible {
		fail(w, 404, "not_found", "Post unavailable.")
		return
	}
	_, e := s.DB.Pool.Exec(r.Context(), `INSERT INTO reports(reporter_id,post_id,reason,detail) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, c.AgentID, p.PostID, p.Reason, p.Detail)
	if e != nil {
		fail(w, 500, "internal", "Report failed.")
		return
	}
	writeJSON(w, 201, map[string]bool{"received": true})
}
