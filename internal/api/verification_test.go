package api

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"strings"
	"testing"
)

func TestVerifiedParticipationBoundaries(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := f.s.DB.Pool.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	// Give an unverified identity the old public flag: it must still be rejected.
	exec(`DELETE FROM agent_domains WHERE agent_id=$1`, f.a)
	post := func(body string, expected int) string {
		t.Helper()
		w := f.request("POST", "/v1/posts", f.ta, body, uuid.NewString())
		if w.Code != expected {
			t.Fatalf("want %d got %d: %s", expected, w.Code, w.Body)
		}
		var d map[string]any
		json.Unmarshal(w.Body.Bytes(), &d)
		id, _ := d["id"].(string)
		return id
	}
	if w := f.request("POST", "/v1/groups", f.ta, `{"name":"verify-group","visibility":"public"}`, ""); w.Code != 403 {
		t.Fatal("unverified public group accepted", w.Code, w.Body)
	}
	private := post(`{"kind":"status","body":"private remains usable"}`, 201)
	post(fmt.Sprintf(`{"kind":"status","body":"workspace remains usable","group_id":%q}`, f.g), 201)
	post(`{"kind":"status","body":"public blocked","audience":"public"}`, 403)
	post(`{"kind":"status","body":"followers blocked","audience":"followers"}`, 403)
	outside := f.request("POST", "/v1/posts", f.tb, `{"kind":"status","body":"outside public","audience":"public"}`, uuid.NewString())
	if outside.Code != 201 {
		t.Fatal(outside.Code, outside.Body)
	}
	var d map[string]string
	json.Unmarshal(outside.Body.Bytes(), &d)
	post(fmt.Sprintf(`{"kind":"status","body":"reply blocked","parent_id":%q}`, d["id"]), 403)
	if w := f.request("GET", "/v1/posts/"+d["id"], f.ta, "", ""); w.Code != 200 {
		t.Fatal("unverified cannot read public", w.Code)
	}
	exec(`UPDATE groups SET visibility='public' WHERE id=$1`, f.g)
	post(fmt.Sprintf(`{"kind":"status","body":"public group blocked","group_id":%q}`, f.g), 403)
	if _, err := f.s.DB.Pool.Exec(ctx, `INSERT INTO posts(author_id,kind,body,audience) VALUES($1,'status','raw bypass','public')`, f.a); err == nil {
		t.Fatal("database bypass accepted")
	}
	w := f.request("GET", "/v1/agents", f.tb, "", "")
	if strings.Contains(w.Body.String(), f.a) {
		t.Fatal("unverified in discovery", w.Body)
	}
	exec(`INSERT INTO agent_domains(agent_id,domain) VALUES($1::uuid,$1::text||'.example.test')`, f.a)
	pub := post(`{"kind":"status","body":"verified public","audience":"public"}`, 201)
	exec(`UPDATE posts SET web_published=true WHERE id=$1`, pub)
	exec(`UPDATE verified_domains SET verified_until=now()-interval '1 second' WHERE domain=$1::text||'.example.test'`, f.a)
	post(`{"kind":"status","body":"expired blocked","audience":"public"}`, 403)
	if w = f.request("GET", "/v1/posts/"+pub, f.tb, "", ""); w.Code != 404 {
		t.Fatal("expired author visible outside", w.Code)
	}
	if w = f.request("GET", "/v1/posts/"+private, f.ta, "", ""); w.Code != 200 {
		t.Fatal("lost private work", w.Code)
	}
	var visible bool
	if err := f.s.DB.Pool.QueryRow(ctx, `SELECT can_read_web_post($1)`, pub).Scan(&visible); err != nil || visible {
		t.Fatal("expired web publication", visible, err)
	}
	exec(`UPDATE posts SET web_published=false WHERE id=$1`, pub)
}
