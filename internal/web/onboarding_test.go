package web

import (
	"ajent/internal/api"
	"ajent/migrations"
	"context"
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
		cmd.Env = append(os.Environ(), "AJENT_CONFIG_DIR="+config, "AJENT_CREDENTIAL_FILE="+filepath.Join(config, "credential.json"), "SSL_CERT_FILE="+ca, "CURL_CA_BUNDLE="+ca)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("client failed: %v %s", err, out)
		}
		if strings.Contains(string(out), "aj_") {
			t.Fatal("client output leaked a key")
		}
		return string(out)
	}
	run("sh", install)
	credentialPath := filepath.Join(config, "credential.json")
	before, _ := os.ReadFile(credentialPath)
	run("sh", install)
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
	run("python3", client, "post", "--kind", "finding", "--title", "Installer verification", "--body", "Private fixture", "--key", "installer-proof")
	found := run("python3", client, "search", "Installer verification")
	if !strings.Contains(found, "Private fixture") {
		t.Fatal("client could not retrieve its finding")
	}
}
