package web

import (
	"embed"
	"net/http"
	"strings"
)

//go:embed onboarding/install.sh onboarding/install.ps1 onboarding/agent-setup.md all:onboarding/plugins
var onboarding embed.FS

func (s *Server) Join(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	shellCommand := "curl -fsSL " + s.Origin + "/install.sh | sh"
	powerShellCommand := "irm " + s.Origin + "/install.ps1 | iex"
	prompt := "Read " + s.Origin + "/skill.md and follow its Ajent setup instructions. Use the Go client and `ajent setup`; reuse any existing installation. Show me the approval link so I can approve the connection, then confirm which tools are connected or explain how to reload them. Setup only: do not publish or upload project files. Respect explicit security or permission denials; report them instead of bypassing them."
	if err := s.T.ExecuteTemplate(w, "join", struct {
		ShellCommand, PowerShellCommand, Prompt string
		Human                                   *Human
	}{shellCommand, powerShellCommand, prompt, s.human(r)}); err != nil {
		http.Error(w, "Setup instructions unavailable. Reload to try again.", http.StatusServiceUnavailable)
	}
}
func (s *Server) OnboardingAsset(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/")
	if name == "integration.json" || name == "join.py" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		http.Error(w, "Gone: Python client assets have been retired.", http.StatusGone)
		return
	}
	if name == "skill.md" {
		name = "agent-setup.md"
	}
	if name != "install.sh" && name != "install.ps1" && name != "agent-setup.md" {
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
