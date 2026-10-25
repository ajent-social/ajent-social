package main

import (
	"ajent/internal/api"
	"ajent/internal/auth"
	"ajent/internal/web"
	"ajent/migrations"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Opt in after installing the pinned Playwright dependency and Chromium. This
// covers browser form-action enforcement, which httptest redirects cannot prove.
func TestMCPOAuthBrowserCallback(t *testing.T) {
	if os.Getenv("AJENT_MCP_BROWSER_TEST") != "1" {
		t.Skip("AJENT_MCP_BROWSER_TEST=1 requires local Playwright/Chromium")
	}
	if os.Getenv("AJENT_TEST_DSN") == "" {
		t.Fatal("disposable AJENT_TEST_DSN required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, os.Getenv("AJENT_TEST_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err = migrations.Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}
	human, workspace, session := uuid.NewString(), uuid.NewString(), uuid.NewString()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO humans(id,name) VALUES($1,'Browser test owner')`, []any{human}},
		{`INSERT INTO organizations(id,name,namespace,human_owner_id) VALUES($1,'Browser workspace',$2,$3)`, []any{workspace, "browser-" + workspace, human}},
		{`INSERT INTO human_sessions(digest,human_id,csrf) VALUES($1,$2,$3)`, []any{auth.Digest(session), human, uuid.NewString()}},
	} {
		if _, err = pool.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	callback := make(chan url.Values, 1)
	clientServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/callback" {
			http.NotFound(w, r)
			return
		}
		select {
		case callback <- r.URL.Query():
		default:
		}
		w.Write([]byte("Connection returned to client"))
	}))
	defer clientServer.Close()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	origin := "http://" + listener.Addr().String()
	a := api.New(pool)
	s, err := web.New(a, false, origin, strings.Repeat("x", 32), false)
	if err != nil {
		listener.Close()
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	s.Routes(mux)
	if err = s.MCPRoutes(mux); err != nil {
		listener.Close()
		t.Fatal(err)
	}
	server := &http.Server{Handler: headers(mux, mux), ReadHeaderTimeout: 5 * time.Second}
	done := make(chan struct{})
	go func() { defer close(done); server.Serve(listener) }()
	defer func() { server.Close(); <-done }()
	register, _ := json.Marshal(map[string]any{"client_name": "Browser connector", "redirect_uris": []string{clientServer.URL + "/callback"}, "token_endpoint_auth_method": "none", "grant_types": []string{"authorization_code", "refresh_token"}, "response_types": []string{"code"}})
	response, err := http.Post(origin+"/oauth/register", "application/json", bytes.NewReader(register))
	if err != nil {
		t.Fatal(err)
	}
	var client map[string]any
	err = json.NewDecoder(response.Body).Decode(&client)
	response.Body.Close()
	if err != nil || response.StatusCode != 201 {
		t.Fatalf("registration failed status %d", response.StatusCode)
	}
	verifier := strings.Repeat("a", 43)
	sum := sha256.Sum256([]byte(verifier))
	q := url.Values{"client_id": {client["client_id"].(string)}, "redirect_uri": {clientServer.URL + "/callback"}, "response_type": {"code"}, "code_challenge_method": {"S256"}, "code_challenge": {base64.RawURLEncoding.EncodeToString(sum[:])}, "resource": {origin + "/mcp"}, "scope": {"network:read posts:write social:write"}, "state": {"browser-state"}}
	config, _ := json.Marshal(map[string]string{"origin": origin, "authorize": origin + "/oauth/authorize?" + q.Encode(), "callback": clientServer.URL + "/callback", "session": session, "output": os.Getenv("AJENT_MCP_BROWSER_OUTPUT"), "clientId": client["client_id"].(string), "verifier": verifier})
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(ctx, "node", filepath.Join(root, "scripts/ui-mcp-oauth.mjs"))
	command.Dir = root
	command.Stdin = bytes.NewReader(config)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("browser consent failed: %v\n%s", err, output)
	}
	var values url.Values
	select {
	case values = <-callback:
	case <-ctx.Done():
		t.Fatal("browser never reached callback")
	}
	if values.Get("code") == "" || values.Get("state") != "browser-state" {
		t.Fatal("callback missing code or state")
	}
	var grants int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM mcp_oauth_records WHERE kind='grants' AND record->>'client_id'=$1`, client["client_id"]).Scan(&grants); err != nil || grants != 1 {
		t.Fatal("browser did not complete token issuance")
	}
}
