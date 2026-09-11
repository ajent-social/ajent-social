package main

import (
	"ajent/internal/telemetry"
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequestTelemetry(t *testing.T) {
	for _, tc := range []struct {
		name, method, path, route string
		status                    int
	}{
		{"clone preserves attribution", "GET", "/v1/posts/private-id?q=private-query", "GET /v1/posts/{id}", 200},
		{"empty response is 200", "GET", "/empty", "GET /empty", 200},
		{"unknown path redacted", "GET", "/private-secret", "unmatched", 404},
		{"method not allowed", "POST", "/empty", "unmatched", 405},
		{"panic is recorded", "GET", "/panic", "GET /panic", 500},
		{"panic after write remains visible", "GET", "/partial", "GET /partial", 200},
		{"health separated", "GET", "/healthz", "GET /healthz", 200},
		{"redirect path redacted", "CONNECT", "/tree/private-secret", "unmatched", 307},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var logs bytes.Buffer
			old := slog.Default()
			slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
			defer slog.SetDefault(old)
			m := http.NewServeMux()
			m.HandleFunc("CONNECT /tree/{id}/", func(w http.ResponseWriter, r *http.Request) {})
			m.HandleFunc("GET /v1/posts/{id}", func(w http.ResponseWriter, r *http.Request) {
				n := 0
				telemetry.Record(w, telemetry.Outcome{ResultCount: &n})
				w.Write([]byte("private-body"))
			})
			m.HandleFunc("GET /empty", func(w http.ResponseWriter, r *http.Request) {})
			m.HandleFunc("GET /panic", func(w http.ResponseWriter, r *http.Request) { panic("private-panic") })
			m.HandleFunc("GET /partial", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("partial")); panic("private-panic") })
			m.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })
			r := httptest.NewRequest(tc.method, tc.path, strings.NewReader("private-request"))
			r.Header.Set("Authorization", "Bearer private-token")
			r.Header.Set("X-Ajent-Client", "private-header")
			w := httptest.NewRecorder()
			headers(limit(m, nil), m).ServeHTTP(w, r)
			if w.Code != tc.status {
				t.Fatalf("status %d", w.Code)
			}
			var entry map[string]any
			if err := json.Unmarshal(bytes.TrimSpace(logs.Bytes()), &entry); err != nil {
				t.Fatalf("expected one JSON record: %v", err)
			}
			if entry["response_bytes"] != float64(w.Body.Len()) {
				t.Fatalf("body byte count missing or wrong: got %v want %d", entry["response_bytes"], w.Body.Len())
			}
			if entry["route"] != tc.route || entry["status"] != float64(tc.status) {
				t.Fatalf("wrong attribution: %v", entry)
			}
			if entry["request_id"] != w.Header().Get("X-Request-ID") || entry["client"] != "unknown" {
				t.Fatalf("invalid correlation: %v", entry)
			}
			if strings.Contains(logs.String(), "private-") {
				t.Fatal("private input leaked")
			}
			if strings.Contains(tc.name, "panic") && entry["error_code"] != "panic" {
				t.Fatal("panic hidden")
			}
			if tc.path == "/healthz" && entry["traffic"] != "health" {
				t.Fatal("health counted as application")
			}
			if tc.route == "GET /v1/posts/{id}" && entry["result_count"] != float64(0) {
				t.Fatal("empty outcome missing")
			}
		})
	}
}

func TestThrottledRouteTelemetry(t *testing.T) {
	var logs bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	defer slog.SetDefault(old)
	m := http.NewServeMux()
	m.HandleFunc("POST /login", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	h := headers(limit(m, nil), m)
	for i := 0; i < 11; i++ {
		logs.Reset()
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/login", nil))
	}
	var entry map[string]any
	if err := json.Unmarshal(logs.Bytes(), &entry); err != nil {
		t.Fatal(err)
	}
	if entry["route"] != "POST /login" || entry["status"] != float64(429) || entry["error_code"] != "http_429" {
		t.Fatalf("throttle lost: %v", entry)
	}
}

// A failed downstream write can accept only a prefix. Count accepted bytes,
// not the requested length, and preserve the caller's write error.
type partialTelemetryWriter struct{ *httptest.ResponseRecorder }

func (w partialTelemetryWriter) Write(p []byte) (int, error) {
	n, _ := w.ResponseRecorder.Write(p[:2])
	return n, io.ErrShortWrite
}
func TestResponseBytesPartialWrite(t *testing.T) {
	underlying := httptest.NewRecorder()
	recorder := &responseStatus{ResponseWriter: partialTelemetryWriter{underlying}}
	n, err := recorder.Write([]byte("é🔍"))
	if n != 2 || err != io.ErrShortWrite || recorder.bytesWritten != 2 || underlying.Body.Len() != 2 {
		t.Fatalf("partial write changed or miscounted: n=%d err=%v bytes=%d", n, err, recorder.bytesWritten)
	}
}
