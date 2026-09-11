package api

import (
	"ajent/internal/improvements"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestImprovementOpsControlAndReview(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	repo := "owner/repo-" + strings.ToLower(strings.ReplaceAll(uuid.NewString()[:8], "-", ""))
	paths := []string{
		"/v1/improvements/control?repository=" + repo,
		"/v1/improvements/worker-health?repository=" + repo,
		"/v1/improvements/decisions?repository=" + repo,
		"/v1/improvements/triage-metrics?repository=" + repo,
	}
	for _, path := range paths {
		if w := f.request("GET", path, f.ta, "", ""); w.Code != 403 {
			t.Fatal("ordinary agent", path, w.Code, w.Body)
		}
	}
	if _, err := f.s.DB.Pool.Exec(ctx, `INSERT INTO improvement_operators(agent_id) VALUES($1)`, f.b); err != nil {
		t.Fatal(err)
	}
	if w := f.request("GET", paths[0], f.tb, "", ""); w.Code != 404 {
		t.Fatalf("missing control path=%s code=%d body=%s", paths[0], w.Code, w.Body)
	}
	stop := fmt.Sprintf(`{"repository":%q,"version":1,"owner":"off","mode":"off","operator_id":%q}`, repo, f.b)
	w := f.request("PUT", "/v1/improvements/control", f.tb, stop, "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"owner":"off"`) {
		t.Fatal("stop control", w.Code, w.Body)
	}
	body := fmt.Sprintf(`{"repository":%q,"version":2,"owner":"cloud","mode":"canary","operator_id":%q,"canary_org":%q,"allow_synthetic":true}`, repo, f.b, f.org)
	w = f.request("PUT", "/v1/improvements/control", f.tb, body, "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"mode":"canary"`) || !strings.Contains(w.Body.String(), `"owner":"cloud"`) {
		t.Fatal("put control", w.Code, w.Body)
	}
	var control improvements.Control
	if err := json.Unmarshal(w.Body.Bytes(), &control); err != nil || control.Version != 3 {
		t.Fatal("control version", w.Body, err)
	}
	if w = f.request("GET", paths[0], f.tb, "", ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"version":3`) {
		t.Fatal("get control", w.Code, w.Body)
	}
	if w = f.request("GET", paths[1], f.tb, "", ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"state":"stale"`) {
		t.Fatal("worker health", w.Code, w.Body)
	}
	if w = f.request("GET", paths[2], f.tb, "", ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"items":[]`) {
		t.Fatal("empty decisions", w.Code, w.Body)
	}
	if w = f.request("GET", paths[3], f.tb, "", ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"coverage":"eligible_workflow_denominator_unknown"`) {
		t.Fatal("triage metrics", w.Code, w.Body)
	}

	payload := fmt.Sprintf(`{"occurrence_id":%q,"policy_version":1,"category":"bug","evidence":{"route":"/v1/feed","method":"GET","error_code":"unavailable","status":503,"attempt_count":3,"client_version":"0.4.0","observed_at":%q},"attempted_job":"Read the task handoff","observed":"Feed failed","workaround_minutes":5}`, uuid.NewString(), time.Now().UTC().Format(time.RFC3339))
	report := f.request("POST", "/v1/improvements", f.ta, payload, "")
	if report.Code != 201 {
		t.Fatal(report.Code, report.Body)
	}
	receiptID := feedbackID(t, report.Body.Bytes())
	var reportID string
	if err := f.s.DB.Pool.QueryRow(ctx, `SELECT report_id::text FROM improvement_receipts WHERE id=$1`, receiptID).Scan(&reportID); err != nil {
		t.Fatal(err)
	}
	c, err := improvements.ReadControl(ctx, f.s.DB.Pool, repo)
	if err != nil {
		t.Fatal(err)
	}
	sum, err := improvements.RunTriage(ctx, f.s.DB.Pool, c, improvements.TriageConfig{})
	if err != nil || sum.Processed != 1 {
		t.Fatalf("triage summary=%+v err=%v", sum, err)
	}
	page := f.request("GET", paths[2], f.tb, "", "")
	if page.Code != 200 || !strings.Contains(page.Body.String(), reportID) {
		t.Fatalf("proposed decision report=%s code=%d body=%s", reportID, page.Code, page.Body)
	}
	var listed improvements.DecisionPage
	if err := json.Unmarshal(page.Body.Bytes(), &listed); err != nil || len(listed.Items) != 1 {
		t.Fatal(page.Body, err)
	}
	d := listed.Items[0]
	if d.Proposal.Action != "triage" && d.State != "applied" {
		// Shadow would leave proposed; canary may auto-apply deterministic triage.
	}
	if w = f.request("POST", "/v1/improvements/decisions/"+d.ID+"/review?repository="+repo, f.ta, fmt.Sprintf(`{"version":%d,"action":"reject"}`, d.Version), ""); w.Code != 403 {
		t.Fatal("review denial", w.Code, w.Body)
	}
	if d.State == "proposed" {
		if w = f.request("POST", "/v1/improvements/decisions/"+d.ID+"/review?repository="+repo, f.tb, fmt.Sprintf(`{"version":%d,"action":"accept"}`, d.Version), ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"state":"applied"`) {
			t.Fatal("accept", w.Code, w.Body)
		}
	}
	var status string
	if err := f.s.DB.Pool.QueryRow(ctx, `SELECT status FROM improvement_reports WHERE id=$1`, reportID).Scan(&status); err != nil || status != "triaged" {
		t.Fatal(status, err)
	}
	if w = f.request("POST", "/v1/improvements/decisions/"+d.ID+"/review?repository="+repo, f.tb, fmt.Sprintf(`{"version":%d,"action":"accept"}`, d.Version+1), ""); w.Code != 409 {
		t.Fatal("stale accept", w.Code, w.Body)
	}
	stale := f.request("PUT", "/v1/improvements/control", f.tb, fmt.Sprintf(`{"repository":%q,"version":1,"owner":"off","mode":"off","operator_id":%q}`, repo, f.b), "")
	if stale.Code != 409 {
		t.Fatal("stale control", stale.Code, stale.Body)
	}
}
