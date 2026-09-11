package web

import (
	"ajent/internal/api"
	"ajent/migrations"
	"context"
	"encoding/json"
	"encoding/pem"
	"github.com/jackc/pgx/v5/pgxpool"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestOnboardingDiscovery(t *testing.T) {
	s, e := New(api.New(nil), true, "https://join.example", "unused", false)
	if e != nil {
		t.Fatal(e)
	}
	m := http.NewServeMux()
	s.Routes(m)
	for _, path := range []string{"/join", "/install.sh", "/join.py", "/skill.md"} {
		w := httptest.NewRecorder()
		m.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 || !strings.Contains(w.Body.String(), "https://join.example") || strings.Contains(w.Body.String(), "__ORIGIN__") {
			t.Fatalf("unusable onboarding %s", path)
		}
	}
}

func TestInstallerHTTPSRoundTrip(t *testing.T) {
	dsn := os.Getenv("AJENT_TEST_DSN")
	if dsn == "" {
		t.Skip("AJENT_TEST_DSN required")
	}
	ctx := context.Background()
	pool, e := pgxpool.New(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer pool.Close()
	if e = migrations.Apply(ctx, pool); e != nil {
		t.Fatal(e)
	}
	if _, e = pool.Exec(ctx, `DELETE FROM signup_buckets`); e != nil {
		t.Fatal(e)
	}
	a := api.New(pool)
	s, e := New(a, true, "https://placeholder.invalid", "test", false)
	if e != nil {
		t.Fatal(e)
	}
	m := http.NewServeMux()
	s.Routes(m)
	a.Routes(m)
	ts := httptest.NewTLSServer(m)
	defer ts.Close()
	s.Origin = ts.URL
	dir := t.TempDir()
	ca := filepath.Join(dir, "ca.pem")
	if e = os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ts.Certificate().Raw}), 0600); e != nil {
		t.Fatal(e)
	}
	response, e := ts.Client().Get(ts.URL + "/install.sh")
	if e != nil {
		t.Fatal(e)
	}
	b, e := io.ReadAll(response.Body)
	response.Body.Close()
	if e != nil {
		t.Fatal(e)
	}
	install := filepath.Join(dir, "install.sh")
	os.WriteFile(install, b, 0600)
	config := filepath.Join(dir, "config")
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Env = append(os.Environ(), "AJENT_INTEGRATIONS=0", "AJENT_CONFIG_DIR="+config, "AJENT_CREDENTIAL_FILE="+filepath.Join(config, "credential.json"), "SSL_CERT_FILE="+ca, "CURL_CA_BUNDLE="+ca)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("client failed: %v %s", err, out)
		}
		if strings.Contains(string(out), "aj_") {
			t.Fatal("client output leaked a key")
		}
		return string(out)
	}
	run("sh", install, "--agent-only", "--scope", "user", "--private")
	credentialPath := filepath.Join(config, "credential.json")
	before, _ := os.ReadFile(credentialPath)
	run("sh", install, "--agent-only", "--scope", "user", "--private")
	after, _ := os.ReadFile(credentialPath)
	if string(before) != string(after) {
		t.Fatal("rerun replaced credentials")
	}
	info, _ := os.Stat(credentialPath)
	if info.Mode().Perm() != 0600 {
		t.Fatal("credential permissions")
	}
	client := filepath.Join(config, "client.py")
	run("python3", client, "me")
	// Exercise a real downloaded integration bundle with the shared-directory
	// arrangement used by installations that share skills across harnesses.
	home := filepath.Join(dir, "home")
	skills := filepath.Join(home, ".claude", "skills")
	if err := os.MkdirAll(skills, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".agents"), 0700); err != nil {
		t.Fatal(err)
	}
	shared := filepath.Join(home, ".agents", "skills")
	if err := os.Symlink(skills, shared); err != nil {
		t.Fatal(err)
	}
	integration := exec.Command("python3", client, "integrations")
	// Keep vendor CLIs out of PATH and configuration under the temporary home.
	integration.Env = append(os.Environ(), "HOME="+home, "XDG_CONFIG_HOME="+filepath.Join(home, ".config"), "PATH=/usr/bin:/bin", "AJENT_INTEGRATIONS=1", "AJENT_CONFIG_DIR="+config, "AJENT_CREDENTIAL_FILE="+credentialPath, "SSL_CERT_FILE="+ca)
	if output, err := integration.CombinedOutput(); err != nil {
		t.Fatalf("shared-directory integration failed: %v %s", err, output)
	}
	if _, err := os.Readlink(shared); err != nil {
		t.Fatal("installation replaced the shared skills symlink", err)
	}
	if content, err := os.ReadFile(filepath.Join(skills, "ajent-network", "SKILL.md")); err != nil || !strings.Contains(string(content), "ajent_login") {
		t.Fatal("shared skill did not install from the downloaded bundle", err)
	}
	if unchanged, err := os.ReadFile(credentialPath); err != nil || string(unchanged) != string(before) {
		t.Fatal("integration update changed the installation credential", err)
	}
	run("python3", client, "post", "--kind", "finding", "--title", "Installer verification", "--body", "Private fixture", "--key", "installer-proof")
	found := run("python3", client, "search", "Installer verification")
	if !strings.Contains(found, "Private fixture") {
		t.Fatal("client could not retrieve its finding")
	}
	mcp := func(profile string, messages string) string {
		t.Helper()
		cmd := exec.Command("python3", client, "mcp", "--profile", profile)
		cmd.Env = append(os.Environ(), "AJENT_CONFIG_DIR="+config, "AJENT_CREDENTIAL_FILE="+credentialPath, "SSL_CERT_FILE="+ca)
		cmd.Stdin = strings.NewReader(messages)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("MCP failed: %v %s", err, out)
		}
		if strings.Contains(string(out), "aj_") || strings.Contains(string(out), `"isError": true`) {
			t.Fatal("MCP error or credential leak")
		}
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if !json.Valid([]byte(line)) {
				t.Fatal("non-JSON MCP stdout")
			}
		}
		return string(out)
	}
	init := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","clientInfo":{"name":"test","version":"1"},"capabilities":{}}}` + "\n"
	listed := mcp("claude-code", init+`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`+"\n")
	if !strings.Contains(listed, "ajent_login") {
		t.Fatal("login tool missing")
	}
	mcp("claude-code", init+`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"ajent_post","arguments":{"kind":"finding","title":"MCP fleet exchange","body":"Independently retrievable evidence","key":"mcp-proof","approved":true}}}`+"\n")
	read := mcp("codex", init+`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"ajent_search","arguments":{"query":"MCP fleet exchange"}}}`+"\n")
	if !strings.Contains(read, "Independently retrievable evidence") {
		t.Fatal("second tool did not retrieve first tool finding")
	}

	// Installation choices flow through the real HTTPS API into an existing MCP profile.
	var installation map[string]string
	if err := json.Unmarshal(before, &installation); err != nil {
		t.Fatal(err)
	}
	domain := installation["agent_id"] + ".installer.test"
	if _, err := pool.Exec(ctx, `INSERT INTO verified_domains(domain,org_id,verified_until) SELECT $1,org_id,now()+interval '30 days' FROM agents WHERE id=$2`, domain, installation["agent_id"]); err != nil {
		t.Fatal(err)
	}
	run("python3", client, "configure", "--scope", "user", "--domain", domain)
	meMessage := init + `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"ajent_me","arguments":{}}}` + "\n"
	active := mcp("codex", meMessage)
	if !strings.Contains(active, domain) {
		t.Fatal("configured domain did not reach existing MCP agent", active)
	}
	if _, err := pool.Exec(ctx, `UPDATE verified_domains SET verified_until=now()-interval '1 second' WHERE domain=$1`, domain); err != nil {
		t.Fatal(err)
	}
	expired := mcp("codex", meMessage)
	if !strings.Contains(expired, "private collaboration remains connected") {
		t.Fatal("expired scope blocked private use", expired)
	}
	if _, err := pool.Exec(ctx, `UPDATE verified_domains SET verified_until=now()+interval '30 days' WHERE domain=$1`, domain); err != nil {
		t.Fatal(err)
	}
	restored := mcp("codex", meMessage)
	if !strings.Contains(restored, domain) || strings.Contains(restored, "private collaboration remains connected") {
		t.Fatal("renewed scope did not recover", restored)
	}
	run("python3", client, "configure", "--scope", "user", "--private")
	private := mcp("codex", meMessage)
	if strings.Contains(private, domain) {
		t.Fatal("private override retained domain authority", private)
	}

}

func TestIntegrationConfigPreservation(t *testing.T) {
	cmd := exec.Command("python3", "onboarding/integrations_test.py")
	cmd.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1")
	if out, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("integration config tests: %v %s", e, out)
	}
}

func TestDomainScopeClient(t *testing.T) {
	cmd := exec.Command("python3", "onboarding/scopes_test.py")
	cmd.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("scope client tests: %v %s", err, out)
	}
}
