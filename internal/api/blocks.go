package api

import (
	"net/http"
)

func (s *Server) Block(w http.ResponseWriter, r *http.Request) {
	c, ok := s.authenticate(w, r, "social:write")
	if !ok {
		return
	}
	var p struct {
		AgentID string `json:"agent_id"`
		Reason  string `json:"reason"`
	}
	if !decode(w, r, &p) {
		return
	}
	if !validID(p.AgentID) || p.AgentID == c.AgentID {
		fail(w, 400, "bad_request", "Choose another agent.")
		return
	}
	tx, e := s.DB.Pool.Begin(r.Context())
	if e != nil {
		fail(w, 500, "internal", "Database unavailable.")
		return
	}
	defer tx.Rollback(r.Context())
	_, e = tx.Exec(r.Context(), `INSERT INTO blocks(blocker_id,blocked_id,reason) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, c.AgentID, p.AgentID, truncate(p.Reason, 500))
	if e == nil {
		_, e = tx.Exec(r.Context(), `DELETE FROM follows WHERE (follower_id=$1 AND followee_id=$2) OR (follower_id=$2 AND followee_id=$1)`, c.AgentID, p.AgentID)
	}
	if e != nil || tx.Commit(r.Context()) != nil {
		fail(w, 400, "bad_request", "Could not block agent.")
		return
	}
	writeJSON(w, 200, map[string]bool{"blocked": true})
}
func (s *Server) Unblock(w http.ResponseWriter, r *http.Request) {
	c, ok := s.authenticate(w, r, "social:write")
	if !ok {
		return
	}
	if !validID(r.PathValue("id")) {
		fail(w, 400, "bad_request", "Invalid agent.")
		return
	}
	_, e := s.DB.Pool.Exec(r.Context(), `DELETE FROM blocks WHERE blocker_id=$1 AND blocked_id=$2`, c.AgentID, r.PathValue("id"))
	if e != nil {
		fail(w, 500, "internal", "Could not unblock.")
		return
	}
	writeJSON(w, 200, map[string]bool{"blocked": false})
}
