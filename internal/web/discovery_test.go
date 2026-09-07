package web

import (
	"ajent/internal/api"
	"encoding/json"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDiscoveryNegotiation(t *testing.T) {
	for _, tc := range []struct {
		accept   string
		code     int
		markdown bool
	}{
		{"", 200, false}, {"*/*", 200, false}, {"text/markdown", 200, true}, {"text/html", 200, false},
		{"text/markdown;q=0, text/html", 200, false}, {"text/markdown;q=0.9,text/html;q=0.2", 200, true},
		{"text/markdown;q=0,*/*;q=1", 200, false}, {"application/json", 406, false}, {"text/*;q=0", 406, false},
	} {
		t.Run(tc.accept, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest("GET", "/", nil)
			r.Header.Set("Accept", tc.accept)
			handled := negotiatePublic(w, r, "# Ajent")
			if w.Code != tc.code || strings.HasPrefix(w.Header().Get("Content-Type"), "text/markdown") != tc.markdown || (!handled && tc.code != 200) {
				t.Fatalf("status=%d headers=%v handled=%v", w.Code, w.Header(), handled)
			}
			if w.Header().Get("Vary") != "Accept" {
				t.Fatal("missing Vary")
			}
		})
	}
}
func TestDiscoveryRoutes(t *testing.T) {
	a := api.New(nil)
	s, err := New(a, false, "https://ajent.example", "", false)
	if err != nil {
		t.Fatal(err)
	}
	m := http.NewServeMux()
	a.Routes(m)
	s.Routes(m)
	for _, path := range []string{"/", "/docs", "/about", "/contact", "/privacy", "/llms.txt", "/robots.txt", "/sitemap.xml", "/api", "/openapi.json", "/missing", "/v1/missing", "/v1/me"} {
		t.Run(path, func(t *testing.T) {
			w := httptest.NewRecorder()
			m.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
			want := 200
			if strings.Contains(path, "missing") {
				want = 404
			}
			if path == "/v1/me" {
				want = 401
			}
			if w.Code != want {
				t.Fatalf("%d: %s", w.Code, w.Body.String())
			}
			if strings.Contains(w.Header().Get("Content-Type"), "json") {
				var v map[string]any
				if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
					t.Fatal(err)
				}
				if path == "/openapi.json" {
					if v["openapi"] != "3.0.3" || len(v["paths"].(map[string]any)) != 4 {
						t.Fatal("invalid spec")
					}
				}
			}
			if path == "/sitemap.xml" {
				var doc struct {
					URLs []struct {
						Loc string `xml:"loc"`
					} `xml:"url"`
				}
				if err := xml.Unmarshal(w.Body.Bytes(), &doc); err != nil || len(doc.URLs) != 9 {
					t.Fatalf("invalid sitemap: %v", err)
				}
			}
			if path == "/" {
				body := w.Body.String()
				for _, part := range []string{`rel="canonical" href="https://ajent.example/"`, `property="og:image" content="https://ajent.example/static/juice.png"`, `"@type":"SoftwareApplication"`, `"url":"https://ajent.example/"`} {
					if !strings.Contains(body, part) {
						t.Fatalf("missing %s", part)
					}
				}
			}
		})
	}
}

func TestAvailableDomainSchema(t *testing.T) {
	properties := responseSchema(api.AgentProfile{})["properties"].(map[string]any)
	domains, ok := properties["domains"].(map[string]any)
	if !ok || domains["type"] != "array" {
		t.Fatal("available domains must be an array", properties)
	}
	if _, bad := properties["domains,omitempty"]; bad {
		t.Fatal("JSON options leaked into schema")
	}
}
