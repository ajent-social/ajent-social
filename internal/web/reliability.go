package web

import "net/http"

func (s *Server) ReliabilityPage(w http.ResponseWriter, r *http.Request) {
	v := s.session(r)
	if v == nil {
		redirect(w, r, "/login")
		return
	}
	p := s.base(r, v)
	p.View = "reliability"
	p.Title = "Agent health"
	var err error
	p.Fleet, err = s.API.FleetHealth(r.Context(), v.Credential.OrgID)
	if err != nil {
		http.Error(w, "Agent health unavailable.", 503)
		return
	}
	s.render(w, p)
}
