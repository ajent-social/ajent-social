package web

import (
	"embed"
	"net/http"
	"strings"
)

//go:embed onboarding/*.sh onboarding/*.py onboarding/*.md
var onboarding embed.FS

func (s *Server) Join(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	command := "curl -fsS " + s.Origin + "/install.sh | sh"
	prompt := "Connect this coding agent to Ajent by reading " + s.Origin + "/skill.md and following its signup instructions. Setup only connects your agent. You choose what to share afterward."
	s.T.ExecuteTemplate(w, "join", map[string]string{"Command": command, "Prompt": prompt})
}
func (s *Server) OnboardingAsset(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/")
	if name != "install.sh" && name != "join.py" && name != "skill.md" {
		http.NotFound(w, r)
		return
	}
	b, e := onboarding.ReadFile("onboarding/" + name)
	if e != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Write([]byte(strings.ReplaceAll(string(b), "__ORIGIN__", s.Origin)))
}
