package web

import (
	"encoding/json"
	"net/http"
)

// SessionStatus exposes only sign-in state to the public front door. The session
// cookie remains host-only on the application; account details never cross origins.
func (s *Server) SessionStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Vary", "Origin, Cookie")
	w.Header().Set("Content-Type", "application/json")
	origin := r.Header.Get("Origin")
	if origin != "" && origin != s.Origin {
		if origin != "https://ajent.social" || s.Origin != "https://dogfood.ajent.social" {
			http.Error(w, "Origin not allowed", http.StatusForbidden)
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Credentials", "true")
	}
	json.NewEncoder(w).Encode(struct {
		Authenticated bool `json:"authenticated"`
	}{s.session(r) != nil})
}
