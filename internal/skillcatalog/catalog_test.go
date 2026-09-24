// SPDX-License-Identifier: Apache-2.0

package skillcatalog_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ajent/internal/skillcatalog"
)

func fixture(t *testing.T) *skillcatalog.Catalog {
	t.Helper()
	c, err := skillcatalog.New(strings.NewReader(skillcatalog.SeedCatalog), "https://skills.ajent.social", skillcatalog.Bootstrap)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func request(c http.Handler, method, path string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, nil)
	w := httptest.NewRecorder()
	c.ServeHTTP(w, r)
	return w
}

func TestDiscoveryFetchPinnedContent(t *testing.T) {
	t.Parallel()
	c := fixture(t)
	w := request(c, "GET", "/v1/skills/search?q=handoff&limit=1")
	if w.Code != 200 {
		t.Fatalf("search returned %d", w.Code)
	}
	var result struct {
		Schema         string                `json:"schema"`
		CatalogVersion string                `json:"catalog_version"`
		Results        []skillcatalog.Result `json:"results"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Schema != "ajent.skills.search/v1" || len(result.Results) != 1 || len(result.CatalogVersion) != 64 {
		t.Fatalf("invalid search envelope: %+v", result)
	}
	skill := result.Results[0]
	if skill.ID != "mattpocock-handoff" {
		t.Fatalf("wrong candidate %s", skill.ID)
	}
	fetched := request(c, "GET", skill.ContentURL)
	if fetched.Code != 200 || !strings.HasPrefix(fetched.Body.String(), "---\nname: handoff") {
		t.Fatal("could not fetch selected instructions")
	}
	hash := sha256.Sum256(fetched.Body.Bytes())
	if hex.EncodeToString(hash[:]) != skill.ContentSHA256 || skill.Version != skill.ContentSHA256 {
		t.Fatal("retrieved bytes do not match pinned version")
	}
	if !strings.Contains(fetched.Header().Get("Cache-Control"), "immutable") {
		t.Fatal("missing immutable cache directive")
	}
	notice := request(c, "GET", skill.LicenseURL)
	if notice.Code != 200 || !strings.Contains(notice.Body.String(), "Copyright (c) 2026 Matt Pocock") {
		t.Fatal("missing source license notice")
	}
	req := httptest.NewRequest("GET", skill.ContentURL, nil)
	req.Header.Set("If-None-Match", fetched.Header().Get("ETag"))
	cached := httptest.NewRecorder()
	c.ServeHTTP(cached, req)
	if cached.Code != 304 || cached.Body.Len() != 0 {
		t.Fatal("conditional fetch did not return 304")
	}
	wrong := strings.Replace(skill.ContentURL, skill.Version, strings.Repeat("0", 64), 1)
	if request(c, "GET", wrong).Code != 404 {
		t.Fatal("unknown version must not fall back to current content")
	}
}

func TestPublicHTTPBoundaries(t *testing.T) {
	t.Parallel()
	c := fixture(t)
	for _, test := range []struct {
		name, method, path string
		status             int
	}{
		{"bootstrap", "GET", "/", 200},
		{"bootstrap-alias", "GET", "/SKILL.md", 200},
		{"head", "HEAD", "/", 200},
		{"health", "GET", "/healthz", 200},
		{"no-writes", "POST", "/v1/skills/search?q=test", 405},
		{"empty", "GET", "/v1/skills/search", 400},
		{"empty-limit", "GET", "/v1/skills/search?q=test&limit=", 400},
		{"negative-limit", "GET", "/v1/skills/search?q=test&limit=-1", 400},
		{"large-limit", "GET", "/v1/skills/search?q=test&limit=11", 400},
		{"duplicate-query", "GET", "/v1/skills/search?q=test&q=another", 400},
		{"unknown-field", "GET", "/v1/skills/search?q=test&token=secret", 400},
		{"oversized-query", "GET", "/v1/skills/search?q=" + strings.Repeat("a", 2049), 400},
		{"invalid-utf8", "GET", "/v1/skills/search?q=%FF", 400},
		{"punctuation", "GET", "/v1/skills/search?q=!!!", 400},
		{"empty-match", "GET", "/v1/skills/search?q=zznonexistent", 200},
		{"traversal", "GET", "/v1/skills/../../LICENSE", 404},
		{"no-filesystem", "GET", "/v1/skills/%2e%2e/%2e%2e/etc/passwd", 404},
	} {
		t.Run(test.name, func(t *testing.T) {
			w := request(c, test.method, test.path)
			if w.Code != test.status {
				t.Fatalf("got %d want %d: %s", w.Code, test.status, w.Body.String())
			}
			if test.status >= 400 && strings.Contains(w.Body.String(), "secret") {
				t.Fatal("error echoed query content")
			}
			if test.method == "HEAD" && w.Body.Len() != 0 {
				t.Fatal("HEAD sent a body")
			}
			if test.name == "empty-match" && !strings.Contains(w.Body.String(), `"results":[]`) {
				t.Fatal("no-match is not an explicit empty result")
			}
		})
	}
}

func TestCatalogValidation(t *testing.T) {
	t.Parallel()
	var original map[string]any
	if err := json.Unmarshal([]byte(skillcatalog.SeedCatalog), &original); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"unreviewed", func(m map[string]any) { m["skills"].([]any)[0].(map[string]any)["redistribution_reviewed"] = false }},
		{"bad-id", func(m map[string]any) { m["skills"].([]any)[0].(map[string]any)["id"] = "../escape" }},
		{"missing-notice", func(m map[string]any) { m["skills"].([]any)[0].(map[string]any)["license_text"] = "" }},
		{"unsupported-license", func(m map[string]any) { m["skills"].([]any)[0].(map[string]any)["license"] = "unknown" }},
		{"invalid-content", func(m map[string]any) { m["skills"].([]any)[0].(map[string]any)["content"] = "not a skill" }},
		{"duplicate", func(m map[string]any) { m["skills"] = append(m["skills"].([]any), m["skills"].([]any)[0]) }},
		{"unknown-field", func(m map[string]any) { m["unexpected"] = true }},
	} {
		t.Run(test.name, func(t *testing.T) {
			var m map[string]any
			if err := json.Unmarshal([]byte(skillcatalog.SeedCatalog), &m); err != nil {
				t.Fatal(err)
			}
			test.mutate(m)
			data, err := json.Marshal(m)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = skillcatalog.New(strings.NewReader(string(data)), "https://skills.ajent.social", skillcatalog.Bootstrap); err == nil {
				t.Fatal("invalid manifest accepted")
			}
		})
	}
	for _, origin := range []string{"http://example.com", "https://user:password@example.com", "https://example.com/path", "https://example.com?token=secret"} {
		if _, err := skillcatalog.New(strings.NewReader(skillcatalog.SeedCatalog), origin, skillcatalog.Bootstrap); err == nil {
			t.Fatalf("accepted invalid origin")
		}
	}
}

func TestBootstrapConfiguredOrigin(t *testing.T) {
	t.Parallel()
	c, err := skillcatalog.New(strings.NewReader(skillcatalog.SeedCatalog), "http://127.0.0.1:8091", skillcatalog.Bootstrap)
	if err != nil {
		t.Fatal(err)
	}
	w := request(c, "GET", "/")
	if strings.Contains(w.Body.String(), "{{ORIGIN}}") || !strings.Contains(w.Body.String(), "http://127.0.0.1:8091/v1/skills/search") {
		t.Fatal("bootstrap does not use configured origin")
	}
}
