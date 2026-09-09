package api

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestGroupPendingMembershipCannotDiscoverPrivateMetadata(t *testing.T) {
	f := setup(t)
	if _, err := f.s.DB.Pool.Exec(context.Background(), `INSERT INTO group_members(group_id,agent_id,state) VALUES($1,$2,'pending')`, f.g, f.b); err != nil {
		t.Fatal(err)
	}
	w := f.request("GET", "/v1/groups", f.tb, "", "")
	if w.Code != 200 || strings.Contains(w.Body.String(), f.g) {
		t.Fatalf("pending membership exposed private group: %d %s", w.Code, w.Body)
	}
}

func TestGroupLifecycle(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	request := func(method, suffix, token, body string, want int) string {
		t.Helper()
		w := f.request(method, "/v1/groups/"+f.g+suffix, token, body, uuid.NewString())
		if w.Code != want {
			t.Fatalf("%s %s: got %d want %d: %s", method, suffix, w.Code, want, w.Body)
		}
		return w.Body.String()
	}
	request("GET", "", f.tb, "", 404)
	request("GET", "", f.ta, "", 200)
	request("PATCH", "", f.ta, `{"version":1,"description":"Checked build advice","rules":"Record applicability."}`, 200)
	request("PATCH", "", f.ta, `{"version":1,"description":"stale"}`, 409)
	request("DELETE", "/membership", f.ta, `{"version":2}`, 409)
	root := f.post(f.ta, f.g, "")
	request("PUT", "/pins/"+root, f.ta, `{"version":2,"position":1}`, 200)
	other := f.post(f.ta, "", "")
	request("PUT", "/pins/"+other, f.ta, `{"version":3,"position":2}`, 400)
	request("PATCH", "", f.ta, `{"version":3,"archived":true}`, 200)
	w := f.request("POST", "/v1/posts", f.ta, fmt.Sprintf(`{"kind":"finding","body":"blocked","title":"Archived write","group_id":%q}`, f.g), uuid.NewString())
	if w.Code != 409 {
		t.Fatalf("archived group accepted post: %d %s", w.Code, w.Body)
	}
	request("GET", "", f.ta, "", 200)
	request("PATCH", "", f.ta, `{"version":4,"archived":false}`, 200)
	if _, err := f.s.DB.Pool.Exec(ctx, `INSERT INTO group_members(group_id,agent_id) VALUES($1,$2)`, f.g, f.b); err != nil {
		t.Fatal(err)
	}
	request("PUT", "/owner", f.ta, fmt.Sprintf(`{"version":5,"agent_id":%q}`, f.b), 200)
	request("PATCH", "", f.ta, `{"version":6,"archived":true}`, 404)
	request("DELETE", "/membership", f.ta, `{"version":6}`, 200)
	request("GET", "", f.ta, "", 404)
	request("GET", "", f.tb, "", 200)
}

func TestGroupStorageGuardsAndPinExpiry(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	root := f.post(f.ta, f.g, "")
	if _, err := f.s.DB.Pool.Exec(ctx, `INSERT INTO group_pins(group_id,post_id,position) VALUES($1,$2,1)`, f.g, root); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.DB.Pool.Exec(ctx, `DELETE FROM group_members WHERE group_id=$1 AND agent_id=$2`, f.g, f.a); err == nil {
		t.Fatal("last-owner deletion committed")
	}
	if _, err := f.s.DB.Pool.Exec(ctx, `UPDATE posts SET expires_at=now()-interval '1 minute' WHERE id=$1`, root); err != nil {
		t.Fatal(err)
	}
	w := f.request("GET", "/v1/groups/"+f.g, f.ta, "", "")
	if w.Code != 200 || strings.Contains(w.Body.String(), root) {
		t.Fatalf("expired pin exposed: %d %s", w.Code, w.Body)
	}
	if _, err := f.s.DB.Pool.Exec(ctx, `UPDATE groups SET archived_at=now() WHERE id=$1`, f.g); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.DB.Pool.Exec(ctx, `UPDATE posts SET body='',title='',structured='{}',deleted_at=now() WHERE id=$1`, root); err != nil {
		t.Fatalf("archive blocked retention erasure: %v", err)
	}
	if _, err := f.s.DB.Pool.Exec(ctx, `INSERT INTO posts(author_id,group_id,kind,body,expires_at) VALUES($1,$2,'status','direct writer',now()+interval '1 day')`, f.a, f.g); err == nil {
		t.Fatal("direct writer bypassed archive")
	}
}

func TestGroupReplayAndRevokedAuthority(t *testing.T) {
	f := setup(t)
	path := "/v1/groups/" + f.g
	key := uuid.NewString()
	body := `{"version":1,"rules":"Keep evidence scoped."}`
	first := f.request("PATCH", path, f.ta, body, key)
	again := f.request("PATCH", path, f.ta, body, key)
	if first.Code != 200 || again.Code != 200 || first.Body.String() != again.Body.String() {
		t.Fatalf("replay differs: %s / %s", first.Body, again.Body)
	}
	conflict := f.request("PATCH", path, f.ta, `{"version":2,"rules":"different"}`, key)
	if conflict.Code != 409 {
		t.Fatalf("key conflict: %d", conflict.Code)
	}
	if _, err := f.s.DB.Pool.Exec(context.Background(), `INSERT INTO group_members(group_id,agent_id,role) VALUES($1,$2,'owner')`, f.g, f.b); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.DB.Pool.Exec(context.Background(), `DELETE FROM group_members WHERE group_id=$1 AND agent_id=$2`, f.g, f.a); err != nil {
		t.Fatal(err)
	}
	denied := f.request("PATCH", path, f.ta, body, key)
	if denied.Code != 404 {
		t.Fatalf("removed member replay exposed authority: %d %s", denied.Code, denied.Body)
	}
}
