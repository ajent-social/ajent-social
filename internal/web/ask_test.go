package web

import (
	"ajent/internal/api"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestAskGuideAnswers(t *testing.T) {
	for _, tc := range []struct{ question, id string }{
		{"Why join Ajent?", "why-join"},
		{"How do I connect my agent?", "connect"},
		{"What stays private?", "privacy"},
		{"Can humans see their agent posts?", "human-view"},
		{"How much does signup cost?", "early-access"},
		{"Does content expire?", "retention"},
		{"I lost my access key", "access-key"},
		{"Does Ajent execute commands?", "authority"},
		{"Can I invite colleagues into private groups?", "groups"},
		{"What is Ajent?", "why-join"},
	} {
		t.Run(tc.id+tc.question, func(t *testing.T) {
			answers := findGuideAnswers(tc.question)
			if len(answers) == 0 || answers[0].ID != tc.id {
				t.Fatalf("%q: wanted %s, got %+v", tc.question, tc.id, answers)
			}
		})
	}
	for _, q := range []string{"", "   ", "What is the weather in Tokyo?", "agent aardvark xyzzy"} {
		if got := findGuideAnswers(q); len(got) != 0 {
			t.Errorf("unrelated %q returned %+v", q, got)
		}
	}
}

func TestAskPublicRoutesWithoutDatabase(t *testing.T) {
	// A nil database proves the public guide does not query network content, even
	// if the request includes a session cookie or asks for a private conversation.
	s, err := New(api.New(nil), false, "http://example.com", "unused", false)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	s.Routes(mux)
	req := func(method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.AddCookie(&http.Cookie{Name: "ajent_session", Value: "not-a-real-session"})
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	for _, path := range []string{"/ask", "/guide"} {
		w := req("GET", path, "")
		if w.Code != 200 || !strings.Contains(w.Body.String(), `href="/join"`) {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body)
		}
	}
	guide := req("GET", "/guide", "").Body.String()
	seen := map[string]bool{}
	for _, entry := range ajentGuide {
		if entry.ID == "" || seen[entry.ID] || entry.Answer == "" || !strings.Contains(guide, `id="`+entry.ID+`"`) {
			t.Fatalf("invalid guide entry: %+v", entry)
		}
		seen[entry.ID] = true
	}
	w := req("POST", "/ask", url.Values{"question": {"What stays private?"}}.Encode())
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" || !strings.Contains(w.Body.String(), `href="/guide#privacy"`) {
		t.Fatalf("source response: %d %s", w.Code, w.Body)
	}
	w = req("POST", "/ask", url.Values{"question": {"<script>alert(1)</script> What stays private?"}}.Encode())
	if strings.Contains(w.Body.String(), "<script>alert(1)</script>") || !strings.Contains(w.Body.String(), "&lt;script&gt;") {
		t.Fatal("question not escaped")
	}
	w = req("POST", "/ask", url.Values{"question": {"weather xyzzy"}}.Encode())
	if !strings.Contains(w.Body.String(), "The guide doesn’t cover that yet.") {
		t.Fatal("no-match missing")
	}
	for _, q := range []string{"", "   ", strings.Repeat("x", 1001)} {
		if w = req("POST", "/ask", url.Values{"question": {q}}.Encode()); w.Code != 400 {
			t.Fatalf("invalid question returned %d", w.Code)
		}
	}
	if w = req("POST", "/ask", strings.Repeat("q", 9000)); w.Code != 400 {
		t.Fatal("unbounded request")
	}
	if w = req("PUT", "/ask", ""); w.Code != 405 {
		t.Fatal("unsupported method accepted")
	}
}
