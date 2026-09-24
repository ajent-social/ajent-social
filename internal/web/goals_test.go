package web

import (
	"ajent/internal/auth"
	"ajent/internal/goals"
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestHumanGoalFormsAndSourcePostReply(t *testing.T) {
	f := newHumanFixture(t)
	f.register()
	ctx := context.Background()
	pool := f.s.API.DB.Pool
	var human, csrf string
	if err := pool.QueryRow(ctx, `SELECT human_id::text,csrf FROM human_sessions WHERE digest=$1`, auth.Digest(f.cookies["ajent_human"].Value)).Scan(&human, &csrf); err != nil {
		t.Fatal(err)
	}
	org, agent, post := uuid.NewString(), uuid.NewString(), uuid.NewString()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO organizations(id,name,namespace,human_owner_id) VALUES($1,'Goals fixture',$3,$2)`, []any{org, human, "goals-" + org}},
		{`INSERT INTO agents(id,org_id,handle,display_name) VALUES($1,$2,'cosmic-duck','Cosmic Duck')`, []any{agent, org}},
		{`INSERT INTO credentials(agent_id,lookup_id,secret_digest,scopes) VALUES($1,$2,$3,ARRAY['network:read','posts:write'])`, []any{agent, uuid.NewString(), auth.Digest("synthetic-goal-test")}},
		{`INSERT INTO posts(id,author_id,kind,title,body) VALUES($1,$2,'status','Tracking a goal','Reply here')`, []any{post, agent}},
	} {
		if _, err := pool.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	create := url.Values{"csrf": {csrf}, "workspace": {org}, "agent": {agent}, "key": {"create-web"}, "title": {"Fix <script>unsafe()</script> onboarding"}, "outcome": {"Recover interrupted signup"}, "criteria": {"Regression test passes"}, "authority": {"Code changes only"}, "priority": {"normal"}, "source_post_id": {post}}
	bad := url.Values{}
	for k, v := range create {
		bad[k] = append([]string(nil), v...)
	}
	bad.Set("csrf", "wrong")
	f.want(f.request("POST", "/human/goals", bad, ""), 403)
	w := f.request("POST", "/human/goals", create, "")
	f.want(w, http.StatusSeeOther)
	location := w.Header().Get("Location")
	parsed, err := url.Parse(location)
	if err != nil {
		t.Fatal(err)
	}
	goalID := strings.TrimPrefix(parsed.Path, "/human/goals/")
	w = f.request("GET", location, nil, "")
	f.want(w, 200)
	if !strings.Contains(w.Body.String(), "&lt;script&gt;") || strings.Contains(w.Body.String(), "<script>unsafe") {
		t.Fatal("unsafe title rendering")
	}
	if !strings.Contains(w.Body.String(), "No agent update yet") {
		t.Fatal("fabricated activity")
	}
	w = f.request("GET", "/human/workspaces?workspace="+org, nil, "")
	f.want(w, 200)
	if !strings.Contains(w.Body.String(), `href="/human/goals/`+goalID+`"`) {
		t.Fatal("status post has no reply link", w.Body)
	}
	change := url.Values{"csrf": {csrf}, "version": {"1"}, "key": {"reply"}, "action": {"message"}, "body": {"Please prioritize recovery"}}
	f.want(f.request("POST", parsed.Path+"/events", change, ""), http.StatusSeeOther)
	stale := url.Values{}
	for k, v := range change {
		stale[k] = append([]string(nil), v...)
	}
	stale.Set("key", "stale")
	stale.Set("body", "Preserve this draft")
	w = f.request("POST", parsed.Path+"/events", stale, "")
	f.want(w, 409)
	if !strings.Contains(w.Body.String(), "Preserve this draft") || !strings.Contains(w.Body.String(), `name="version" value="2"`) {
		t.Fatal("stale draft lost", w.Body)
	}
	ev, err := f.s.goalService().Events(ctx, goals.Actor{HumanID: human}, goalID, 0)
	if err != nil || len(ev) != 2 || ev[0].ActorKind != "human" || ev[0].ActorID != human {
		t.Fatal("wrong attribution", ev, err)
	}
	settings := url.Values{"csrf": {csrf}, "workspace": {org}, "timezone": {"America/Los_Angeles"}, "hour": {"9"}, "enabled": {"yes"}}
	f.want(f.request("POST", "/human/goal-reports/settings", settings, ""), http.StatusSeeOther)
	f.want(f.request("GET", "/human/goals?workspace="+org+"&tab=updates", nil, ""), 200)
	other := newHumanFixture(t)
	other.register()
	other.want(other.request("GET", parsed.Path, nil, ""), 404)
	other.want(other.request("GET", "/human/goals?workspace="+org, nil, ""), 404)
	delete(f.cookies, "ajent_human")
	f.want(f.request("GET", parsed.Path, nil, ""), http.StatusSeeOther)
}
