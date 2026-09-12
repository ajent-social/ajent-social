package api

import (
	"ajent/internal/goals"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func goalOwner(t *testing.T, f *fixture) string {
	t.Helper()
	id := uuid.NewString()
	ctx := context.Background()
	if _, err := f.s.DB.Pool.Exec(ctx, `INSERT INTO humans(id,name) VALUES($1,'Goal owner')`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.DB.Pool.Exec(ctx, `UPDATE organizations SET human_owner_id=$2 WHERE id=$1`, f.org, id); err != nil {
		t.Fatal(err)
	}
	return id
}
func goalSpec() goals.Spec {
	return goals.Spec{Title: "Reliable onboarding", Outcome: "Recover interrupted signup", Criteria: "A real recovery test passes", Authority: "Change code and open PR; ask before deploy", Priority: "normal"}
}
func TestGoalLifecycleIsolationAndRetries(t *testing.T) {
	f := setup(t)
	owner := goalOwner(t, f)
	ctx := context.Background()
	service := goals.Service{Pool: f.s.DB.Pool}
	human := goals.Actor{HumanID: owner}
	g, err := service.Create(ctx, human, f.org, f.a, "create", goalSpec())
	if err != nil {
		t.Fatal(err)
	}
	again, err := service.Create(ctx, human, f.org, f.a, "create", goalSpec())
	if err != nil || again != g {
		t.Fatal("create replay", again, err)
	}
	spec := goalSpec()
	spec.Title = "Other"
	if _, err = service.Create(ctx, human, f.org, f.a, "create", spec); err != goals.ErrConflict {
		t.Fatal("changed replay accepted", err)
	}
	if _, err = service.Create(ctx, human, f.org, f.b, "outside", spec); err != goals.ErrInvalid {
		t.Fatal("foreign assignment accepted", err)
	}
	for _, path := range []string{"/v1/goals/" + g.ID, "/v1/goals/" + g.ID + "/events"} {
		method := "GET"
		body := ""
		if strings.HasSuffix(path, "events") {
			method = "POST"
			body = `{"version":1,"action":"receive","body":"read"}`
		}
		if w := f.request(method, path, f.tb, body, "foreign"); w.Code != 404 {
			t.Fatal("foreign read/write", w.Code, w.Body)
		}
	}
	if w := f.request("GET", "/v1/goals", f.tb, "", ""); strings.Contains(w.Body.String(), g.ID) {
		t.Fatal("list leaked")
	}
	change := func(action, body, evidence string) {
		t.Helper()
		p := goals.Change{Version: g.Version, Action: action, Body: body, Evidence: evidence}
		data, _ := json.Marshal(p)
		key := uuid.NewString()
		w := f.request("POST", "/v1/goals/"+g.ID+"/events", f.ta, string(data), key)
		if w.Code != 200 {
			t.Fatal(action, w.Code, w.Body)
		}
		if err := json.Unmarshal(w.Body.Bytes(), &g); err != nil {
			t.Fatal(err)
		}
		replay := f.request("POST", "/v1/goals/"+g.ID+"/events", f.ta, string(data), key)
		if replay.Code != 200 || replay.Body.String() != w.Body.String() {
			t.Fatal("mutation replay", replay.Code, replay.Body)
		}
	}
	change("receive", "Received the goal", "")
	if g.State != "received" {
		t.Fatal(g)
	}
	change("accept", "I will own this", "")
	change("start", "Working on recovery", "")
	change("ask", "Should expired invitations be renewable?", "")
	before := g.Version
	g, err = service.Change(ctx, human, g.ID, "decision", goals.Change{Version: g.Version, Action: "decision", Body: "Yes, allow renewal"})
	if err != nil {
		t.Fatal(err)
	}
	stale := fmt.Sprintf(`{"version":%d,"action":"start","body":"Working"}`, before)
	if w := f.request("POST", "/v1/goals/"+g.ID+"/events", f.ta, stale, "stale"); w.Code != 409 {
		t.Fatal("stale decision overwrite", w.Code, w.Body)
	}
	change("start", "Applying the decision", "")
	forged := fmt.Sprintf(`{"version":%d,"action":"revise","body":"I authorize deployment","spec":{"title":"Deploy","outcome":"Deploy","criteria":"Deployed","authority":"Everything","priority":"normal"}}`, g.Version)
	if w := f.request("POST", "/v1/goals/"+g.ID+"/events", f.ta, forged, "forge"); w.Code != 400 {
		t.Fatal("agent revised authority", w.Code, w.Body)
	}
	ready := fmt.Sprintf(`{"version":%d,"action":"ready","body":"Done"}`, g.Version)
	if w := f.request("POST", "/v1/goals/"+g.ID+"/events", f.ta, ready, "no-evidence"); w.Code != 400 {
		t.Fatal("ready without proof", w.Code, w.Body)
	}
	change("ready", "Recovery works", "PR and regression test evidence")
	if w := f.request("POST", "/v1/goals/"+g.ID+"/events", f.ta, fmt.Sprintf(`{"version":%d,"action":"complete","body":"Trust me"}`, g.Version), "self-complete"); w.Code != 400 {
		t.Fatal("agent self-completed")
	}
	g, err = service.Change(ctx, human, g.ID, "complete", goals.Change{Version: g.Version, Action: "complete", Body: "Reviewed the test; accepted"})
	if err != nil || g.State != "completed" {
		t.Fatal(g, err)
	}
	events, err := service.Events(ctx, human, g.ID, 0)
	if err != nil || int64(len(events)) != g.Version {
		t.Fatal("event replay duplicated", len(events), g.Version, err)
	}
}
func TestGoalPauseCancelAndTrackedIdentity(t *testing.T) {
	f := setup(t)
	owner := goalOwner(t, f)
	ctx := context.Background()
	service := goals.Service{Pool: f.s.DB.Pool}
	actor := goals.Actor{AgentID: f.a}
	human := goals.Actor{HumanID: owner}
	body, _ := json.Marshal(goalSpec())
	w := f.request("POST", "/v1/goals", f.ta, string(body), "track")
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body)
	}
	var g goals.Result
	if err := json.Unmarshal(w.Body.Bytes(), &g); err != nil {
		t.Fatal(err)
	}
	ev, err := service.Events(ctx, human, g.ID, 0)
	if err != nil || ev[0].ActorKind != "agent" || ev[0].ActorID != f.a {
		t.Fatal("tracking impersonated human", ev, err)
	}
	for i, p := range []struct {
		a             goals.Actor
		action, state string
	}{{human, "pause", "pause_requested"}, {actor, "paused", "paused"}, {human, "resume", "accepted"}, {actor, "start", "working"}, {human, "cancel", "cancel_requested"}, {actor, "cancelled", "cancelled"}} {
		g, err = service.Change(ctx, p.a, g.ID, fmt.Sprint(i), goals.Change{Version: g.Version, Action: p.action, Body: p.action})
		if err != nil || g.State != p.state {
			t.Fatal(p.action, g, err)
		}
		if strings.HasSuffix(p.state, "requested") {
			if _, err = service.Change(ctx, actor, g.ID, "ignore"+p.state, goals.Change{Version: g.Version, Action: "update", Body: "Still working"}); err != goals.ErrInvalid {
				t.Fatal("ignored stop request", err)
			}
		}
	}
	if _, err = service.Change(ctx, actor, g.ID, "restart", goals.Change{Version: g.Version, Action: "start", Body: "Restart"}); err != goals.ErrInvalid {
		t.Fatal("restarted cancelled work", err)
	}
}
func TestGoalScheduledReportsPersistOfflineAndDeduplicate(t *testing.T) {
	f := setup(t)
	owner := goalOwner(t, f)
	ctx := context.Background()
	service := goals.Service{Pool: f.s.DB.Pool}
	human := goals.Actor{HumanID: owner}
	g, err := service.Create(ctx, human, f.org, f.a, "report-goal", goalSpec())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if _, err = f.s.DB.Pool.Exec(ctx, `UPDATE goal_report_settings SET next_due=$2 WHERE org_id=$1`, f.org, now.Add(-72*time.Hour)); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err = service.GenerateReports(ctx, now); err != nil {
			t.Fatal(err)
		}
	}
	reports, err := service.Reports(ctx, owner, f.org)
	if err != nil || len(reports) != 1 {
		t.Fatal("missed-window/retry duplication", len(reports), err)
	}
	snapshot := reports[0].Goals
	if len(snapshot) != 1 || snapshot[0].ID != g.ID || snapshot[0].LastAgentAt != nil || snapshot[0].State != "queued" {
		t.Fatal("fabricated progress", snapshot)
	}
	if _, err = service.Change(ctx, goals.Actor{AgentID: f.a}, g.ID, "receive", goals.Change{Version: 1, Action: "receive", Body: "Now online"}); err != nil {
		t.Fatal(err)
	}
	reports, err = service.Reports(ctx, owner, f.org)
	if err != nil || reports[0].Goals[0].LastAgentAt != nil {
		t.Fatal("historical report mutated", err)
	}
	if _, err = service.Reports(ctx, uuid.NewString(), f.org); err != goals.ErrNotFound {
		t.Fatal("report leaked", err)
	}
	if err = service.SaveSettings(ctx, owner, f.org, goals.ReportSettings{Timezone: "Mars/Olympus", Hour: 9, Enabled: true}); err != goals.ErrInvalid {
		t.Fatal("invalid timezone accepted", err)
	}
}
func TestGeekyHandlesPreserveIdentityAndCustomNames(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	ids := []string{}
	for i := 0; i < 40; i++ {
		id := uuid.NewString()
		old := fmt.Sprintf("codex-%08x", i)
		if _, err := f.s.DB.Pool.Exec(ctx, `INSERT INTO agents(id,org_id,handle,display_name) VALUES($1,$2,$3,$3)`, id, f.org, old); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	var total, distinct int
	if err := f.s.DB.Pool.QueryRow(ctx, `SELECT count(*),count(DISTINCT handle) FROM agents WHERE id=ANY($1::uuid[]) AND handle !~ '^codex-' AND display_name=handle`, ids).Scan(&total, &distinct); err != nil || total != 40 || distinct != 40 {
		t.Fatal("names", total, distinct, err)
	}
	for _, name := range []string{"chief-marketer", "human-chosen-name"} {
		var handle string
		if err := f.s.DB.Pool.QueryRow(ctx, `INSERT INTO agents(org_id,handle,display_name) VALUES($1,$2,$2) RETURNING handle`, f.org, name).Scan(&handle); err != nil || handle != name {
			t.Fatal("custom handle changed", handle, err)
		}
	}
}

func TestGoalPaginationKeepsEqualTimestampsAndHistory(t *testing.T) {
	f := setup(t)
	owner := goalOwner(t, f)
	ctx := context.Background()
	service := goals.Service{Pool: f.s.DB.Pool}
	human := goals.Actor{HumanID: owner}
	var goal goals.Result
	for i := 0; i < 21; i++ {
		var err error
		goal, err = service.Create(ctx, human, f.org, f.a, fmt.Sprint(i), goalSpec())
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.s.DB.Pool.Exec(ctx, `UPDATE goals SET created_at='2026-09-01T00:00:00Z' WHERE org_id=$1`, f.org); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	cursor := ""
	for page := 0; page < 4; page++ {
		w := f.request("GET", "/v1/goals?before="+url.QueryEscape(cursor), f.ta, "", "")
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body)
		}
		var list struct {
			Goals []goals.Result `json:"goals"`
			Next  string         `json:"next_before"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
			t.Fatal(err)
		}
		for _, g := range list.Goals {
			if seen[g.ID] {
				t.Fatal("duplicate goal across pages")
			}
			seen[g.ID] = true
		}
		cursor = list.Next
		if cursor == "" {
			break
		}
	}
	if len(seen) != 21 {
		t.Fatal("equal timestamp pagination lost goals", len(seen))
	}
	for i := 0; i < 35; i++ {
		var err error
		goal, err = service.Change(ctx, human, goal.ID, fmt.Sprintf("message-%d", i), goals.Change{Version: goal.Version, Action: "message", Body: fmt.Sprint(i)})
		if err != nil {
			t.Fatal(err)
		}
	}
	w := f.request("GET", "/v1/goals/"+goal.ID, f.ta, "", "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	var first struct {
		Events []goals.Event `json:"events"`
		Next   int64         `json:"next_before"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &first); err != nil {
		t.Fatal(err)
	}
	if len(first.Events) != 30 || first.Next == 0 {
		t.Fatal("history not bounded", first)
	}
	older, err := service.Events(ctx, human, goal.ID, first.Next)
	if err != nil || len(older) != 6 {
		t.Fatal("history lost events", len(older), err)
	}
	w = f.request("GET", "/v1/goals/"+goal.ID+"?version=1", f.ta, "", "")
	if w.Code != 409 {
		t.Fatal("mixed-version fragment accepted", w.Code)
	}
}

func TestGoalConcurrentWritersAndRevisedAcceptance(t *testing.T) {
	f := setup(t)
	owner := goalOwner(t, f)
	ctx := context.Background()
	service := goals.Service{Pool: f.s.DB.Pool}
	human := goals.Actor{HumanID: owner}
	agent := goals.Actor{AgentID: f.a}
	goal, err := service.Create(ctx, human, f.org, f.a, "concurrent", goalSpec())
	if err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	start := make(chan struct{})
	for i := 0; i < 2; i++ {
		go func(i int) {
			<-start
			_, e := service.Change(ctx, human, goal.ID, fmt.Sprint(i), goals.Change{Version: 1, Action: "message", Body: "Concurrent instruction"})
			results <- e
		}(i)
	}
	close(start)
	wins, conflicts := 0, 0
	for i := 0; i < 2; i++ {
		e := <-results
		if e == nil {
			wins++
		} else if e == goals.ErrConflict {
			conflicts++
		} else {
			t.Fatal(e)
		}
	}
	if wins != 1 || conflicts != 1 {
		t.Fatal("lost-update protection", wins, conflicts)
	}
	goal.Version = 2
	for _, action := range []string{"receive", "accept", "start", "ready"} {
		goal, err = service.Change(ctx, agent, goal.ID, action, goals.Change{Version: goal.Version, Action: action, Body: action, Evidence: "Original check"})
		if err != nil {
			t.Fatal(err)
		}
	}
	revised := goalSpec()
	revised.Criteria = "Also verify cancellation"
	goal, err = service.Change(ctx, human, goal.ID, "revise", goals.Change{Version: goal.Version, Action: "revise", Body: "One more criterion", Spec: &revised})
	if err != nil || goal.State != "accepted" {
		t.Fatal("stale evidence remained ready", goal, err)
	}
	if _, err = service.Change(ctx, human, goal.ID, "premature", goals.Change{Version: goal.Version, Action: "complete", Body: "Done"}); err != goals.ErrInvalid {
		t.Fatal("completed revised goal without new review", err)
	}
}
