package web

import (
	"ajent/internal/auth"
	"ajent/internal/goals"
	"bytes"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/google/uuid"
)

type GoalsPage struct {
	ReportHistory                 []goals.Report
	Human                         *Human
	Workspaces                    []HumanWorkspace
	Workspace                     string
	Goals                         []goals.Goal
	Goal                          *goals.Goal
	Events                        []goals.Event
	Agents                        []HumanAgent
	Reports                       []goals.Report
	Settings                      goals.ReportSettings
	Tab, Error, Notice, Key, Next string
	EventBefore                   int64
	Form                          goals.Spec
	Draft, Action, AgentID        string
}

func (s *Server) goalRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /human/goals", s.humanGoals)
	m.HandleFunc("GET /human/goals/{id}", s.humanGoals)
	m.HandleFunc("POST /human/goals", s.humanGoalCreate)
	m.HandleFunc("POST /human/goals/{id}/events", s.humanGoalChange)
	m.HandleFunc("POST /human/goal-reports/settings", s.humanReportSettings)
}
func (s *Server) goalService() goals.Service { return goals.Service{Pool: s.API.DB.Pool} }
func (s *Server) renderGoals(w http.ResponseWriter, p GoalsPage, status int) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if p.Key == "" {
		p.Key = uuid.NewString()
	}
	var b bytes.Buffer
	if err := s.T.ExecuteTemplate(&b, "goals", p); err != nil {
		http.Error(w, "Goals unavailable. Reload to try again.", 503)
		return
	}
	w.WriteHeader(status)
	b.WriteTo(w)
}
func (s *Server) goalPage(r *http.Request, h *Human) (GoalsPage, error) {
	p := GoalsPage{Human: h, Workspace: r.URL.Query().Get("workspace"), Tab: r.URL.Query().Get("tab"), Notice: r.URL.Query().Get("notice"), Form: goals.Spec{Priority: "normal"}}
	rows, err := s.API.DB.Pool.Query(r.Context(), `SELECT id::text,name FROM organizations WHERE human_owner_id=$1 AND status='active' ORDER BY created_at LIMIT 100`, h.ID)
	if err != nil {
		return p, err
	}
	for rows.Next() {
		var x HumanWorkspace
		if err = rows.Scan(&x.ID, &x.Name); err != nil {
			rows.Close()
			return p, err
		}
		p.Workspaces = append(p.Workspaces, x)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return p, err
	}
	service := s.goalService()
	actor := goals.Actor{HumanID: h.ID}
	if id := r.PathValue("id"); id != "" {
		g, e := service.Get(r.Context(), actor, id)
		if e != nil {
			return p, e
		}
		p.Goal = &g
		p.Workspace = g.Workspace
		p.Form = g.Spec
		before := int64(0)
		if raw := r.URL.Query().Get("before"); raw != "" {
			before, e = strconv.ParseInt(raw, 10, 64)
			if e != nil || before < 1 {
				return p, goals.ErrInvalid
			}
		}
		p.Events, e = service.Events(r.Context(), actor, id, before)
		if e != nil {
			return p, e
		}
		if len(p.Events) == 30 {
			p.EventBefore = p.Events[len(p.Events)-1].Version
		}
	}
	if p.Workspace == "" && len(p.Workspaces) > 0 {
		p.Workspace = p.Workspaces[0].ID
	}
	if p.Workspace == "" {
		return p, nil
	}
	if err = service.Owns(r.Context(), h.ID, p.Workspace); err != nil {
		return p, err
	}
	if p.Goal == nil {
		p.Goals, err = service.List(r.Context(), actor, p.Workspace, r.URL.Query().Get("before"))
		if err != nil {
			return p, err
		}
		if len(p.Goals) == 100 {
			p.Next = p.Goals[len(p.Goals)-1].Cursor()
		}
	}
	rows, err = s.API.DB.Pool.Query(r.Context(), `SELECT a.id::text,a.handle FROM agents a WHERE a.org_id=$1 AND a.status='active' AND EXISTS(SELECT 1 FROM credentials c WHERE c.agent_id=a.id AND c.revoked_at IS NULL AND (c.expires_at IS NULL OR c.expires_at>now()) AND NOT EXISTS(SELECT 1 FROM fleet_members f JOIN credentials parent ON parent.id=f.parent_credential_id WHERE f.credential_id=c.id AND (parent.revoked_at IS NOT NULL OR parent.expires_at<=now()))) ORDER BY a.handle LIMIT 100`, p.Workspace)
	if err != nil {
		return p, err
	}
	for rows.Next() {
		var a HumanAgent
		if err = rows.Scan(&a.ID, &a.Handle); err != nil {
			rows.Close()
			return p, err
		}
		p.Agents = append(p.Agents, a)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return p, err
	}
	p.Settings, err = service.Settings(r.Context(), h.ID, p.Workspace)
	if err != nil {
		return p, err
	}
	if p.Tab == "updates" {
		p.ReportHistory, err = service.ReportHistory(r.Context(), h.ID, p.Workspace)
		if err != nil {
			return p, err
		}
		if id := r.URL.Query().Get("report"); id != "" {
			var report goals.Report
			report, err = service.Report(r.Context(), h.ID, p.Workspace, id)
			p.Reports = []goals.Report{report}
		} else {
			p.Reports, err = service.Reports(r.Context(), h.ID, p.Workspace)
		}
	}
	return p, err
}
func (s *Server) humanGoals(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	h := s.human(r)
	if h == nil {
		redirect(w, r, "/human/login")
		return
	}
	p, err := s.goalPage(r, h)
	if err != nil {
		s.goalFailure(w, r, p, err)
		return
	}
	s.renderGoals(w, p, 200)
}
func (s *Server) goalFailure(w http.ResponseWriter, r *http.Request, p GoalsPage, err error) {
	code := 503
	p.Error = "Goals are temporarily unavailable. Your operation is not confirmed. Retry with the same content."
	if errors.Is(err, goals.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if errors.Is(err, goals.ErrConflict) {
		code = 409
		p.Error = "This goal changed while you were writing. Your text is preserved below. Review the latest conversation before sending again."
		p.Key = uuid.NewString()
	}
	if errors.Is(err, goals.ErrInvalid) {
		code = 400
		p.Error = "Check the required fields, field lengths and action. Use an active agent in this workspace. The action may not be available in the current goal state."
	}
	s.renderGoals(w, p, code)
}
func goalSpec(r *http.Request) (goals.Spec, error) {
	p := goals.Spec{SourcePostID: r.FormValue("source_post_id"), Title: r.FormValue("title"), Outcome: r.FormValue("outcome"), Criteria: r.FormValue("criteria"), Authority: r.FormValue("authority"), Priority: r.FormValue("priority"), Budget: r.FormValue("budget")}
	if v := r.FormValue("deadline"); v != "" {
		d, err := time.Parse("2006-01-02", v)
		if err != nil {
			return p, goals.ErrInvalid
		}
		d = d.Add(24*time.Hour - time.Second)
		p.Deadline = &d
	}
	return p, nil
}
func (s *Server) humanGoalCreate(w http.ResponseWriter, r *http.Request) {
	h := s.goalAction(w, r)
	if h == nil {
		return
	}
	q := r.URL.Query()
	q.Set("workspace", r.FormValue("workspace"))
	q.Set("tab", "new")
	r.URL.RawQuery = q.Encode()
	p, err := s.goalPage(r, h)
	if err != nil {
		s.goalFailure(w, r, p, err)
		return
	}
	p.Key = r.FormValue("key")
	p.AgentID = r.FormValue("agent")
	p.Form, err = goalSpec(r)
	if err != nil {
		s.goalFailure(w, r, p, err)
		return
	}
	result, err := s.goalService().Create(r.Context(), goals.Actor{HumanID: h.ID}, p.Workspace, p.AgentID, p.Key, p.Form)
	if err != nil {
		s.goalFailure(w, r, p, err)
		return
	}
	redirect(w, r, "/human/goals/"+result.ID+"?notice=Goal+assigned.+Waiting+for+the+agent+to+check+in.")
}
func (s *Server) humanGoalChange(w http.ResponseWriter, r *http.Request) {
	h := s.goalAction(w, r)
	if h == nil {
		return
	}
	p, err := s.goalPage(r, h)
	if err != nil {
		s.goalFailure(w, r, p, err)
		return
	}
	p.Draft = r.FormValue("body")
	p.Action = r.FormValue("action")
	p.Key = r.FormValue("key")
	version, err := strconv.ParseInt(r.FormValue("version"), 10, 64)
	if err != nil {
		s.goalFailure(w, r, p, goals.ErrInvalid)
		return
	}
	change := goals.Change{Version: version, Action: p.Action, Body: p.Draft}
	if p.Action == "revise" {
		p.Form, err = goalSpec(r)
		if err != nil {
			s.goalFailure(w, r, p, err)
			return
		}
		change.Spec = &p.Form
	}
	_, err = s.goalService().Change(r.Context(), goals.Actor{HumanID: h.ID}, p.Goal.ID, p.Key, change)
	if err != nil {
		s.goalFailure(w, r, p, err)
		return
	}
	redirect(w, r, "/human/goals/"+p.Goal.ID+"?notice=Saved.+The+agent+will+receive+this+at+its+next+check-in.")
}
func (s *Server) humanReportSettings(w http.ResponseWriter, r *http.Request) {
	h := s.goalAction(w, r)
	if h == nil {
		return
	}
	org := r.FormValue("workspace")
	q := r.URL.Query()
	q.Set("workspace", org)
	q.Set("tab", "updates")
	r.URL.RawQuery = q.Encode()
	p, err := s.goalPage(r, h)
	if err != nil {
		s.goalFailure(w, r, p, err)
		return
	}
	hour, err := strconv.Atoi(r.FormValue("hour"))
	if err != nil {
		s.goalFailure(w, r, p, goals.ErrInvalid)
		return
	}
	p.Settings = goals.ReportSettings{Timezone: r.FormValue("timezone"), Hour: hour, Enabled: r.FormValue("enabled") == "yes"}
	if err = s.goalService().SaveSettings(r.Context(), h.ID, org, p.Settings); err != nil {
		s.goalFailure(w, r, p, err)
		return
	}
	redirect(w, r, "/human/goals?workspace="+url.QueryEscape(org)+"&tab=updates&notice=Report+schedule+saved.")
}

func (s *Server) goalAction(w http.ResponseWriter, r *http.Request) *Human {
	w.Header().Set("Cache-Control", "no-store")
	h := s.human(r)
	if h == nil {
		http.Error(w, "Sign in as yourself to continue.", 401)
		return nil
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if r.ParseForm() != nil || r.Header.Get("Origin") != s.Origin || !auth.ConstantEq([]byte(h.CSRF), []byte(r.PostForm.Get("csrf"))) {
		http.Error(w, "Reload this page to continue.", 403)
		return nil
	}
	return h
}
