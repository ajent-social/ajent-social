package web

import (
	"ajent/internal/api"
	"ajent/migrations"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Exercise the public routes against a real store: unavailable resources must
// preserve recovery navigation without revealing unpublished or private data.
func TestPublicMissingPagesPreserveNavigation(t *testing.T) {
	dsn := os.Getenv("AJENT_TEST_DSN")
	if dsn == "" {
		t.Skip("AJENT_TEST_DSN required")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := migrations.Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}
	mustExec := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	org, agent := uuid.NewString(), uuid.NewString()
	namespace := "unpublished-" + org + ".example"
	mustExec(`INSERT INTO organizations(id,name,namespace) VALUES($1,'hidden-organization-name',$2)`, org, namespace)
	mustExec(`INSERT INTO agents(id,org_id,handle,display_name,public_posting) VALUES($1,$2,'hidden-author','hidden-author-name',true)`, agent, org)
	mustExec(`INSERT INTO verified_domains(domain,org_id,verified_until) VALUES($1,$2,now()+interval '30 days')`, "proof-"+org+".example", org)
	mustExec(`INSERT INTO agent_domains(agent_id,domain) VALUES($1,$2)`, agent, "proof-"+org+".example")
	mustExec(`INSERT INTO directory_listings(org_id,name,description,published) VALUES($1,'hidden-listing-name','hidden-listing-description',false)`, org)
	privatePost, unpublishedPost := uuid.NewString(), uuid.NewString()
	mustExec(`INSERT INTO posts(id,author_id,kind,body,audience,web_published) VALUES($1,$2,'finding','hidden-private-post-body','private',true),($3,$2,'finding','hidden-network-post-body','public',false)`, privatePost, agent, unpublishedPost)
	s, err := New(api.New(pool), false, "http://example.com", "", false)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	s.Routes(mux)
	cases := []struct{ name, path string }{
		{"malformed domain namespace", "/domains/" + strings.Repeat("x", 254)},
		{"missing domain namespace", "/domains/missing-" + uuid.NewString() + ".example"},
		{"unpublished domain listing", "/domains/" + namespace},
		{"malformed discovery ID", "/explore/posts/not-a-uuid"},
		{"missing discovery ID", "/explore/posts/" + uuid.NewString()},
		{"private discovery", "/explore/posts/" + privatePost},
		{"unpublished discovery", "/explore/posts/" + unpublishedPost},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, tc.path, nil)
			r.Header.Set("Accept", "text/html")
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, r)
			if w.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want 404", w.Code)
			}
			if got := w.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
				t.Errorf("content type = %q, want HTML recovery page", got)
			}
			body := w.Body.String()
			for _, want := range []string{"<title>Not Found", "<main", "<h1>Not Found</h1>", `href="/"`, `href="/join"`, `href="/docs"`} {
				if !strings.Contains(body, want) {
					t.Errorf("missing recovery-page element %q", want)
				}
			}
			for _, hidden := range []string{org, agent, namespace, privatePost, unpublishedPost, "hidden-organization-name", "hidden-author", "hidden-listing-name", "hidden-listing-description", "hidden-private-post-body", "hidden-network-post-body"} {
				if strings.Contains(body, hidden) {
					t.Errorf("unavailable page disclosed private fixture data %q", hidden)
				}
			}
		})
	}
}
