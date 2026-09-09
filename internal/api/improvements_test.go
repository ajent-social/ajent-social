package api

import (
	"ajent/internal/events"
	"ajent/internal/jobs"
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"strings"
	"sync"
	"testing"
	"time"
)

func feedbackPayload(id string) string {
	return fmt.Sprintf(`{"occurrence_id":%q,"policy_version":1,"category":"bug","evidence":{"route":"/v1/feed","method":"GET","error_code":"unavailable","status":503,"attempt_count":3,"client_version":"0.4.0"},"attempted_job":"Read the task handoff","observed":"Feed failed","workaround_minutes":5}`, id)
}
func feedbackID(t *testing.T, body []byte) string {
	t.Helper()
	var v map[string]any
	if err := json.Unmarshal(body, &v); err != nil {
		t.Fatal(err)
	}
	return v["report_id"].(string)
}
func TestImprovementReplayPrivacyAndWithdrawal(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	occ := uuid.NewString()
	payload := feedbackPayload(occ)
	w := f.request("POST", "/v1/improvements", f.ta, payload, "")
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body)
	}
	id := feedbackID(t, w.Body.Bytes())
	for i := 0; i < 2; i++ {
		w = f.request("POST", "/v1/improvements", f.ta, payload, "")
		if w.Code != 200 || feedbackID(t, w.Body.Bytes()) != id {
			t.Fatal(w.Code, w.Body)
		}
	}
	w = f.request("POST", "/v1/improvements", f.ta, strings.Replace(payload, "Feed failed", "Different", 1), "")
	if w.Code != 409 {
		t.Fatal("changed payload", w.Code, w.Body)
	}
	for _, method := range []string{"GET", "DELETE"} {
		w = f.request(method, "/v1/improvements/"+id, f.tb, "", "")
		if w.Code != 404 {
			t.Fatal("foreign", method, w.Code, w.Body)
		}
	}
	w = f.request("GET", "/v1/improvements", f.ta, "", "")
	if w.Code != 403 {
		t.Fatal("operator selfgrant", w.Code)
	}
	for _, bad := range []string{strings.Replace(payload, `"policy_version":1`, `"policy_version":2`, 1), strings.Replace(payload, `"/v1/feed"`, `"/v1/posts/private-secret"`, 1), strings.Replace(payload, `"bug"`, `"feature"`, 1), strings.Replace(payload, `"observed":`, `"reporter_id":"forged","observed":`, 1)} {
		if w = f.request("POST", "/v1/improvements", f.ta, bad, ""); w.Code != 400 {
			t.Fatal("validation", w.Code, w.Body)
		}
	}
	for i := 0; i < 2; i++ {
		if w = f.request("DELETE", "/v1/improvements/"+id, f.ta, "", ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"withdrawn"`) {
			t.Fatal("withdrawal", w.Code, w.Body)
		}
	}
	var count int
	var contextJSON, evidence string
	if err := f.s.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM improvement_occurrences WHERE reporter_id=$1`, f.a).Scan(&count); err != nil || count != 1 {
		t.Fatal("one occurrence", count, err)
	}
	if err := f.s.DB.Pool.QueryRow(ctx, `SELECT context::text,evidence::text FROM improvement_occurrences WHERE reporter_id=$1`, f.a).Scan(&contextJSON, &evidence); err != nil || contextJSON != "{}" || evidence != "{}" {
		t.Fatal("retained context", contextJSON, evidence, err)
	}
}
func TestImprovementConcurrentGrouping(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	codes := make(chan int, 8)
	payload := feedbackPayload(uuid.NewString())
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); codes <- f.request("POST", "/v1/improvements", f.ta, payload, "").Code }()
	}
	wg.Wait()
	close(codes)
	created := 0
	for c := range codes {
		if c == 201 {
			created++
		} else if c != 200 {
			t.Fatal("concurrent response", c)
		}
	}
	if created != 1 {
		t.Fatal("created", created)
	}
	if _, err := f.s.DB.Pool.Exec(ctx, `UPDATE agents SET org_id=$1 WHERE id=$2`, f.org, f.b); err != nil {
		t.Fatal(err)
	}
	a := f.request("POST", "/v1/improvements", f.ta, feedbackPayload(uuid.NewString()), "")
	b := f.request("POST", "/v1/improvements", f.tb, feedbackPayload(uuid.NewString()), "")
	if a.Code != 200 || b.Code != 200 {
		t.Fatal(a.Code, a.Body, b.Code, b.Body)
	}
	aid, bid := feedbackID(t, a.Body.Bytes()), feedbackID(t, b.Body.Bytes())
	if aid == bid {
		t.Fatal("shared receipt leaked")
	}
	if w := f.request("GET", "/v1/improvements/"+aid, f.tb, "", ""); w.Code != 404 {
		t.Fatal("foreign receipt", w.Code)
	}
	var count int
	if err := f.s.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM improvement_reports WHERE org_id=$1`, f.org).Scan(&count); err != nil || count != 1 {
		t.Fatal(count, err)
	}
}
func TestImprovementResolutionInboxAndRecurrence(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	w := f.request("POST", "/v1/improvements", f.ta, feedbackPayload(uuid.NewString()), "")
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body)
	}
	receipt := feedbackID(t, w.Body.Bytes())
	var canonical string
	if err := f.s.DB.Pool.QueryRow(ctx, `SELECT report_id::text FROM improvement_receipts WHERE id=$1`, receipt).Scan(&canonical); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.DB.Pool.Exec(ctx, `INSERT INTO improvement_operators(agent_id) VALUES($1)`, f.b); err != nil {
		t.Fatal(err)
	}
	path := "/v1/improvements/" + canonical
	if w = f.request("PATCH", path, f.ta, `{"version":1,"status":"triaged"}`, ""); w.Code != 403 {
		t.Fatal(w.Code, w.Body)
	}
	if w = f.request("PATCH", path, f.tb, `{"version":1,"status":"verified"}`, ""); w.Code != 409 {
		t.Fatal("operator can't verify", w.Code, w.Body)
	}
	for _, bad := range []string{`{"version":1,"issue_url":"https://evil.example/x"}`, `{"version":1,"issue_url":"https://github.com/o/r/issues/1?secret=x"}`} {
		if w = f.request("PATCH", path, f.tb, bad, ""); w.Code != 400 {
			t.Fatal(w.Code, w.Body)
		}
	}
	steps := []string{`{"version":1,"status":"triaged","workaround":"Try the persisted inbox."}`, `{"version":2,"issue_url":"https://github.com/ajent-social/ajent-social/issues/1"}`, `{"version":3,"status":"planned"}`, `{"version":4,"status":"shipped_awaiting_verification","deployed_revision":"fix001"}`}
	for _, body := range steps {
		if w = f.request("PATCH", path, f.tb, body, ""); w.Code != 200 {
			t.Fatal(w.Code, w.Body)
		}
	}
	if w = f.request("PATCH", path, f.tb, steps[0], ""); w.Code != 409 {
		t.Fatal("stale update", w.Code, w.Body)
	}
	if w = f.request("GET", "/v1/improvements/"+receipt, f.ta, "", ""); w.Code != 200 || !strings.Contains(w.Body.String(), "Try the persisted inbox.") || strings.Contains(w.Body.String(), canonical) {
		t.Fatal("projection", w.Code, w.Body)
	}
	inbox := f.request("GET", "/v1/inbox", f.ta, "", "")
	if inbox.Code != 200 || !strings.Contains(inbox.Body.String(), "improvement_verification") || !strings.Contains(inbox.Body.String(), receipt) {
		t.Fatal("typed inbox", inbox.Code, inbox.Body)
	}
	again := f.request("GET", "/v1/inbox", f.ta, "", "")
	if inbox.Body.String() != again.Body.String() {
		t.Fatal("redelivery changed")
	}
	foreign := f.request("GET", "/v1/inbox", f.tb, "", "")
	if strings.Contains(foreign.Body.String(), receipt) {
		t.Fatal("prompt leaked")
	}
	verifyPath := "/v1/improvements/" + receipt + "/verify"
	if w = f.request("POST", verifyPath, f.tb, fmt.Sprintf(`{"verification_id":%q,"deployed_revision":"fix001","outcome":"not_reproduced","evidence":"Retried the feed"}`, uuid.NewString()), ""); w.Code != 404 {
		t.Fatal("foreign verify", w.Code)
	}
	unable := fmt.Sprintf(`{"verification_id":%q,"deployed_revision":"fix001","outcome":"unable_to_verify"}`, uuid.NewString())
	if w = f.request("POST", verifyPath, f.ta, unable, ""); w.Code != 200 || !strings.Contains(w.Body.String(), "shipped_awaiting_verification") {
		t.Fatal("unable passed", w.Code, w.Body)
	}
	verified := fmt.Sprintf(`{"verification_id":%q,"deployed_revision":"fix001","outcome":"not_reproduced","evidence":"Retried original feed and received handoff"}`, uuid.NewString())
	for i := 0; i < 2; i++ {
		if w = f.request("POST", verifyPath, f.ta, verified, ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"verified"`) {
			t.Fatal("verify replay", w.Code, w.Body)
		}
	}
	recurrence := fmt.Sprintf(`{"verification_id":%q,"deployed_revision":"fix001","outcome":"reproduced","evidence":"Same feed failed on next original-task retry"}`, uuid.NewString())
	if w = f.request("POST", verifyPath, f.ta, recurrence, ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"triaged"`) {
		t.Fatal("recurrence", w.Code, w.Body)
	}
	if w = f.request("GET", "/v1/improvements/metrics", f.tb, "", ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"eligible_operation_denominator":null`) {
		t.Fatal("metrics", w.Code, w.Body)
	}
	if _, err := f.s.DB.Pool.Exec(ctx, `UPDATE improvement_occurrences SET created_at=now()-interval '91 days' WHERE receipt_id=$1`, receipt); err != nil {
		t.Fatal(err)
	}
	if err := jobs.Cleanup(ctx, f.s.DB.Pool); err != nil {
		t.Fatal(err)
	}
	var evidence string
	if err := f.s.DB.Pool.QueryRow(ctx, `SELECT context::text FROM improvement_occurrences WHERE receipt_id=$1`, receipt).Scan(&evidence); err != nil || evidence != "{}" {
		t.Fatal("retention", evidence, err)
	}
}

func TestImprovementLimitsExpiryAndMerge(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	createFeature := func(job string) string {
		t.Helper()
		body := fmt.Sprintf(`{"occurrence_id":%q,"policy_version":1,"category":"feature","attempted_job":%q,"expected":"A missing tool"}`, uuid.NewString(), job)
		w := f.request("POST", "/v1/improvements", f.ta, body, "")
		if w.Code != 201 {
			t.Fatal(w.Code, w.Body)
		}
		return feedbackID(t, w.Body.Bytes())
	}
	source, target := createFeature("First task"), createFeature("Second task")
	if _, err := f.s.DB.Pool.Exec(ctx, `INSERT INTO improvement_operators(agent_id) VALUES($1)`, f.b); err != nil {
		t.Fatal(err)
	}
	var a, b string
	for receipt, dest := range map[string]*string{source: &a, target: &b} {
		if err := f.s.DB.Pool.QueryRow(ctx, `SELECT report_id::text FROM improvement_receipts WHERE id=$1`, receipt).Scan(dest); err != nil {
			t.Fatal(err)
		}
	}
	w := f.request("PATCH", "/v1/improvements/"+a, f.tb, fmt.Sprintf(`{"version":1,"merge_into":%q,"merge_version":1}`, b), "")
	if w.Code != 200 {
		t.Fatal("merge", w.Code, w.Body)
	}
	for i, state := range []string{"triaged", "planned", "shipped_awaiting_verification"} {
		w = f.request("PATCH", "/v1/improvements/"+b, f.tb, fmt.Sprintf(`{"version":%d,"status":%q,"deployed_revision":"mergedfix"}`, i+2, state), "")
		if i < 2 { // Revisions must not be edited before shipping.
			if w.Code != 409 {
				t.Fatal("premature revision", w.Code, w.Body)
			}
			w = f.request("PATCH", "/v1/improvements/"+b, f.tb, fmt.Sprintf(`{"version":%d,"status":%q}`, i+2, state), "")
		}
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body)
		}
	}
	w = f.request("GET", "/v1/improvements/"+source, f.ta, "", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "shipped_awaiting_verification") {
		t.Fatal("merged receipt lost", w.Code, w.Body)
	}
	verify := fmt.Sprintf(`{"verification_id":%q,"deployed_revision":"mergedfix","outcome":"not_reproduced","evidence":"Retried missing workflow"}`, uuid.NewString())
	w = f.request("POST", "/v1/improvements/"+source+"/verify", f.ta, verify, "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	w = f.request("POST", "/v1/improvements/"+target+"/verify", f.ta, verify, "")
	if w.Code != 409 {
		t.Fatal("verification replay crossreceipt", w.Code, w.Body)
	}
	if _, err := f.s.DB.Pool.Exec(ctx, `UPDATE improvement_prompts SET created_at=now()-interval '31 days' WHERE receipt_id=$1`, target); err != nil {
		t.Fatal(err)
	}
	expired := strings.Replace(verify, `"verification_id":`, `"verification_id":`, 1)
	var parsed map[string]any
	if err := json.Unmarshal([]byte(expired), &parsed); err != nil {
		t.Fatal(err)
	}
	parsed["verification_id"] = uuid.NewString()
	raw, _ := json.Marshal(parsed)
	w = f.request("POST", "/v1/improvements/"+target+"/verify", f.ta, string(raw), "")
	if w.Code != 409 {
		t.Fatal("expired verified", w.Code, w.Body)
	}
	// Existing occurrence replay bypasses occurrence allowance, new work does not.
	fresh := feedbackPayload(uuid.NewString())
	w = f.request("POST", "/v1/improvements", f.ta, fresh, "")
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body)
	}
	receipt := feedbackID(t, w.Body.Bytes())
	if _, err := f.s.DB.Pool.Exec(ctx, `INSERT INTO improvement_occurrences(reporter_id,occurrence_id,receipt_id,payload_hash) SELECT $1,gen_random_uuid(),$2,'synthetic quota fixture' FROM generate_series(1,100)`, f.a, receipt); err != nil {
		t.Fatal(err)
	}
	if w = f.request("POST", "/v1/improvements", f.ta, fresh, ""); w.Code != 200 {
		t.Fatal("quota replay", w.Code, w.Body)
	}
	if w = f.request("POST", "/v1/improvements", f.ta, feedbackPayload(uuid.NewString()), ""); w.Code != 429 || w.Header().Get("Retry-After") == "" {
		t.Fatal("quota", w.Code, w.Body)
	}
}

func TestImprovementPromptPublicationOrdering(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	w := f.request("POST", "/v1/improvements", f.ta, feedbackPayload(uuid.NewString()), "")
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body)
	}
	receipt := feedbackID(t, w.Body.Bytes())
	var canonical string
	if err := f.s.DB.Pool.QueryRow(ctx, `SELECT report_id::text FROM improvement_receipts WHERE id=$1`, receipt).Scan(&canonical); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.DB.Pool.Exec(ctx, `INSERT INTO improvement_operators(agent_id) VALUES($1)`, f.b); err != nil {
		t.Fatal(err)
	}
	for i, state := range []string{"triaged", "planned"} {
		w = f.request("PATCH", "/v1/improvements/"+canonical, f.tb, fmt.Sprintf(`{"version":%d,"status":%q}`, i+1, state), "")
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body)
		}
	}
	held, err := f.s.DB.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Rollback(ctx)
	if _, err = held.Exec(ctx, `SELECT pg_advisory_xact_lock(4242)`); err != nil {
		t.Fatal(err)
	}
	done := make(chan int, 1)
	go func() {
		done <- f.request("PATCH", "/v1/improvements/"+canonical, f.tb, `{"version":3,"status":"shipped_awaiting_verification","deployed_revision":"orderedfix"}`, "").Code
	}()
	deadline := time.Now().Add(3 * time.Second)
	for {
		select {
		case code := <-done:
			t.Fatalf("prompt committed outside publisher lock: %d", code)
		default:
		}
		var waiting bool
		if err = f.s.DB.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_locks WHERE locktype='advisory' AND objid=4242 AND NOT granted)`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("prompt did not wait on publisher lock")
		}
	}
	if err = held.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case code := <-done:
		if code != 200 {
			t.Fatal(code)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("prompt never completed")
	}
	f.post(f.ta, f.g, "")
	if _, err = events.PublishOnce(ctx, f.s.DB.Pool); err != nil {
		t.Fatal(err)
	}
	var promptSeq, postSeq int64
	if err = f.s.DB.Pool.QueryRow(ctx, `SELECT (SELECT seq FROM improvement_prompts WHERE receipt_id=$1),(SELECT max(seq) FROM stream_events)`, receipt).Scan(&promptSeq, &postSeq); err != nil || promptSeq >= postSeq {
		t.Fatal("sequence order", promptSeq, postSeq, err)
	}
	w = f.request("GET", "/v1/inbox", f.ta, "", "")
	var box inboxResponse
	if err = json.Unmarshal(w.Body.Bytes(), &box); err != nil {
		t.Fatal(err)
	}
	if len(box.Items) != 2 || box.Items[0].ID != receipt {
		t.Fatal("ordered typed delivery", w.Body)
	}
	w = f.request("POST", "/v1/inbox/ack", f.ta, fmt.Sprintf(`{"through":%d}`, box.Through), "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	w = f.request("GET", "/v1/inbox", f.ta, "", "")
	if strings.Contains(w.Body.String(), receipt) {
		t.Fatal("ack lost", w.Body)
	}
}

func TestImprovementRetentionClosedAndInactive(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	w := f.request("POST", "/v1/improvements", f.ta, feedbackPayload(uuid.NewString()), "")
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body)
	}
	receipt := feedbackID(t, w.Body.Bytes())
	var canonical string
	if err := f.s.DB.Pool.QueryRow(ctx, `SELECT report_id::text FROM improvement_receipts WHERE id=$1`, receipt).Scan(&canonical); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.DB.Pool.Exec(ctx, `UPDATE improvement_reports SET status='verified',updated_at=now()-interval '31 days' WHERE id=$1`, canonical); err != nil {
		t.Fatal(err)
	}
	if err := jobs.Cleanup(ctx, f.s.DB.Pool); err != nil {
		t.Fatal(err)
	}
	var body string
	if err := f.s.DB.Pool.QueryRow(ctx, `SELECT context::text FROM improvement_occurrences WHERE receipt_id=$1`, receipt).Scan(&body); err != nil || body != "{}" {
		t.Fatal("closed context retained", body, err)
	}
	// An active merge target preserves its inactive source and original receipts.
	var target string
	if err := f.s.DB.Pool.QueryRow(ctx, `INSERT INTO improvement_reports(org_id,signature,category) VALUES($1,$2,'feature') RETURNING id::text`, f.org, uuid.NewString()).Scan(&target); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.DB.Pool.Exec(ctx, `UPDATE improvement_reports SET merged_into=$2,status='merged',updated_at=now()-interval '91 days' WHERE id=$1`, canonical, target); err != nil {
		t.Fatal(err)
	}
	if err := jobs.Cleanup(ctx, f.s.DB.Pool); err != nil {
		t.Fatal(err)
	}
	if w = f.request("GET", "/v1/improvements/"+receipt, f.ta, "", ""); w.Code != 200 {
		t.Fatal("active merge lost receipt", w.Code, w.Body)
	}
	if _, err := f.s.DB.Pool.Exec(ctx, `UPDATE improvement_reports SET updated_at=now()-interval '91 days' WHERE id=$1`, target); err != nil {
		t.Fatal(err)
	}
	if err := jobs.Cleanup(ctx, f.s.DB.Pool); err != nil {
		t.Fatal(err)
	}
	if w = f.request("GET", "/v1/improvements/"+receipt, f.ta, "", ""); w.Code != 404 {
		t.Fatal("inactive metadata retained", w.Code, w.Body)
	}
	var count int
	if err := f.s.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM improvement_occurrences WHERE reporter_id=$1`, f.a).Scan(&count); err != nil || count != 0 {
		t.Fatal("inactive replay hash retained", count, err)
	}
}
