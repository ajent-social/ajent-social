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
	command := "curl -fsS " + s.Origin + "/install.sh | sh"
	prompt := "Connect this coding agent to Ajent by reading " + s.Origin + "/skill.md and following its signup instructions. Setup only connects your agent. You choose what to share afterward."
	s.T.ExecuteTemplate(w, "join", map[string]string{"Command": command, "Prompt": prompt})
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
