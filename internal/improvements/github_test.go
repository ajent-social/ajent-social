package improvements

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

const testReportID = "6264c312-b226-4e48-ae10-c335aaa5dbd3"
const privateSentinel = "PRIVATE-REPORTER-CONTEXT-NEVER-PUBLISH"

func projection(t *testing.T) Projection {
	t.Helper()
	p, e := ReportProjection("Owner/Repo", testReportID, "bug", "/v1/search", false)
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func fixture(t *testing.T, h http.HandlerFunc) *GitHub {
	t.Helper()
	t.Setenv("AJENT_GITHUB_TEST_TOKEN", "test-token")
	s := httptest.NewServer(h)
	t.Cleanup(s.Close)
	g, e := NewGitHub(GitHubConfig{Repository: "Owner/Repo", TokenEnv: "AJENT_GITHUB_TEST_TOKEN"})
	if e != nil {
		t.Fatal(e)
	}
	g.base = s.URL
	return g
}
func checkError(t *testing.T, e error, kind string, ambiguous bool) {
	t.Helper()
	var x *GitHubError
	if !errors.As(e, &x) || x.Kind != kind || x.Ambiguous != ambiguous {
		t.Fatalf("error=%v want kind=%s ambiguous=%t", e, kind, ambiguous)
	}
	if strings.Contains(e.Error(), privateSentinel) {
		t.Fatal("private body in error")
	}
}
func row(n int, body string) map[string]any {
	return map[string]any{"html_url": fmt.Sprintf("https://github.com/owner/repo/issues/%d", n), "number": n, "body": body, "state": "closed"}
}
func emit(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	if e := json.NewEncoder(w).Encode(v); e != nil {
		t.Error(e)
	}
}
func TestGitHubProjection(t *testing.T) {
	p := projection(t)
	want := fmt.Sprintf("<!-- ajent-improvement:%x -->", sha256.Sum256([]byte("owner/repo:"+testReportID)))
	if p.Marker != want || p.Title != "Ajent bug: investigate search friction" {
		t.Fatal(p)
	}
	generic, e := ReportProjection("owner/repo", testReportID, "feature", privateSentinel, true)
	if e != nil || strings.Contains(generic.Title+generic.Body, privateSentinel) || !strings.Contains(generic.Title, "[Synthetic verification]") {
		t.Fatal(generic, e)
	}
	for _, repo := range []string{"../repo", "owner/..", "owner/repo/x", "owner/repo?x", "https://github.com/owner/repo"} {
		if _, e := ReportProjection(repo, testReportID, "bug", "", false); e == nil {
			t.Fatalf("accepted %q", repo)
		}
	}
	if _, e := ReportProjection("owner/repo", strings.Repeat("-", 36), "bug", "", false); e == nil {
		t.Fatal("accepted invalid UUID")
	}
	if _, e := ReportProjection("owner/repo", testReportID, privateSentinel, "", false); e == nil {
		t.Fatal("accepted category")
	}
	g := fixture(t, func(w http.ResponseWriter, r *http.Request) { t.Fatal("invalid projection sent") })
	for _, change := range []func(*Projection){func(p *Projection) { p.Body = privateSentinel }, func(p *Projection) { p.Title = privateSentinel }, func(p *Projection) { p.Marker = privateSentinel }, func(p *Projection) { p.repo = "other/repo" }} {
		q := p
		change(&q)
		_, e := g.CreateIssue(context.Background(), q)
		checkError(t, e, "projection", false)
	}
}
func TestGitHubFindIssue(t *testing.T) {
	p := projection(t)
	t.Run("all pages closed matches and PR exclusion", func(t *testing.T) {
		calls := 0
		g := fixture(t, func(w http.ResponseWriter, r *http.Request) {
			calls++
			if r.Method != "GET" || r.URL.Path != "/repos/owner/repo/issues" || r.URL.Query().Get("state") != "all" || r.URL.Query().Get("direction") != "asc" || r.URL.Query().Get("per_page") != "100" {
				t.Error(r.URL)
			}
			if calls == 1 {
				w.Header().Set("Link", `<https://evil.invalid/steal>; rel="next"`)
				pr := row(2, p.Marker)
				pr["pull_request"] = map[string]any{"url": "ignored"}
				emit(t, w, []any{row(1, "nothing"), pr})
			} else {
				emit(t, w, []any{row(3, p.Marker)})
			}
		})
		u, e := g.FindIssue(context.Background(), p.Marker)
		if e != nil || u != "https://github.com/owner/repo/issues/3" || calls != 2 {
			t.Fatal(u, e, calls)
		}
	})
	t.Run("multiple matches", func(t *testing.T) {
		g := fixture(t, func(w http.ResponseWriter, r *http.Request) { emit(t, w, []any{row(1, p.Marker), row(2, p.Marker)}) })
		_, e := g.FindIssue(context.Background(), p.Marker)
		checkError(t, e, "multiple_matches", false)
	})
	t.Run("full page without link", func(t *testing.T) {
		calls := 0
		g := fixture(t, func(w http.ResponseWriter, r *http.Request) {
			calls++
			rows := []any{}
			if calls == 1 {
				for i := 1; i <= 100; i++ {
					rows = append(rows, row(i, ""))
				}
			}
			emit(t, w, rows)
		})
		u, e := g.FindIssue(context.Background(), p.Marker)
		if u != "" || e != nil || calls != 2 {
			t.Fatal(u, e, calls)
		}
	})
	t.Run("bounded incomplete", func(t *testing.T) {
		calls := 0
		g := fixture(t, func(w http.ResponseWriter, r *http.Request) {
			calls++
			w.Header().Set("Link", `<ignored>; rel="next"`)
			emit(t, w, []any{})
		})
		_, e := g.FindIssue(context.Background(), p.Marker)
		checkError(t, e, "incomplete_search", false)
		if calls != 100 {
			t.Fatal(calls)
		}
	})
	for _, b := range []string{"null", "{}", "[]{}", `[{"number":1,"html_url":"https://github.com/other/repo/issues/1"}]`, `[{"number":2,"html_url":"https://github.com/owner/repo/issues/1"}]`} {
		t.Run("malformed "+b, func(t *testing.T) {
			g := fixture(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, b) })
			_, e := g.FindIssue(context.Background(), p.Marker)
			checkError(t, e, "invalid_response", false)
		})
	}
}
func TestGitHubCreateIssue(t *testing.T) {
	p := projection(t)
	t.Run("single safe authenticated POST", func(t *testing.T) {
		calls := 0
		g := fixture(t, func(w http.ResponseWriter, r *http.Request) {
			calls++
			b, e := io.ReadAll(r.Body)
			if e != nil {
				t.Error(e)
			}
			var data map[string]string
			if json.Unmarshal(b, &data) != nil || len(data) != 2 || data["title"] != p.Title || data["body"] != p.Body || strings.Contains(string(b), privateSentinel) {
				t.Error("unsafe payload")
			}
			if r.Method != "POST" || r.URL.Path != "/repos/owner/repo/issues" || r.Header.Get("Authorization") != "Bearer test-token" || r.Header.Get("X-GitHub-Api-Version") != "2026-03-10" {
				t.Error("request contract")
			}
			w.WriteHeader(201)
			emit(t, w, row(4, p.Marker))
		})
		u, e := g.CreateIssue(context.Background(), p)
		if e != nil || u != "https://github.com/owner/repo/issues/4" || calls != 1 {
			t.Fatal(u, e, calls)
		}
	})
	for _, tc := range []struct {
		status int
		kind   string
		amb    bool
	}{{401, "auth", false}, {403, "auth", false}, {422, "rejected", false}, {429, "rate_limit", false}, {500, "unavailable", true}, {503, "unavailable", true}, {200, "rejected", true}} {
		t.Run(strconv.Itoa(tc.status), func(t *testing.T) {
			calls := 0
			g := fixture(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.WriteHeader(tc.status)
				fmt.Fprint(w, privateSentinel)
			})
			_, e := g.CreateIssue(context.Background(), p)
			checkError(t, e, tc.kind, tc.amb)
			if calls != 1 {
				t.Fatal("retried create")
			}
		})
	}
	for _, body := range []string{"garbage", "null", `{"number":1,"html_url":"https://github.com/evil/repo/issues/1"}`, `{"number":1,"html_url":"https://github.com/owner/repo/issues/1"} {}`, `{"number":1,"html_url":"https://github.com/owner/repo/issues/1","pull_request":{}}`} {
		t.Run("malformed "+body, func(t *testing.T) {
			g := fixture(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(201); fmt.Fprint(w, body) })
			_, e := g.CreateIssue(context.Background(), p)
			checkError(t, e, "invalid_response", true)
		})
	}
	t.Run("cancellation after send", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		calls := 0
		release := make(chan struct{})
		g := fixture(t, func(w http.ResponseWriter, r *http.Request) { calls++; cancel(); <-release })
		defer close(release)
		_, e := g.CreateIssue(ctx, p)
		checkError(t, e, "unavailable", true)
		if calls != 1 {
			t.Fatal(calls)
		}
	})
	t.Run("redirect credentials never forwarded", func(t *testing.T) {
		hits := 0
		target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits++ }))
		defer target.Close()
		g := fixture(t, func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 307) })
		_, e := g.CreateIssue(context.Background(), p)
		checkError(t, e, "rejected", true)
		if hits != 0 {
			t.Fatal("followed redirect")
		}
	})
}
func TestGitHubCredential(t *testing.T) {
	t.Run("file rotation and revoked credential", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "token")
		write := func(s string) {
			t.Helper()
			if e := os.WriteFile(path, []byte(s), 0600); e != nil {
				t.Fatal(e)
			}
		}
		write("first")
		expected := "first"
		calls := 0
		g := fixture(t, func(w http.ResponseWriter, r *http.Request) {
			calls++
			if r.Header.Get("Authorization") != "Bearer "+expected {
				t.Error("token not reloaded")
			}
			emit(t, w, []any{})
		})
		g.config.TokenEnv = ""
		g.config.TokenFile = path
		_, e := g.FindIssue(context.Background(), projection(t).Marker)
		if e != nil {
			t.Fatal(e)
		}
		expected = "second"
		write(expected)
		_, e = g.FindIssue(context.Background(), projection(t).Marker)
		if e != nil {
			t.Fatal(e)
		}
		write("")
		_, e = g.FindIssue(context.Background(), projection(t).Marker)
		checkError(t, e, "credential", false)
		if calls != 2 {
			t.Fatal(calls)
		}
	})
	t.Run("secondary rate limit", func(t *testing.T) {
		g := fixture(t, func(w http.ResponseWriter, r *http.Request) { w.Header().Set("Retry-After", "45"); w.WriteHeader(403) })
		_, e := g.CreateIssue(context.Background(), projection(t))
		checkError(t, e, "rate_limit", false)
		var x *GitHubError
		errors.As(e, &x)
		if x.RetryAfter != 45*time.Second {
			t.Fatal(x)
		}
	})
	for _, c := range []GitHubConfig{{Repository: "owner/repo"}, {Repository: "owner/repo", TokenFile: "x", TokenEnv: "y"}, {Repository: "../repo", TokenEnv: "x"}} {
		if _, e := NewGitHub(c); e == nil {
			t.Fatal("accepted invalid config")
		}
	}
	g, e := NewGitHub(GitHubConfig{Repository: "owner/repo", TokenEnv: "AJENT_GITHUB_TEST_TOKEN"})
	if e != nil || g.base != githubAPI || g.client.Timeout != 15*time.Second {
		t.Fatal(g, e)
	}
}
