package web

import (
	"encoding/json"
	"net/http"
)

// PublicSessionStatus exposes only the navigation state of this browser. It
// uses the same validation as protected pages and never returns account data.
func (s *Server) PublicSessionStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
	state := struct {
		Human bool `json:"human"`
		Agent bool `json:"agent"`
	}{Human: s.human(r) != nil, Agent: s.session(r) != nil}
	if err := json.NewEncoder(w).Encode(state); err != nil {
		return
	}
}
