package api

import (
	"ajent/internal/auth"
	"ajent/internal/domains"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestScopedDomainEnrollmentAndConversation(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	must := func(sql string, args ...any) {
		t.Helper()
		if _, err := f.s.DB.Pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	must(`DELETE FROM signup_buckets`)
	root, _, _, _ := auth.Generate()
	response := f.request("POST", "/v1/signup", "", fmt.Sprintf(`{"token":%q,"handle":%q}`, root, "scope-"+uuid.NewString()[:8]), "")
	if response.Code != 201 {
		t.Fatal(response.Body)
	}
	var owner map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &owner); err != nil {
		t.Fatal(err)
	}
	var org string
	if err := f.s.DB.Pool.QueryRow(ctx, `SELECT org_id::text FROM agents WHERE id=$1`, owner["agent_id"]).Scan(&org); err != nil {
		t.Fatal(err)
	}
	a, b := uuid.NewString()+".test", uuid.NewString()+".test"
	for _, domain := range []string{a, b} {
		must(`INSERT INTO verified_domains(domain,org_id,verified_until) VALUES($1,$2,now()+interval '30 days')`, domain, org)
	}
	project := "project:" + strings.Repeat("a", 64)
	grant := func(domain, scope string) string {
		t.Helper()
		w := f.request("POST", "/v1/domain-grants", root, fmt.Sprintf(`{"domain":%q,"scope":%q}`, domain, scope), "")
		if w.Code != 200 {
			t.Fatalf("grant %d %s", w.Code, w.Body)
		}
		var g map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &g); err != nil {
			t.Fatal(err)
		}
		return g["id"]
	}
	ga, gb := grant(a, "user"), grant(b, project)
	if ga != grant(a, "user") {
		t.Fatal("grant retry duplicated authority")
	}
	child, _, _, _ := auth.Generate()
	enroll := func(domain, scope string, grants []string, private bool, status int) map[string]string {
		t.Helper()
		raw, _ := json.Marshal(map[string]any{"profile": "codex-scoped", "token": child, "scope": scope, "grants": grants, "domain": domain, "private_only": private})
		w := f.request("POST", "/v1/fleet", root, string(raw), "")
		if w.Code != status {
			t.Fatalf("enroll %d wanted %d: %s", w.Code, status, w.Body)
		}
		out := map[string]string{}
		if status < 300 {
			if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
				t.Fatal(err)
			}
		}
		return out
	}
	enroll(b, "project:"+strings.Repeat("b", 64), []string{gb}, false, 403)
	enrolled := enroll(a, project, []string{ga, gb}, false, 201)
	if w := f.request("POST", "/v1/domain-grants", child, fmt.Sprintf(`{"domain":%q,"scope":"user"}`, a), ""); w.Code != 403 {
		t.Fatal("child minted grant")
	}
	if w := f.request("POST", "/v1/domain-grants", root, fmt.Sprintf(`{"domain":%q,"scope":"user"}`, f.b+".example.test"), ""); w.Code != 403 {
		t.Fatal("foreign domain granted")
	}
	if w := f.request("GET", "/v1/me", child, "", ""); !strings.Contains(w.Body.String(), a) || !strings.Contains(w.Body.String(), b) {
		t.Fatal("available domains missing", w.Body)
	}
	post := func(domain, parent string, status int) string {
		t.Helper()
		body, _ := json.Marshal(map[string]string{"kind": "question", "body": "Scoped conversation", "audience": "public", "domain": domain, "parent_id": parent})
		w := f.request("POST", "/v1/posts", child, string(body), uuid.NewString())
		if w.Code != status {
			t.Fatalf("post %d wanted %d: %s", w.Code, status, w.Body)
		}
		var out map[string]any
		if status < 300 {
			if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
				t.Fatal(err)
			}
			return out["id"].(string)
		}
		return ""
	}
	first := post("", "", 201)
	enroll(b, project, []string{ga, gb}, false, 200)
	reply := post("", first, 201)
	for _, id := range []string{first, reply} {
		var d string
		if err := f.s.DB.Pool.QueryRow(ctx, `SELECT domain FROM posts WHERE id=$1`, id).Scan(&d); err != nil || d != a {
			t.Fatal("conversation relabeled", d, err)
		}
	}
	post(b, first, 409)
	second := post("", "", 201)
	if w := f.request("DELETE", "/v1/domain-grants/"+ga, child, "", ""); w.Code != 403 {
		t.Fatal("child revoked grant")
	}
	if w := f.request("DELETE", "/v1/domain-grants/"+ga, root, "", ""); w.Code != 200 {
		t.Fatal(w.Body)
	}
	if w := f.request("GET", "/v1/posts/"+first, f.tb, "", ""); w.Code != 404 {
		t.Fatal("revoked domain content visible", w.Body)
	}
	if w := f.request("GET", "/v1/posts/"+second, f.tb, "", ""); w.Code != 200 {
		t.Fatal("unrelated domain hidden", w.Body)
	}
	post(a, "", 403)
	must(`UPDATE agents SET domain_scope_disabled=true,public_posting=false WHERE id=$1`, enrolled["agent_id"])
	enroll(b, project, []string{gb}, false, 200)
	if w := f.request("GET", "/v1/me", child, "", ""); strings.Contains(w.Body.String(), `"public_posting":true`) {
		t.Fatal("human disable was undone by enrollment")
	}
	enroll("", project, nil, true, 200)
	if w := f.request("GET", "/v1/me", child, "", ""); strings.Contains(w.Body.String(), `"public_posting":true`) {
		t.Fatal("private override failed")
	}
	body, _ := json.Marshal(map[string]string{"kind": "question", "body": "Still private", "group_id": enrolled["group_id"]})
	if w := f.request("POST", "/v1/posts", child, string(body), uuid.NewString()); w.Code != 201 {
		t.Fatal("private work denied", w.Body)
	}
}

func TestDomainProofAndRenewal(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	// Mark the fixture credential as an installation using the same signup path.
	if _, err := f.s.DB.Pool.Exec(ctx, `DELETE FROM signup_buckets`); err != nil {
		t.Fatal(err)
	}
	root, _, _, _ := auth.Generate()
	w := f.request("POST", "/v1/signup", "", fmt.Sprintf(`{"token":%q,"handle":%q}`, root, "proof-"+uuid.NewString()[:8]), "")
	if w.Code != 201 {
		t.Fatal(w.Body)
	}
	domain := uuid.NewString() + ".test"
	challenge := f.request("POST", "/v1/domains/proof", root, fmt.Sprintf(`{"domain":%q,"action":"challenge"}`, domain), "")
	if challenge.Code != 200 {
		t.Fatal(challenge.Body)
	}
	var proof map[string]string
	if err := json.Unmarshal(challenge.Body.Bytes(), &proof); err != nil {
		t.Fatal(err)
	}
	check := func(match bool) int {
		r := httptest.NewRequest("POST", "/v1/domains/proof", strings.NewReader(fmt.Sprintf(`{"domain":%q,"action":"check"}`, domain)))
		r.Header.Set("Authorization", "Bearer "+root)
		w := httptest.NewRecorder()
		f.s.domainProof(w, r, func(_ context.Context, name string) ([]string, error) {
			if name != "_ajent-verification."+domain {
				t.Fatal(name)
			}
			if match {
				return []string{proof["value"]}, nil
			}
			return nil, nil
		})
		return w.Code
	}
	if check(false) != 409 || check(true) != 200 {
		t.Fatal("DNS proof lifecycle failed")
	}
	if _, err := f.s.DB.Pool.Exec(ctx, `UPDATE verified_domains SET next_check_at=now()-interval '1 day',verified_until=now()+interval '2 days' WHERE domain=$1`, domain); err != nil {
		t.Fatal(err)
	}
	if err := domains.Renew(ctx, f.s.DB.Pool, func(context.Context, string) ([]string, error) { return []string{proof["value"]}, nil }); err != nil {
		t.Fatal(err)
	}
	var extended bool
	if err := f.s.DB.Pool.QueryRow(ctx, `SELECT verified_until>now()+interval '29 days' FROM verified_domains WHERE domain=$1`, domain).Scan(&extended); err != nil || !extended {
		t.Fatal("lease not extended", err)
	}
	if _, err := f.s.DB.Pool.Exec(ctx, `UPDATE verified_domains SET next_check_at=now()-interval '1 day',verified_until=now()+interval '2 days' WHERE domain=$1`, domain); err != nil {
		t.Fatal(err)
	}
	if err := domains.Renew(ctx, f.s.DB.Pool, func(context.Context, string) ([]string, error) { return nil, nil }); err != nil {
		t.Fatal(err)
	}
	if err := f.s.DB.Pool.QueryRow(ctx, `SELECT verified_until<now()+interval '3 days' AND last_check_error<>'' AND next_check_at>now() FROM verified_domains WHERE domain=$1`, domain).Scan(&extended); err != nil || !extended {
		t.Fatal("failed proof extended lease", err)
	}
}
