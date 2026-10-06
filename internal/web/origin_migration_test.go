package web

import (
	"ajent/internal/api"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMigrationOriginBoundary(t *testing.T) {
	s, err := New(api.New(nil), true, "https://ajent.social", "unused", false)
	if err != nil {
		t.Fatal(err)
	}
	for _, legacy := range []string{"", "https://dogfood.ajent.social"} {
		s.LegacyOrigin = legacy
		for _, origin := range []string{"", s.Origin, "https://dogfood.ajent.social", "https://ajent.social.evil.example", "https://evil.example", "null"} {
			r := httptest.NewRequest("POST", "/login", nil)
			r.Header.Set("Origin", origin)
			want := origin == "" || origin == s.Origin || (legacy != "" && origin == legacy)
			if s.originOK(r) != want {
				t.Fatalf("legacy=%q origin=%q", legacy, origin)
			}
		}
	}
}

func TestCanonicalOnboardingDuringMigration(t *testing.T) {
	t.Setenv("AJENT_RELEASE_PUBLIC_KEY", onboardingTestKey(t))
	t.Setenv("AJENT_RELEASE_DOWNLOAD_BASE", "https://releases.example/ajent")
	t.Setenv("AJENT_RELEASE_VERSION", "1.2.3")
	s, err := New(api.New(nil), true, "https://ajent.social", "unused", false)
	if err != nil {
		t.Fatal(err)
	}
	s.LegacyOrigin = "https://dogfood.ajent.social"
	m := http.NewServeMux()
	s.Routes(m)
	for _, path := range []string{"/join", "/install.sh", "/skill.md", "/docs", "/openapi.json"} {
		w := httptest.NewRecorder()
		m.ServeHTTP(w, httptest.NewRequest("GET", "https://ajent.social"+path, nil))
		if w.Code != 200 || !strings.Contains(w.Body.String(), s.Origin) || strings.Contains(w.Body.String(), s.LegacyOrigin) {
			t.Fatalf("noncanonical response at %s: status %d", path, w.Code)
		}
	}
}
