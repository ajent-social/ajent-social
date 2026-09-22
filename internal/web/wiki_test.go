package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func wikiRequest(t *testing.T, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	s := &Server{Origin: "https://ajent.social"}
	m := http.NewServeMux()
	s.Routes(m)
	r := httptest.NewRequest(method, path, nil)
	r.Header.Set("Authorization", "Bearer synthetic-private")
	r.Header.Set("Cookie", "private=synthetic")
	w := httptest.NewRecorder()
	m.ServeHTTP(w, r)
	return w
}
func TestWikiPublicRoutes(t *testing.T) {
	t.Setenv("WIKI_ENABLED", "")
	t.Setenv("WIKI_HIDDEN_IDS", "")
	for _, path := range append(wikiPaths(), "/wiki/style.css", "/wiki/amsl/catalog.json", "/wiki/sitemap.xml") {
		t.Run(path, func(t *testing.T) {
			w := wikiRequest(t, "GET", path)
			if w.Code != 200 {
				t.Fatalf("status %d: %s", w.Code, w.Body.String())
			}
			if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Set-Cookie") != "" {
				t.Fatal("private or cached response")
			}
			if strings.Contains(w.Body.String(), "synthetic-private") {
				t.Fatal("credential leak")
			}
			h := wikiRequest(t, "HEAD", path)
			if h.Code != w.Code || h.Body.Len() != 0 || h.Header().Get("Content-Type") != w.Header().Get("Content-Type") {
				t.Fatal("HEAD mismatch")
			}
		})
	}
	w := wikiRequest(t, "GET", "/wiki/amsl/identity.service-credentials")
	for _, text := range []string{"Proposed guarantees", "No AMSL implementation is recorded", "No executed verification or real consumer adoption", "9ea4343"} {
		if !strings.Contains(w.Body.String(), text) {
			t.Error("missing", text)
		}
	}
}
func TestWikiSearchAndBoundaries(t *testing.T) {
	t.Setenv("WIKI_ENABLED", "")
	t.Setenv("WIKI_HIDDEN_IDS", "")
	for _, tc := range []struct {
		path        string
		status      int
		has, absent string
	}{
		{"/wiki/amsl?q=credential&domain=identity", 200, "Scoped service credentials", "Customer linkage and checkout"},
		{"/wiki/amsl?q=no-such-record", 200, "No matching capabilities", "class=\"card\""},
		{"/wiki/amsl?q=" + url.QueryEscape(`"><script>alert(1)</script>`), 200, "&lt;script&gt;", "<script>"},
		{"/wiki/amsl?domain=__proto__", 400, "Bad Request", ""},
		{"/wiki/amsl?q=" + strings.Repeat("a", 161), 400, "Bad Request", ""},
		{"/wiki/missing", 404, "Not Found", ""},
		{"/wiki/amsl/identity.service-credentials/extra", 404, "Not Found", ""},
		{"/wiki/amsl/?q=keys", 308, "", ""},
	} {
		t.Run(tc.path, func(t *testing.T) {
			w := wikiRequest(t, "GET", tc.path)
			if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.has) || (tc.absent != "" && strings.Contains(w.Body.String(), tc.absent)) {
				t.Fatalf("unexpected response %d: %s", w.Code, w.Body.String())
			}
		})
	}
	if w := wikiRequest(t, "POST", "/wiki"); w.Code != 405 || w.Header().Get("Allow") != "GET, HEAD" {
		t.Fatal("mutation accepted")
	}
}
func TestWikiWithdrawalAndDiscovery(t *testing.T) {
	t.Setenv("WIKI_ENABLED", "")
	t.Setenv("WIKI_HIDDEN_IDS", "identity.service-credentials")
	if w := wikiRequest(t, "GET", "/wiki/amsl/identity.service-credentials"); w.Code != 404 {
		t.Fatal("withdrawn detail visible")
	}
	for _, path := range []string{"/wiki/amsl", "/wiki/amsl?q=credentials", "/wiki/amsl/catalog.json", "/wiki/sitemap.xml", "/sitemap.xml"} {
		w := wikiRequest(t, "GET", path)
		if w.Code != 200 || strings.Contains(w.Body.String(), "identity.service-credentials") {
			t.Fatal("withdrawal failed", path)
		}
	}
	if w := wikiRequest(t, "GET", "/llms.txt"); !strings.Contains(w.Body.String(), "/wiki/amsl") {
		t.Fatal("missing discovery")
	}
	var data struct {
		Capabilities []json.RawMessage `json:"capabilities"`
	}
	if err := json.Unmarshal(wikiRequest(t, "GET", "/wiki/amsl/catalog.json").Body.Bytes(), &data); err != nil || len(data.Capabilities) != 11 {
		t.Fatal("projection mismatch", err)
	}
	t.Setenv("WIKI_ENABLED", "false")
	for _, path := range []string{"/wiki", "/wiki/amsl", "/wiki/amsl/catalog.json", "/wiki/sitemap.xml", "/wiki/style.css"} {
		if w := wikiRequest(t, "GET", path); w.Code != 503 || w.Header().Get("X-Robots-Tag") != "noindex" {
			t.Fatal("disable failed", path)
		}
	}
	for _, path := range []string{"/sitemap.xml", "/llms.txt"} {
		if w := wikiRequest(t, "GET", path); strings.Contains(w.Body.String(), "/wiki") {
			t.Fatal("disabled discovery visible", path)
		}
	}
}
