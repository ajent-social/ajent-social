package web

import (
	"ajent/internal/auth"
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestHumanForeignOwnershipWithValidCSRF(t *testing.T) {
	owner := newHumanFixture(t)
	ownerKey, _ := owner.register()
	other := newHumanFixture(t)
	otherKey, _ := other.register()
	ctx := context.Background()
	pool := owner.s.API.DB.Pool
	must := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	session := func(f *humanFixture) (string, string) {
		t.Helper()
		var id, csrf string
		if err := pool.QueryRow(ctx, `SELECT human_id::text,csrf FROM human_sessions WHERE digest=$1`, auth.Digest(f.cookies["ajent_human"].Value)).Scan(&id, &csrf); err != nil {
			t.Fatal(err)
		}
		return id, csrf
	}
	ownerID, ownerCSRF := session(owner)
	otherID, otherCSRF := session(other)
	if ownerID == otherID || ownerCSRF == otherCSRF {
		t.Fatal("fixture humans and CSRF must differ")
	}
	foreignOrg, ownOrg, agent, credential := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	domain := "foreign-" + uuid.NewString() + ".example"
	must(`INSERT INTO organizations(id,name,namespace,human_owner_id) VALUES($1,'foreign-private-workspace',$2,$3),($4,'own-workspace',$5,$6)`, foreignOrg, "foreign-"+foreignOrg, ownerID, ownOrg, "own-"+ownOrg, otherID)
	must(`INSERT INTO agents(id,org_id,handle) VALUES($1,$2,'foreign-private-agent')`, agent, foreignOrg)
	_, lookup, digest, err := auth.Generate()
	if err != nil {
		t.Fatal(err)
	}
	must(`INSERT INTO credentials(id,agent_id,lookup_id,secret_digest,scopes) VALUES($1,$2,$3,$4,ARRAY['network:read'])`, credential, agent, lookup, digest)
	must(`INSERT INTO human_installations(credential_id,human_id) VALUES($1,$2)`, credential, ownerID)
	must(`INSERT INTO verified_domains(domain,org_id,verified_until) VALUES($1,$2,now()+interval '30 days')`, domain, foreignOrg)
	must(`INSERT INTO agent_domains(agent_id,domain) VALUES($1,$2)`, agent, domain)
	// Two passkeys on the requesting account make the last-passkey guard nonblocking.
	extraKey := []byte(uuid.NewString())
	must(`INSERT INTO human_passkeys(id,human_id,credential) SELECT $1,human_id,credential FROM human_passkeys WHERE id=$2`, extraKey, otherKey.id)
	assertForeignUnchanged := func(t *testing.T) {
		t.Helper()
		var intact bool
		err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM credentials WHERE id=$1 AND revoked_at IS NULL) AND EXISTS(SELECT 1 FROM organizations WHERE id=$2 AND human_owner_id=$3) AND EXISTS(SELECT 1 FROM verified_domains WHERE domain=$4 AND org_id=$2 AND verified_until>now()) AND EXISTS(SELECT 1 FROM agent_domains WHERE agent_id=$5 AND domain=$4) AND EXISTS(SELECT 1 FROM human_passkeys WHERE id=$6 AND human_id=$3) AND EXISTS(SELECT 1 FROM human_installations WHERE credential_id=$1 AND human_id=$3)`, credential, foreignOrg, ownerID, domain, agent, ownerKey.id).Scan(&intact)
		if err != nil || !intact {
			t.Fatalf("foreign state changed: intact=%v err=%v", intact, err)
		}
	}
	t.Run("valid requester CSRF permits own domain challenge", func(t *testing.T) {
		w := other.request("POST", "/human/domain", url.Values{"csrf": {otherCSRF}, "workspace": {ownOrg}, "domain": {"own-" + ownOrg + ".example"}, "action": {"challenge"}}, "")
		if w.Code != http.StatusSeeOther {
			t.Fatalf("status=%d want 303: %s", w.Code, w.Body)
		}
		var count int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM domain_challenges WHERE org_id=$1`, ownOrg).Scan(&count); err != nil || count != 1 {
			t.Fatalf("own challenge not persisted: count=%d err=%v", count, err)
		}
	})
	for _, tc := range []struct{ name, kind, id string }{{"foreign installation", "installation", credential}, {"foreign agent", "agent", agent}, {"foreign passkey", "passkey", b64(ownerKey.id)}} {
		t.Run(tc.name, func(t *testing.T) {
			w := other.request("POST", "/human/revoke", url.Values{"csrf": {otherCSRF}, "workspace": {foreignOrg}, "kind": {tc.kind}, "id": {tc.id}}, "")
			if w.Code != http.StatusConflict {
				t.Fatalf("status=%d want 409: %s", w.Code, w.Body)
			}
			if !strings.Contains(w.Body.String(), "Unavailable") {
				t.Fatalf("did not reach resource authorization: %s", w.Body)
			}
			assertForeignUnchanged(t)
		})
	}
	for _, action := range []string{"challenge", "check", "revoke"} {
		t.Run("foreign workspace domain "+action, func(t *testing.T) {
			w := other.request("POST", "/human/domain", url.Values{"csrf": {otherCSRF}, "workspace": {foreignOrg}, "domain": {domain}, "action": {action}}, "")
			if w.Code != http.StatusForbidden {
				t.Fatalf("status=%d want 403: %s", w.Code, w.Body)
			}
			if !strings.Contains(w.Body.String(), "Workspace ownership required") {
				t.Fatalf("wrong denial: %s", w.Body)
			}
			assertForeignUnchanged(t)
		})
	}
	for _, existing := range []bool{false, true} {
		name := "new installation into foreign workspace"
		if existing {
			name = "claim foreign installation"
		}
		t.Run(name, func(t *testing.T) {
			proof, err := random()
			if err != nil {
				t.Fatal(err)
			}
			_, lookup, digest, err := auth.Generate()
			if err != nil {
				t.Fatal(err)
			}
			var existingID any
			if existing {
				existingID = credential
			}
			must(`INSERT INTO installation_links(lookup_id,secret_digest,approval_digest,handle,existing_credential) VALUES($1,$2,$3,'synthetic-foreign-claim',$4)`, lookup, digest, auth.Digest(proof), existingID)
			w := other.request("POST", "/human/link/approve", url.Values{"csrf": {otherCSRF}, "workspace": {foreignOrg}, "token": {proof}, "confirm": {"yes"}, "scope": {"user"}}, "")
			if w.Code != http.StatusForbidden {
				t.Fatalf("status=%d want 403: %s", w.Code, w.Body)
			}
			if !strings.Contains(w.Body.String(), "You do not own this workspace") {
				t.Fatalf("wrong denial: %s", w.Body)
			}
			var untouched bool
			if err := pool.QueryRow(ctx, `SELECT approved_credential IS NULL FROM installation_links WHERE lookup_id=$1`, lookup).Scan(&untouched); err != nil || !untouched {
				t.Fatalf("foreign link approved: untouched=%v err=%v", untouched, err)
			}
			assertForeignUnchanged(t)
		})
	}
	t.Run("foreign workspace dashboard", func(t *testing.T) {
		w := other.request("GET", "/human/workspaces?workspace="+foreignOrg, nil, "")
		if w.Code != http.StatusNotFound {
			t.Fatalf("status=%d want 404: %s", w.Code, w.Body)
		}
		for _, private := range []string{"foreign-private-workspace", "foreign-private-agent", domain} {
			if strings.Contains(w.Body.String(), private) {
				t.Errorf("foreign dashboard leaked %q", private)
			}
		}
	})
	t.Run("valid requester CSRF permits own extra passkey removal", func(t *testing.T) {
		w := other.request("POST", "/human/revoke", url.Values{"csrf": {otherCSRF}, "workspace": {ownOrg}, "kind": {"passkey"}, "id": {b64(extraKey)}}, "")
		if w.Code != http.StatusSeeOther {
			t.Fatalf("status=%d want 303: %s", w.Code, w.Body)
		}
		var removed bool
		if err := pool.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM human_passkeys WHERE id=$1)`, extraKey).Scan(&removed); err != nil || !removed {
			t.Fatalf("own passkey not removed: removed=%v err=%v", removed, err)
		}
		assertForeignUnchanged(t)
	})
}
