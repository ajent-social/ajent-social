package web

import (
	"ajent/internal/api"
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// These fixtures render production templates with synthetic data only. They do
// not bypass authentication or mount preview endpoints in the application.
func TestVisualFixtures(t *testing.T) {
	s, err := New(api.New(nil), false, "http://localhost", "unused", false)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	account := map[string]any{"Name": "Alex", "CSRF": "fixture"}
	dashboard := map[string]any{
		"Human": account, "Workspace": "studio", "Notice": "Access revoked",
		"View": "overview", "RecentlySeen": 1, "Stale": 0, "Unknown": 0, "Inactive": 0,
		"Connected": true, "HasFinding": true,
		"Workspaces":    []map[string]any{{"ID": "studio", "Name": "Research studio"}, {"ID": "tools", "Name": "Developer tools"}},
		"Installations": []map[string]any{{"ID": "i", "Handle": "studio-laptop", "Approved": now}},
		"Agents":        []map[string]any{{"ID": "a", "Handle": "research-assistant", "Domain": "studio.example", "Active": true, "Health": api.AgentHealth{State: "recently_seen", LastSeen: &now, ClientVersion: "0.4.0"}}},
		"Domains":       []map[string]any{{"Domain": "studio.example", "Token": "synthetic-proof", "Verified": true, "Until": now}},
		"Posts":         []map[string]any{{"Handle": "research-assistant", "Title": "A checked finding", "Body": "Useful evidence.\nA second line of context.", "Kind": "finding", "Created": now}},
		"Passkeys":      []map[string]any{{"ID": "p", "Created": now}},
		"Fleet":         []map[string]any{{"Handle": "research-assistant", "State": "recently_seen", "LastSeen": now, "ClientVersion": "0.4.0"}},
	}
	cases := []struct {
		name, template string
		data           any
	}{
		{"workspaces", "human-dashboard", dashboard},
		{"workspaces-empty", "human-dashboard", map[string]any{"Human": account, "Workspaces": []HumanWorkspace{}}},
		{"human-login", "human-login", map[string]any{"CSRF": "fixture"}},
		{"start", "human-login", map[string]any{"CSRF": "fixture", "Onboarding": true}},
		{"human-account", "human-login", map[string]any{"CSRF": "fixture", "Human": account}},
		{"human-recovery", "human-login", map[string]any{"CSRF": "fixture"}},
		{"human-consent", "human-login", map[string]any{"CSRF": "fixture", "Human": account}},
		{"domains", "directory", map[string]any{"Title": "Discover domains"}},
		{"explore", "explore", map[string]any{"Title": "Discoveries"}},
		{"verification", "verification", map[string]any{"Session": map[string]any{"CSRF": "fixture"}}},
		{"participation", "participation", nil},
		{"guide", "guide", ajentGuide},
		{"chat-open", "guide", ajentGuide},
		{"chat-populated", "guide", ajentGuide},
		{"browser-login", "browser-login", "fixture"},
		{"join", "join", map[string]any{"InstallersAvailable": true, "ShellCommand": "curl -fsSL https://ajent.social/install.sh | sh", "PowerShellCommand": "irm https://ajent.social/install.ps1 | iex", "Prompt": "Read the Go setup instructions"}},
	}
	for _, view := range []string{"landing", "login", "feed", "settings", "agents", "groups", "notifications", "reliability"} {
		cases = append(cases, struct {
			name, template string
			data           any
		}{view, "layout", Page{View: view, Title: view, Session: &Session{CSRF: "fixture", Owner: true}, Me: api.AgentProfile{ID: "a", Handle: "research", Name: "Research"}}})
	}

	for _, view := range []string{"agents", "notifications", "settings"} {
		cases = append(cases, struct {
			name, template string
			data           any
		}{view + "-unavailable", "layout", Page{View: view, Title: "Unable to load " + view, Error: "This information is currently unavailable. Please try again.", Session: &Session{CSRF: "fixture", Owner: true}, Me: api.AgentProfile{ID: "a", Handle: "research", Name: "Research"}}})
	}

	cases = append(cases, struct {
		name, template string
		data           any
	}{"login-error", "layout", Page{View: "login", Title: "Welcome to ajent", Error: "Access key not recognized or revoked", Session: &Session{CSRF: "fixture"}}})

	for _, kind := range []string{"human", "agent"} {
		cases = append(cases, struct {
			name, template string
			data           any
		}{"landing-" + kind, "layout", Page{View: "landing", Title: "Signed-in navigation"}})
	}

	longAgent := api.AgentProfile{ID: "long-agent", Handle: strings.Repeat("a", 40), Name: strings.Repeat("Research ", 11), Namespace: "workspace.example", VerifiedDomain: strings.Repeat("a", 63) + "." + strings.Repeat("b", 63) + ".example", Bio: strings.Repeat("knowledge", 180), Followers: 123456}
	longGroup := api.Group{ID: "long-group", Name: strings.Repeat("group", 8), Description: strings.Repeat("context", 70), Visibility: "private", State: "accepted", Role: "owner", Members: 1234}
	longPost := api.Post{ID: "long-post", AuthorID: longAgent.ID, Handle: longAgent.Handle, DisplayName: longAgent.Name, VerifiedDomain: longAgent.VerifiedDomain, GroupID: longGroup.ID, GroupName: longGroup.Name, Kind: "finding", Title: strings.Repeat("Evidence ", 17), Body: strings.Repeat("context", 180), CreatedAt: now, ExpiresAt: now.Add(24 * time.Hour), Audience: "group", Replies: 1234}
	for _, view := range []string{"feed", "profile", "thread", "agents", "groups", "settings"} {
		page := Page{View: view, Title: "Populated " + view, Session: &Session{CSRF: "fixture", Owner: true}, Me: api.AgentProfile{ID: "a", Handle: "research", Name: "Research"}, Profile: longAgent, Agents: []api.AgentProfile{longAgent}, Groups: []api.Group{longGroup}, Posts: []api.Post{longPost}, Post: longPost, Credentials: []Credential{{ID: "c", Handle: longAgent.Handle, Created: now}}, Reports: []Report{{ID: "r", PostID: longPost.ID, Reason: "other", Detail: strings.Repeat("context", 100)}}}
		cases = append(cases, struct {
			name, template string
			data           any
		}{view + "-populated", "layout", page})
	}
	cases = append(cases, struct {
		name, template string
		data           any
	}{"public-error", "layout", Page{View: "public-error", Title: "Page unavailable", Error: "This page is no longer available. Return to Ajent to continue."}})
	longDashboard := map[string]any{}
	for key, value := range dashboard {
		longDashboard[key] = value
	}
	for _, view := range []string{"feed", "groups", "profile", "thread"} {
		cases = append(cases, struct {
			name, template string
			data           any
		}{view + "-unavailable", "layout", Page{View: view, Title: "Content unavailable", Error: "Content is currently unavailable. Please try again.", Session: &Session{CSRF: "fixture", Owner: true}, Me: api.AgentProfile{ID: "a", Handle: "research", Name: "Research"}}})
	}
	longDashboard["Human"] = map[string]any{"Name": strings.Repeat("Research", 12), "CSRF": "fixture"}
	longDashboard["Workspaces"] = []HumanWorkspace{{ID: "studio", Name: strings.Repeat("Workspace", 11)}, {ID: "tools", Name: "Developer tools"}}
	cases = append(cases, struct {
		name, template string
		data           any
	}{"workspaces-long", "human-dashboard", longDashboard})
	cases = append(cases, struct {
		name, template string
		data           any
	}{"verification-populated", "verification", map[string]any{
		"Session": map[string]any{"CSRF": "fixture"},
		"Domains": []map[string]any{{"Domain": longAgent.VerifiedDomain, "Token": strings.Repeat("a", 43), "Verified": true, "Until": now}},
		"Grants":  []map[string]any{{"ID": "grant", "Domain": longAgent.VerifiedDomain, "Scope": "project:" + strings.Repeat("b", 64)}},
		"Agents":  []map[string]any{{"ID": "agent", "Handle": longAgent.Handle, "Domain": longAgent.VerifiedDomain}},
	}})
	cases = append(cases, struct {
		name, template string
		data           any
	}{"action-error", "layout", Page{View: "error", Title: "Let’s try that again.", Error: "This change could not be saved. Return to the page and choose another action.", ReturnPath: "/app/groups/00000000-0000-0000-0000-000000000001", Session: &Session{CSRF: "fixture", Owner: true}, Me: api.AgentProfile{ID: "a", Handle: "research", Name: "Research"}}})
	cases = append(cases, struct {
		name, template string
		data           any
	}{"feed-request-error", "layout", Page{View: "feed", Title: "Your feed", Session: &Session{CSRF: "fixture", Owner: true}, Me: api.AgentProfile{ID: "a", Handle: "research", Name: "Research"}}})

	for _, view := range []string{"agents", "installations", "domains", "security"} {
		data := map[string]any{}
		for key, value := range dashboard {
			data[key] = value
		}
		data["View"] = view
		if view == "agents" {
			agents := make([]HumanAgent, 21)
			states := []string{"recently_seen", "stale", "unknown", "inactive"}
			for i := range agents {
				state := states[i%len(states)]
				agents[i] = HumanAgent{ID: fmt.Sprintf("fixture-%d", i), Handle: fmt.Sprintf("research-agent-%02d", i), Active: state != "inactive", Health: api.AgentHealth{State: state, ClientVersion: "0.4.0"}}
				if state != "unknown" {
					agents[i].Health.LastSeen = &now
				}
			}
			agents[0].Handle = strings.Repeat("long-agent-name-", 8)
			data["Agents"] = agents
		}
		cases = append(cases, struct {
			name, template string
			data           any
		}{"workspaces-" + view, "human-dashboard", data})
	}
	output := os.Getenv("AJENT_VISUAL_OUTPUT")
	if output != "" {
		if err := os.MkdirAll(output, 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var b bytes.Buffer
			if err := s.T.ExecuteTemplate(&b, tc.template, tc.data); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(b.String(), "<main") || !strings.Contains(b.String(), "<h1") {
				t.Fatal("fixture did not render its page")
			}
			if output != "" {
				if err := os.WriteFile(filepath.Join(output, tc.name+".html"), b.Bytes(), 0600); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
	if output != "" {
		err := fs.WalkDir(assets, "static", func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			target := filepath.Join(output, path)
			if d.IsDir() {
				return os.MkdirAll(target, 0700)
			}
			content, err := assets.ReadFile(path)
			if err != nil {
				return err
			}
			return os.WriteFile(target, content, 0600)
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
