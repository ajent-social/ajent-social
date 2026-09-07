package api

import (
	"ajent/internal/auth"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestPublicSignup(t *testing.T) {
	f := setup(t)
	// This suite uses a disposable database, and signup limits are deployment-wide.
	_, err := f.s.DB.Pool.Exec(context.Background(), `DELETE FROM signup_buckets`)
	if err != nil {
		t.Fatal(err)
	}
	signup := func(token, extra string, ip string) *httptest.ResponseRecorder {
		b := fmt.Sprintf(`{"token":%q,"handle":"new-agent"%s}`, token, extra)
		r := httptest.NewRequest("POST", "/v1/signup", strings.NewReader(b))
		r.RemoteAddr = ip
		w := httptest.NewRecorder()
		f.m.ServeHTTP(w, r)
		return w
	}
	token, _, _, _ := auth.Generate()
	bad := signup(token, `,"org_id":"`+f.org+`"`, "192.0.2.1:1")
	if bad.Code != 400 {
		t.Fatalf("organization injection: %d", bad.Code)
	}
	w := signup(token, "", "192.0.2.1:1")
	if w.Code != 201 {
		t.Fatalf("signup: %d %s", w.Code, w.Body)
	}
	if strings.Contains(w.Body.String(), token) {
		t.Fatal("key echoed")
	}
	var result map[string]string
	json.Unmarshal(w.Body.Bytes(), &result)
	replay := signup(token, "", "192.0.2.1:1")
	if replay.Code != 200 || replay.Body.String() != w.Body.String() {
		t.Fatalf("retry did not preserve identity: %s", replay.Body)
	}
	me := f.request("GET", "/v1/me", token, "", "")
	if me.Code != 200 || strings.Contains(me.Body.String(), f.org) {
		t.Fatalf("identity isolation: %s", me.Body)
	}
	private := f.post(f.ta, f.g, "")
	if got := f.request("GET", "/v1/posts/"+private, token, "", ""); got.Code != 404 {
		t.Fatal("private fixture leaked")
	}
	if got := f.request("POST", "/v1/agents", token, `{}`, ""); got.Code != 401 {
		t.Fatal("signup obtained operator access")
	}
	if got := f.request("POST", "/v1/posts", token, `{"kind":"status","body":"hello","audience":"public"}`, uuid.NewString()); got.Code != 403 {
		t.Fatal("signup enabled public posting")
	}
	f.post(token, "", "")
	for i := 0; i < 4; i++ {
		k, _, _, _ := auth.Generate()
		if got := signup(k, "", "192.0.2.1:2"); got.Code != 201 {
			t.Fatalf("admission %d: %d", i, got.Code)
		}
	}
	k, _, _, _ := auth.Generate()
	if got := signup(k, "", "192.0.2.1:3"); got.Code != 429 || got.Header().Get("Retry-After") == "" {
		t.Fatal("per-IP cap not enforced")
	}
	if got := signup(token, "", "192.0.2.1:3"); got.Code != 200 {
		t.Fatal("cap blocked retry")
	}
	_, err = f.s.DB.Pool.Exec(context.Background(), `UPDATE credentials SET revoked_at=now() WHERE id=$1`, result["credential_id"])
	if err != nil {
		t.Fatal(err)
	}
	if got := signup(token, "", "192.0.2.1:3"); got.Code != 409 {
		t.Fatal("revoked key reenrolled")
	}
	_, err = f.s.DB.Pool.Exec(context.Background(), `UPDATE signup_buckets SET count=100 WHERE subject='global'`)
	if err != nil {
		t.Fatal(err)
	}
	k, _, _, _ = auth.Generate()
	if got := signup(k, "", "192.0.2.2:1"); got.Code != 429 {
		t.Fatal("global cap not enforced")
	}
}
