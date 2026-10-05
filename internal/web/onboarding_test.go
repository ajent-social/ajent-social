package web

import (
	"ajent/internal/api"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func onboardingMux(t *testing.T) http.Handler {
	t.Helper()
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
