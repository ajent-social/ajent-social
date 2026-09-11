package web

import (
	"ajent/internal/api"
	"bytes"
	"strings"
	"testing"
)

func TestAnalyticsIsPublicAndOptIn(t *testing.T) {
	t.Setenv("AJENT_GA_MEASUREMENT_ID", "G-B4WXNXM8HQ")
	s, err := New(api.New(nil), true, "https://ajent.social", "setup-token", false)
	if err != nil {
		t.Fatal(err)
	}
	var public bytes.Buffer
	if err := s.T.ExecuteTemplate(&public, "layout", Page{View: "landing", Title: "Landing", Session: &Session{}}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(public.String(), `data-measurement-id="G-B4WXNXM8HQ"`) {
		t.Fatal("public page did not include the configured measurement ID")
	}
	var app bytes.Buffer
	if err := s.T.ExecuteTemplate(&app, "layout", Page{View: "feed", Title: "Feed", Session: &Session{}}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(app.String(), "analytics.js") {
		t.Fatal("private app page included analytics")
	}

	t.Setenv("AJENT_GA_MEASUREMENT_ID", "not-an-id")
	s, err = New(api.New(nil), true, "https://ajent.social", "setup-token", false)
	if err != nil {
		t.Fatal(err)
	}
	public.Reset()
	if err := s.T.ExecuteTemplate(&public, "layout", Page{View: "landing", Title: "Landing", Session: &Session{}}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(public.String(), "analytics.js") {
		t.Fatal("invalid measurement ID was rendered")
	}
}
