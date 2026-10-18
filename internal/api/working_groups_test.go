package api

import (
	"ajent/internal/events"
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"strings"
	"sync"
	"testing"
)

func wgSQL(t *testing.T, f *fixture, q string, args ...any) {
	t.Helper()
	if _, err := f.s.DB.Pool.Exec(context.Background(), q, args...); err != nil {
		t.Fatal(err)
	}
}
func wgID(t *testing.T, body []byte) string {
	t.Helper()
	var v struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &v); err != nil || v.ID == "" {
		t.Fatalf("invalid group: %s", body)
	}
	return v.ID
}
func wgSubscribe(t *testing.T, f *fixture, id string, on bool) {
	t.Helper()
	w := f.request("PUT", "/v1/groups/"+id+"/subscription", f.ta, fmt.Sprintf(`{"subscribed":%t}`, on), uuid.NewString())
	if w.Code != 200 {
		t.Fatalf("subscribe %d %s", w.Code, w.Body)
	}
}
func wgInbox(t *testing.T, f *fixture, id string) inboxResponse {
	t.Helper()
	w := f.request("GET", "/v1/inbox?group_id="+id, f.ta, "", "")
	if w.Code != 200 {
		t.Fatalf("inbox %d %s", w.Code, w.Body)
	}
	var v inboxResponse
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(w.Body.String(), "goal_updates") {
		t.Fatal("unrelated goals")
	}
	return v
}

func TestWorkingGroupsPurposeAtomicAndPrivate(t *testing.T) {
	f := setup(t)
	body := `{"name":"purpose-test","description":"bounded","visibility":"private","purpose_key":"review"}`
	var wg sync.WaitGroup
	ids := make(chan string, 8)
	errs := make(chan string, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := f.request("POST", "/v1/groups", f.ta, body, uuid.NewString())
			if w.Code != 201 {
				errs <- fmt.Sprintf("%d %s", w.Code, w.Body)
				return
			}
			var v struct {
				ID string `json:"id"`
			}
			_ = json.Unmarshal(w.Body.Bytes(), &v)
			ids <- v.ID
		}()
	}
	wg.Wait()
	close(ids)
	close(errs)
	for e := range errs {
		t.Error(e)
	}
	seen := map[string]bool{}
	for id := range ids {
		seen[id] = true
	}
	if len(seen) != 1 {
		t.Fatalf("purpose race: %v", seen)
	}
	for id := range seen {
		wgSQL(t, f, `UPDATE group_members SET agent_id=$2 WHERE group_id=$1 AND agent_id=$3`, id, f.b, f.a)
	}
	w := f.request("POST", "/v1/groups", f.ta, body, uuid.NewString())
	if w.Code != 409 || strings.Contains(w.Body.String(), "purpose-test") {
		t.Fatalf("inaccessible purpose %d %s", w.Code, w.Body)
	}
}

func TestWorkingGroupsDirectCanonicalAndLifecycle(t *testing.T) {
	f := setup(t)
	wgSQL(t, f, `UPDATE agents SET org_id=$1 WHERE id=$2`, f.org, f.b)
	var wg sync.WaitGroup
	ids := make(chan string, 8)
	errs := make(chan string, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			token, target := f.ta, f.b
			if i%2 == 1 {
				token, target = f.tb, f.a
			}
			w := f.request("POST", "/v1/groups/direct", token, fmt.Sprintf(`{"agent_id":%q,"purpose_key":"review"}`, target), uuid.NewString())
			if w.Code != 201 {
				errs <- fmt.Sprintf("%d %s", w.Code, w.Body)
				return
			}
			var v struct {
				ID string `json:"id"`
			}
			_ = json.Unmarshal(w.Body.Bytes(), &v)
			ids <- v.ID
		}(i)
	}
	wg.Wait()
	close(ids)
	close(errs)
	for e := range errs {
		t.Error(e)
	}
	seen := map[string]bool{}
	id := ""
	for x := range ids {
		seen[x] = true
		id = x
	}
	if len(seen) != 1 {
		t.Fatalf("pair race: %v", seen)
	}
	body := fmt.Sprintf(`{"agent_id":%q,"purpose_key":"other"}`, f.b)
	w := f.request("POST", "/v1/groups/direct", f.ta, body, uuid.NewString())
	if w.Code != 201 || wgID(t, w.Body.Bytes()) == id {
		t.Fatalf("distinct purpose %d %s", w.Code, w.Body)
	}
	wgSQL(t, f, `UPDATE groups SET archived_at=now() WHERE id=$1`, id)
	w = f.request("POST", "/v1/groups/direct", f.ta, fmt.Sprintf(`{"agent_id":%q,"purpose_key":"review"}`, f.b), uuid.NewString())
	if w.Code != 409 || strings.Contains(w.Body.String(), id) {
		t.Fatalf("archive leaked/reused %d %s", w.Code, w.Body)
	}
	wgSQL(t, f, `UPDATE groups SET archived_at=NULL WHERE id=$1`, id)
	wgSQL(t, f, `DELETE FROM group_members WHERE group_id=$1 AND role<>'owner'`, id)
	w = f.request("POST", "/v1/groups/direct", f.ta, fmt.Sprintf(`{"agent_id":%q,"purpose_key":"review"}`, f.b), uuid.NewString())
	if w.Code != 409 {
		t.Fatalf("changed pair reused %d %s", w.Code, w.Body)
	}
}

func TestWorkingGroupsRejectTargetsAndHardenInvitation(t *testing.T) {
	for _, mode := range []string{"foreign", "inactive", "blocked", "self"} {
		t.Run(mode, func(t *testing.T) {
			f := setup(t)
			target := f.b
			if mode != "foreign" {
				wgSQL(t, f, `UPDATE agents SET org_id=$1 WHERE id=$2`, f.org, f.b)
			}
			switch mode {
			case "inactive":
				wgSQL(t, f, `UPDATE agents SET status='disabled' WHERE id=$1`, f.b)
			case "blocked":
				wgSQL(t, f, `INSERT INTO blocks(blocker_id,blocked_id) VALUES($1,$2)`, f.b, f.a)
			case "self":
				target = f.a
			}
			w := f.request("POST", "/v1/groups/direct", f.ta, fmt.Sprintf(`{"agent_id":%q,"purpose_key":"review"}`, target), uuid.NewString())
			if w.Code != 404 && w.Code != 400 {
				t.Fatalf("target accepted %d %s", w.Code, w.Body)
			}
			if mode != "self" {
				w = f.request("POST", "/v1/groups/"+f.g+"/members/"+target, f.ta, `{"action":"add"}`, uuid.NewString())
				if w.Code != 404 {
					t.Fatalf("invite accepted %d %s", w.Code, w.Body)
				}
			}
		})
	}
}

func TestWorkingGroupsScopedInboxIsolation(t *testing.T) {
	f := setup(t)
	w := f.request("POST", "/v1/groups", f.ta, `{"name":"second","visibility":"private"}`, uuid.NewString())
	if w.Code != 201 {
		t.Fatal(w.Body)
	}
	second := wgID(t, w.Body.Bytes())
	// First subscription excludes already-published history, even after an initial unsubscribe.
	wgSubscribe(t, f, f.g, false)
	f.post(f.ta, f.g, "")
	if _, err := events.PublishOnce(context.Background(), f.s.DB.Pool); err != nil {
		t.Fatal(err)
	}
	wgSubscribe(t, f, f.g, true)
	wgSubscribe(t, f, second, true)
	if got := wgInbox(t, f, f.g); len(got.Items) != 0 {
		t.Fatal("history replayed")
	}
	a := f.post(f.ta, f.g, "")
	b := f.post(f.ta, second, "")
	if _, err := events.PublishOnce(context.Background(), f.s.DB.Pool); err != nil {
		t.Fatal(err)
	}
	aa, bb := wgInbox(t, f, f.g), wgInbox(t, f, second)
	if len(aa.Items) != 1 || aa.Items[0].ID != a || len(bb.Items) != 1 || bb.Items[0].ID != b {
		t.Fatalf("scoped results %+v %+v", aa, bb)
	}
	ack := func(id string, n int64, token string) int {
		return f.request("POST", "/v1/inbox/ack", token, fmt.Sprintf(`{"group_id":%q,"through":%d}`, id, n), uuid.NewString()).Code
	}
	if code := ack(f.g, aa.Through+100000, f.ta); code != 409 {
		t.Fatalf("future ack %d", code)
	}
	if code := ack(f.g, aa.Through, f.tb); code != 404 {
		t.Fatalf("foreign ack %d", code)
	}
	if code := ack(f.g, aa.Through, f.ta); code != 200 {
		t.Fatalf("ack %d", code)
	}
	if got := wgInbox(t, f, second); got.Acknowledged != bb.Acknowledged || len(got.Items) != 1 {
		t.Fatal("group ack crossed cursors")
	}
	var global int
	if err := f.s.DB.Pool.QueryRow(context.Background(), `SELECT count(*) FROM agent_inbox WHERE agent_id=$1 AND acknowledged_seq>0`, f.a).Scan(&global); err != nil || global != 0 {
		t.Fatalf("global cursor changed: %d %v", global, err)
	}
	globalResponse := f.request("GET", "/v1/inbox", f.ta, "", "")
	if globalResponse.Code != 200 {
		t.Fatal(globalResponse.Body)
	}
	var globalBox inboxResponse
	if err := json.Unmarshal(globalResponse.Body.Bytes(), &globalBox); err != nil {
		t.Fatal(err)
	}
	if w := f.request("POST", "/v1/inbox/ack", f.ta, fmt.Sprintf(`{"through":%d}`, globalBox.Through), ""); w.Code != 200 {
		t.Fatal(w.Body)
	}
	if got := wgInbox(t, f, second); got.Acknowledged != bb.Acknowledged || len(got.Items) != 1 {
		t.Fatal("global ack consumed group delivery")
	}
	wgSubscribe(t, f, second, false)
	if got := f.request("GET", "/v1/inbox?group_id="+second, f.ta, "", ""); got.Code != 404 {
		t.Fatal("unsubscribed delivery")
	}
	wgSubscribe(t, f, second, true)
	if got := wgInbox(t, f, second); len(got.Items) != 1 {
		t.Fatal("resubscribe reset cursor")
	}
	wgSQL(t, f, `UPDATE groups SET archived_at=now() WHERE id=$1`, second)
	if code := ack(second, bb.Through, f.ta); code != 404 {
		t.Fatalf("archive ack %d", code)
	}
}

func TestWorkingGroupsSubscriptionRequiresMembership(t *testing.T) {
	f := setup(t)
	w := f.request("PUT", "/v1/groups/"+f.g+"/subscription", f.tb, `{"subscribed":true}`, uuid.NewString())
	if w.Code != 404 {
		t.Fatalf("subscription grants access %d", w.Code)
	}
	w = f.request("GET", "/v1/inbox?group_id="+f.g, f.ta, "", "")
	if w.Code != 404 {
		t.Fatalf("automatic subscription %d", w.Code)
	}
	wgSubscribe(t, f, f.g, true)
	wgSQL(t, f, `UPDATE groups SET archived_at=now() WHERE id=$1`, f.g)
	w = f.request("GET", "/v1/inbox?group_id="+f.g, f.ta, "", "")
	if w.Code != 404 {
		t.Fatal("archive delivery")
	}
}

func TestWorkingGroupsReplayAndParameterConflicts(t *testing.T) {
	f := setup(t)
	key := uuid.NewString()
	body := `{"name":"keyed","visibility":"private","purpose_key":"build"}`
	first := f.request("POST", "/v1/groups", f.ta, body, key)
	if first.Code != 201 {
		t.Fatal(first.Body)
	}
	id := wgID(t, first.Body.Bytes())
	for _, k := range []string{key, uuid.NewString()} {
		w := f.request("POST", "/v1/groups", f.ta, body, k)
		if w.Code != 201 || wgID(t, w.Body.Bytes()) != id {
			t.Fatalf("replay %d %s", w.Code, w.Body)
		}
	}
	for _, k := range []string{key, uuid.NewString()} {
		w := f.request("POST", "/v1/groups", f.ta, strings.Replace(body, "keyed", "changed", 1), k)
		if w.Code != 409 {
			t.Fatalf("parameter conflict %d %s", w.Code, w.Body)
		}
	}
	wgSQL(t, f, `UPDATE agents SET org_id=$1 WHERE id=$2`, f.org, f.b)
	directBody := fmt.Sprintf(`{"agent_id":%q,"purpose_key":"pair"}`, f.b)
	key = uuid.NewString()
	w := f.request("POST", "/v1/groups/direct", f.ta, directBody, key)
	if w.Code != 201 {
		t.Fatal(w.Body)
	}
	id = wgID(t, w.Body.Bytes())
	wgSQL(t, f, `INSERT INTO blocks(blocker_id,blocked_id) VALUES($1,$2)`, f.a, f.b)
	w = f.request("POST", "/v1/groups/direct", f.ta, directBody, key)
	if w.Code != 404 || strings.Contains(w.Body.String(), id) {
		t.Fatalf("replay bypassed block %d %s", w.Code, w.Body)
	}
}

func TestWorkingGroupsSubscriptionPaginationAndRetry(t *testing.T) {
	f := setup(t)
	wgSQL(t, f, `WITH made AS (INSERT INTO groups(org_id,name) SELECT $1,'page-'||gen_random_uuid()::text FROM generate_series(1,105) RETURNING id), members AS (INSERT INTO group_members(group_id,agent_id,role) SELECT id,$2,'owner' FROM made) INSERT INTO group_subscriptions(agent_id,group_id,subscribed,acknowledged_seq,delivered_seq) SELECT $2,id,true,0,0 FROM made`, f.org, f.a)
	cursor := ""
	seen := map[string]bool{}
	for page := 0; page < 2; page++ {
		w := f.request("GET", "/v1/groups/subscriptions?cursor="+cursor, f.ta, "", "")
		if w.Code != 200 {
			t.Fatalf("list %d %s", w.Code, w.Body)
		}
		var v struct {
			Items []struct {
				GroupID string `json:"group_id"`
			} `json:"items"`
			More bool   `json:"has_more"`
			Next string `json:"next_cursor"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
			t.Fatal(err)
		}
		if len(v.Items) > 100 {
			t.Fatal("unbounded list")
		}
		for _, x := range v.Items {
			if seen[x.GroupID] {
				t.Fatal("duplicate page")
			}
			seen[x.GroupID] = true
		}
		if page == 0 && !v.More {
			t.Fatal("missing page")
		}
		if page == 1 && v.More {
			t.Fatal("unexpected page")
		}
		cursor = v.Next
	}
	if len(seen) != 105 {
		t.Fatalf("lost entries %d", len(seen))
	}
	key := uuid.NewString()
	path := "/v1/groups/" + f.g + "/subscription"
	for i := 0; i < 2; i++ {
		w := f.request("PUT", path, f.ta, `{"subscribed":true}`, key)
		if w.Code != 200 {
			t.Fatal(w.Body)
		}
	}
	if w := f.request("PUT", path, f.ta, `{"subscribed":false}`, key); w.Code != 409 {
		t.Fatal("changed retry accepted")
	}
	wgSubscribe(t, f, f.g, false)
	wgSQL(t, f, `INSERT INTO group_members(group_id,agent_id,role) VALUES($1,$2,'owner')`, f.g, f.b)
	wgSQL(t, f, `UPDATE group_members SET state='pending' WHERE group_id=$1 AND agent_id=$2`, f.g, f.a)
	if w := f.request("PUT", path, f.ta, `{"subscribed":true}`, key); w.Code != 404 {
		t.Fatal("replay bypassed membership")
	}
}

func TestWorkingGroupsInboxBoundsRemovalAndCompaction(t *testing.T) {
	f := setup(t)
	wgSubscribe(t, f, f.g, true)
	for i := 0; i < 23; i++ {
		f.post(f.ta, f.g, "")
	}
	if _, err := events.PublishOnce(context.Background(), f.s.DB.Pool); err != nil {
		t.Fatal(err)
	}
	first := wgInbox(t, f, f.g)
	if len(first.Items) != 20 || !first.More {
		t.Fatalf("bound %+v", first)
	}
	key := uuid.NewString()
	body := fmt.Sprintf(`{"group_id":%q,"through":%d}`, f.g, first.Through)
	for i := 0; i < 2; i++ {
		if w := f.request("POST", "/v1/inbox/ack", f.ta, body, key); w.Code != 200 {
			t.Fatal(w.Body)
		}
	}
	rest := wgInbox(t, f, f.g)
	if len(rest.Items) != 3 || rest.More {
		t.Fatalf("remaining %+v", rest)
	}
	// Simulate retention watermark without deleting other fixtures' stream data.
	wgSQL(t, f, `UPDATE group_subscriptions SET acknowledged_seq=0 WHERE agent_id=$1 AND group_id=$2`, f.a, f.g)
	var prior int64
	if err := f.s.DB.Pool.QueryRow(context.Background(), `SELECT purged_through FROM stream_watermark`).Scan(&prior); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { wgSQL(t, f, `UPDATE stream_watermark SET purged_through=$1`, prior) })
	wgSQL(t, f, `UPDATE stream_watermark SET purged_through=greatest(purged_through,$1)`, first.Through)
	if !wgInbox(t, f, f.g).Gap {
		t.Fatal("missing history gap")
	}
	// Pending members cannot fetch or acknowledge even with a live subscription.
	wgSQL(t, f, `INSERT INTO group_members(group_id,agent_id,role) VALUES($1,$2,'owner')`, f.g, f.b)
	wgSQL(t, f, `DELETE FROM group_members WHERE group_id=$1 AND agent_id=$2`, f.g, f.a)
	if w := f.request("GET", "/v1/inbox?group_id="+f.g, f.ta, "", ""); w.Code != 404 {
		t.Fatal("removed member delivered")
	}
	if w := f.request("POST", "/v1/inbox/ack", f.ta, body, key); w.Code != 404 {
		t.Fatal("removed member ack replayed")
	}
}

func TestWorkingGroupsImmediateInviteJoinAndStableKeys(t *testing.T) {
	f := setup(t)
	wgSQL(t, f, `UPDATE agents SET org_id=$1 WHERE id=$2`, f.org, f.b)
	wgSQL(t, f, `INSERT INTO group_members(group_id,agent_id,state) VALUES($1,$2,'pending')`, f.g, f.b)
	path := "/v1/groups/" + f.g + "/members/" + f.b
	key := uuid.NewString()
	for i := 0; i < 2; i++ {
		w := f.request("POST", path, f.ta, `{"action":"add"}`, key)
		if w.Code != 200 {
			t.Fatal(w.Body)
		}
	}
	var state string
	if err := f.s.DB.Pool.QueryRow(context.Background(), `SELECT state FROM group_members WHERE group_id=$1 AND agent_id=$2`, f.g, f.b).Scan(&state); err != nil || state != "accepted" {
		t.Fatalf("invite not immediate %s %v", state, err)
	}
	key = uuid.NewString()
	w := f.request("POST", "/v1/groups/"+f.g+"/join", f.tb, `{}`, key)
	if w.Code != 200 {
		t.Fatal(w.Body)
	}
	// A key used for an already-joined no-op is still bound to that operation.
	w = f.request("POST", path, f.tb, `{"action":"add"}`, key)
	if w.Code != 404 {
		t.Fatal("nonmanager authority changed")
	}
	w = f.request("POST", "/v1/groups", f.tb, `{"name":"retry-conflict","visibility":"private"}`, key)
	if w.Code != 409 {
		t.Fatalf("joined no-op failed to bind key %d %s", w.Code, w.Body)
	}
	var n int
	if err := f.s.DB.Pool.QueryRow(context.Background(), `SELECT count(*) FROM group_subscriptions WHERE group_id=$1`, f.g).Scan(&n); err != nil || n != 0 {
		t.Fatalf("automatic subscription %d %v", n, err)
	}
}

func TestWorkingGroupsCapabilityAndInvalidInputs(t *testing.T) {
	f := setup(t)
	w := f.request("GET", "/v1/reliability", f.ta, "", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"working_groups"`) || !strings.Contains(w.Body.String(), `"group_lifecycle"`) {
		t.Fatalf("capabilities %d %s", w.Code, w.Body)
	}
	for _, path := range []string{"/v1/inbox?group_id=", "/v1/inbox?group_id=bad", "/v1/groups/subscriptions?cursor=bad"} {
		w = f.request("GET", path, f.ta, "", "")
		if w.Code != 400 {
			t.Fatalf("invalid input %s: %d", path, w.Code)
		}
	}
	for _, body := range []string{`{"through":-1,"group_id":"bad"}`, `{"group_id":"bad"}`, fmt.Sprintf(`{"through":0,"group_id":%q}`, f.g)} {
		w = f.request("POST", "/v1/inbox/ack", f.ta, body, "")
		if w.Code != 400 && w.Code != 404 {
			t.Fatalf("invalid ack %d %s", w.Code, w.Body)
		}
	}
	w = f.request("PUT", "/v1/groups/"+f.g+"/subscription", f.ta, `{"subscribed":true}`, "")
	if w.Code != 400 {
		t.Fatal("subscription accepted without key")
	}
	wgSQL(t, f, `UPDATE agents SET org_id=$1 WHERE id=$2`, f.org, f.b)
	w = f.request("POST", "/v1/groups/direct", f.ta, fmt.Sprintf(`{"agent_id":%q,"purpose_key":"review"}`, f.b), "")
	if w.Code != 400 {
		t.Fatalf("direct without key: %d %s", w.Code, w.Body)
	}
}
