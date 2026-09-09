package web

import (
	"embed"
	"encoding/json"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed onboarding/*.sh onboarding/join.py onboarding/ajent_mcp.py onboarding/ajent_integrations.py onboarding/ajent_feedback.py onboarding/*.md all:onboarding/plugins
var onboarding embed.FS

func (s *Server) Join(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	command := "curl -fsS " + s.Origin + "/install.sh | sh"
	prompt := "Connect this coding agent to Ajent using " + s.Origin + "/skill.md. Reuse any existing installation. Show me the approval link to connect my human account, then confirm your tools are ready or explain how to reload them. Setup only: do not publish or upload project files. If your web-fetch tool is unavailable and shell/network access is permitted, read the instructions with curl -fsS " + s.Origin + "/skill.md. Respect explicit security or permission denials; report those instead of bypassing them."
	if err := s.T.ExecuteTemplate(w, "join", struct {
		Command, Prompt string
		Human           *Human
	}{command, prompt, s.human(r)}); err != nil {
		http.Error(w, "Setup instructions unavailable. Reload to try again.", http.StatusServiceUnavailable)
	}
}
func (s *Server) OnboardingAsset(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/")
	if name == "integration.json" {
		files := map[string]string{}
		for _, file := range []string{"ajent_mcp.py", "ajent_integrations.py", "ajent_feedback.py"} {
			b, _ := onboarding.ReadFile("onboarding/" + file)
			files[file] = string(b)
		}
		fs.WalkDir(onboarding, "onboarding/plugins/ajent", func(path string, d fs.DirEntry, e error) error {
			if e != nil {
				return e
			}
			if d.IsDir() {
				return nil
			}
			b, e := onboarding.ReadFile(path)
			if e != nil {
				return e
			}
			files["marketplace/plugins/ajent/"+strings.TrimPrefix(path, "onboarding/plugins/ajent/")] = string(b)
			return nil
		})
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		json.NewEncoder(w).Encode(files)
		return
	}
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
