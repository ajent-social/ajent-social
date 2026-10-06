package web

import (
	"ajent/internal/api"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func onboardingTestKey(t *testing.T) string {
	t.Helper()
	key := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize)).Public()
	der, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
}

func onboardingMux(t *testing.T) http.Handler {
	t.Helper()
	return onboardingMuxWithKey(t, onboardingTestKey(t))
}

func onboardingMuxWithKey(t *testing.T, key string) http.Handler {
	t.Helper()
	t.Setenv("AJENT_RELEASE_PUBLIC_KEY", key)
	base, version := "", ""
	if key != "" {
		base, version = "https://releases.example/ajent", "1.2.3"
	}
	t.Setenv("AJENT_RELEASE_DOWNLOAD_BASE", base)
	t.Setenv("AJENT_RELEASE_VERSION", version)
	s, err := New(api.New(nil), true, "https://join.example", "unused", false)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	s.Routes(mux)
	return mux
}

func TestPythonOnboardingAssetsAreRetired(t *testing.T) {
	mux := onboardingMux(t)
	for _, path := range []string{"/join.py", "/integration.json"} {
		t.Run(path, func(t *testing.T) {
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
			if response.Code != http.StatusGone {
				t.Fatalf("GET %s = %d, want 410 Gone", path, response.Code)
			}
			if strings.Contains(response.Body.String(), "def ") || strings.Contains(response.Body.String(), "import ") || strings.Contains(response.Body.String(), "ajent_mcp") {
				t.Fatalf("GET %s returned client payload", path)
			}
		})
	}
}

func TestGoOnboardingAssetsAreServed(t *testing.T) {
	mux := onboardingMux(t)
	assets := map[string]string{
		"/install.sh":     "verify_signature",
		"/install.ps1":    "ajentInstallVerifySignature",
		"/skill.md":       "ajent setup",
		"/agent-setup.md": "ajent setup",
	}
	for path, marker := range assets {
		t.Run(path, func(t *testing.T) {
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
			if response.Code != http.StatusOK {
				t.Fatalf("GET %s = %d, want 200: %s", path, response.Code, response.Body.String())
			}
			if !strings.Contains(response.Body.String(), marker) {
				t.Fatalf("GET %s lacks expected Go installer/setup marker %q", path, marker)
			}
			if strings.Contains(response.Body.String(), "__ORIGIN__") {
				t.Fatalf("GET %s contains an unresolved origin", path)
			}
		})
	}
}

func TestBundledPluginUsesGoSessionStartHook(t *testing.T) {
	hooks, err := onboarding.ReadFile("onboarding/plugins/ajent/hooks/hooks.json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(hooks), `"command": "ajent hook session-start"`) || strings.Contains(string(hooks), "python3") {
		t.Fatalf("plugin does not use the Go session-start hook: %s", hooks)
	}
}

func TestJoinUsesGoSetupPromptAndPlatformInstallers(t *testing.T) {
	response := httptest.NewRecorder()
	onboardingMux(t).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/join", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("GET /join = %d", response.Code)
	}
	for _, expected := range []string{"ajent setup", "https://join.example/skill.md", "/install.sh", "/install.ps1"} {
		if !strings.Contains(response.Body.String(), expected) {
			t.Errorf("join page missing %q", expected)
		}
	}
	if strings.Contains(response.Body.String(), "client.py") || strings.Contains(response.Body.String(), "join.py") {
		t.Fatal("join page still describes the Python client")
	}
}

func TestUnconfiguredInstallersFailClosed(t *testing.T) {
	mux := onboardingMuxWithKey(t, "")
	for _, path := range []string{"/install.sh", "/install.ps1"} {
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))
		if rr.Code != http.StatusServiceUnavailable || rr.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("%s status=%d cache=%q", path, rr.Code, rr.Header().Get("Cache-Control"))
		}
		if strings.Contains(rr.Body.String(), "verify_signature") || strings.Contains(rr.Body.String(), "Install-Ajent") {
			t.Fatalf("%s served executable installer", path)
		}
	}
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/join", nil))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "New installations are not available yet") || !strings.Contains(rr.Body.String(), "ajent setup") {
		t.Fatalf("join does not explain safe existing-client path: %s", rr.Body.String())
	}
	for _, unsafe := range []string{"curl -fsSL", "join-shell-command", "join-powershell-command"} {
		if strings.Contains(rr.Body.String(), unsafe) {
			t.Errorf("unconfigured join advertises %q", unsafe)
		}
	}
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/skill.md", nil))
	if !strings.Contains(rr.Body.String(), "Otherwise stop; do not download installers") {
		t.Fatal("agent instructions do not preserve release gate")
	}
}

func TestConfiguredInstallersContainOnlyValidatedPublicKey(t *testing.T) {
	key := onboardingTestKey(t)
	mux := onboardingMuxWithKey(t, key)
	for _, path := range []string{"/install.sh", "/install.ps1"} {
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))
		if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), strings.TrimSpace(key)) || strings.Contains(rr.Body.String(), "__AJENT_RELEASE_PUBLIC_KEY__") {
			t.Fatalf("%s did not render trusted key", path)
		}
	}
	for _, bad := range []string{"garbage", key + "$(touch injected)", key + key, strings.Replace(key, "PUBLIC KEY", "PRIVATE KEY", -1)} {
		t.Setenv("AJENT_RELEASE_PUBLIC_KEY", bad)
		if _, err := New(api.New(nil), true, "https://join.example", "unused", false); err == nil {
			t.Fatal("invalid or injected key accepted")
		}
	}
}

func TestInstallerReleaseActivationRequiresPinnedPublicDistribution(t *testing.T) {
	key := onboardingTestKey(t)
	for _, tc := range []struct{ base, version string }{
		{"", "1.2.3"}, {"https://releases.example", ""}, {"http://releases.example", "1.2.3"}, {"https://user:secret@releases.example", "1.2.3"}, {"https://releases.example/x';evil", "1.2.3"}, {"https://releases.example", "latest"},
	} {
		t.Setenv("AJENT_RELEASE_PUBLIC_KEY", key)
		t.Setenv("AJENT_RELEASE_DOWNLOAD_BASE", tc.base)
		t.Setenv("AJENT_RELEASE_VERSION", tc.version)
		if _, err := New(api.New(nil), true, "https://join.example", "unused", false); err == nil {
			t.Errorf("accepted base=%q version=%q", tc.base, tc.version)
		}
	}
}
