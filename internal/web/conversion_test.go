package web

import (
	"ajent/internal/auth"
	"context"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestConversionJourneyStates(t *testing.T) {
	f := newHumanFixture(t)
	page := f.request("GET", "/start", nil, "")
	f.want(page, 200)
	if !strings.Contains(page.Body.String(), `data-onboarding="yes"`) || page.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("signup did not start a private onboarding journey")
	}
	f.register()
	page = f.request("GET", "/start", nil, "")
	f.want(page, 303)
	if page.Header().Get("Location") != "/join" {
		t.Fatal("existing human account did not continue to agent setup")
	}
	if page.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("account-dependent redirect must not be cached")
	}
	page = f.request("GET", "/join", nil, "")
	f.want(page, 200)
	if !strings.Contains(page.Body.String(), "Signed in as Human Owner") || !strings.Contains(page.Body.String(), "join-prompt") {
		t.Fatal("setup lost the human account context")
	}
	page = f.request("GET", "/human/workspaces", nil, "")
	f.want(page, 200)
	if strings.Contains(page.Body.String(), "first-finding-prompt") {
		t.Fatal("unconnected account was marked ready")
	}
	key, _, _, err := auth.Generate()
	if err != nil {
		t.Fatal(err)
	}
	page = f.request("POST", "/v1/installations/link", map[string]string{"handle": "conversion-" + uuid.NewString()[:8], "project_scope": "project:" + strings.Repeat("c", 64)}, key)
	f.want(page, 200)
	proof := strings.Split(jsonMap(t, page)["url"].(string), "#")[1]
	page = f.request("POST", "/human/link/preview", map[string]string{"Token": proof}, "")
	f.want(page, 200)
	csrf := jsonMap(t, page)["csrf"].(string)
	page = f.request("POST", "/human/link/approve", url.Values{"token": {proof}, "csrf": {csrf}, "scope": {"user"}, "confirm": {"yes"}, "name": {"Conversion test"}}, "")
	f.want(page, 303)
	page = f.request("GET", "/human/workspaces", nil, "")
	f.want(page, 200)
	if !strings.Contains(page.Body.String(), "first-finding-prompt") || !strings.Contains(page.Body.String(), "2. Installation approved") {
		t.Fatal("approved installation did not offer its first finding workflow")
	}
	// Expired credentials must not keep presenting the installation as usable.
	if _, err := f.s.API.DB.Pool.Exec(context.Background(), `UPDATE credentials SET expires_at=now()-interval '1 second' WHERE id IN (SELECT credential_id FROM human_installations WHERE human_id=(SELECT human_id FROM human_sessions WHERE digest=$1))`, auth.Digest(f.cookies["ajent_human"].Value)); err != nil {
		t.Fatal(err)
	}
	page = f.request("GET", "/human/workspaces", nil, "")
	f.want(page, 200)
	if strings.Contains(page.Body.String(), "first-finding-prompt") {
		t.Fatal("expired installation still presented as ready")
	}
}
