package api

import (
	"ajent/internal/auth"
	"context"
	"encoding/json"
	"fmt"
	"testing"
)

func TestFleetAutojoinAuthorityExistingFutureAndExpiry(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	if _, err := f.s.DB.Pool.Exec(ctx, `DELETE FROM signup_buckets`); err != nil {
		t.Fatal(err)
	}
	root, _, _, _ := auth.Generate()
	w := f.request("POST", "/v1/signup", "", fmt.Sprintf(`{"token":%q,"handle":"autojoin-owner"}`, root), "")
	if w.Code != 201 {
		t.Fatalf("signup: %d %s", w.Code, w.Body)
	}
	var owner map[string]string
	json.Unmarshal(w.Body.Bytes(), &owner)
	var org string
	if err := f.s.DB.Pool.QueryRow(ctx, `SELECT org_id::text FROM agents WHERE id=$1`, owner["agent_id"]).Scan(&org); err != nil {
		t.Fatal(err)
	}
	enroll := func(profile, key string) map[string]string {
		t.Helper()
		w := f.request("POST", "/v1/fleet", root, fmt.Sprintf(`{"profile":%q,"token":%q}`, profile, key), "")
		if w.Code != 200 && w.Code != 201 {
			t.Fatalf("enroll %d %s", w.Code, w.Body)
		}
		var d map[string]string
		json.Unmarshal(w.Body.Bytes(), &d)
		return d
	}
	a, _, _, _ := auth.Generate()
	first := enroll("before-join", a)
	if first["autojoin_domain"] != "" {
		t.Fatal("opted in without permission")
	}
	configure := func(key, domain string, status int) {
		t.Helper()
		w := f.request("PUT", "/v1/fleet/autojoin", key, fmt.Sprintf(`{"domain":%q}`, domain), "")
		if w.Code != status {
			t.Fatalf("configure %d %s", w.Code, w.Body)
		}
	}
	configure(root, "sire.test", 403)
	if _, err := f.s.DB.Pool.Exec(ctx, `INSERT INTO verified_domains(domain,org_id,verified_until) VALUES('sire.test',$1,now()+interval '1 day'),('foreign.test',$2,now()+interval '1 day')`, org, f.org); err != nil {
		t.Fatal(err)
	}
	configure(a, "sire.test", 403)
	configure(root, "foreign.test", 403)
	configure(root, "sire.test", 200)
	var domain string
	if err := f.s.DB.Pool.QueryRow(ctx, `SELECT verified_agent_domain($1)`, first["agent_id"]).Scan(&domain); err != nil || domain != "sire.test" {
		t.Fatalf("existing profile not joined: %s %v", domain, err)
	}
	b, _, _, _ := auth.Generate()
	next := enroll("after-join", b)
	if next["autojoin_domain"] != "sire.test" {
		t.Fatal("future profile not joined")
	}
	if _, err := f.s.DB.Pool.Exec(ctx, `UPDATE verified_domains SET verified_until=now()-interval '1 second' WHERE domain='sire.test'`); err != nil {
		t.Fatal(err)
	}
	if enroll("after-join", b)["autojoin_domain"] != "" {
		t.Fatal("expired domain inherited")
	}
	configure(root, "", 200)
}
