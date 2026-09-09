package web

import (
	"ajent/internal/api"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestOpenAPICoversRegisteredAgentRoutes(t *testing.T) {
	s, err := New(api.New(nil), false, "https://ajent.example", "", false)
	if err != nil {
		t.Fatal(err)
	}
	paths := s.openAPIDocument()["paths"].(map[string]any)
	found := map[string]bool{}
	for _, file := range []string{"../api/api.go", "web.go", "human_dashboard.go"} {
		f, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) < 1 {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != "HandleFunc" {
				return true
			}
			lit, ok := call.Args[0].(*ast.BasicLit)
			if !ok {
				return true
			}
			route, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			parts := strings.SplitN(route, " ", 2)
			if len(parts) != 2 || !strings.HasPrefix(parts[1], "/v1/") {
				return true
			}
			found[route] = true
			p, ok := paths[parts[1]].(map[string]any)
			if !ok || p[strings.ToLower(parts[0])] == nil {
				t.Errorf("undocumented route %s", route)
			}
			return true
		})
	}
	for _, op := range apiOperations {
		if op.Path != "/api" && !found[op.Method+" "+op.Path] {
			t.Errorf("documented nonexistent route %s %s", op.Method, op.Path)
		}
	}
	if len(found) != 46 {
		t.Fatalf("review changed route inventory: %d", len(found))
	}
	doc := s.integrationMarkdown()
	for _, want := range []string{"ajent_me", "ajent_post", "ajent_help_improve", "feedback policy", "/v1/improvements", "/v1/fleet", "/v1/browser-login", "Idempotency-Key", "local stdio", "Retry-After"} {
		if !strings.Contains(doc, want) {
			t.Errorf("agent reference missing %s", want)
		}
	}
	if strings.Contains(doc, "<section") || strings.Contains(doc, "<script") {
		t.Fatal("HTML leaked into markdown reference")
	}
}

func TestPublicRecoveryRepresentations(t *testing.T) {
	s, err := New(api.New(nil), false, "https://ajent.example", "", false)
	if err != nil {
		t.Fatal(err)
	}
	for _, accept := range []string{"text/html", "text/markdown"} {
		r := httptest.NewRequest("GET", "/missing", nil)
		r.Header.Set("Accept", accept)
		w := httptest.NewRecorder()
		s.publicError(w, r, 404, "Unavailable.")
		if w.Code != 404 || !strings.Contains(w.Header().Get("Content-Type"), accept) || !strings.Contains(w.Body.String(), "/join") {
			t.Fatalf("%s: %d %s", accept, w.Code, w.Body.String())
		}
		if accept == "text/html" && !strings.Contains(w.Body.String(), "<!doctype html>") {
			t.Fatal("browser received unbranded error")
		}
	}
}
