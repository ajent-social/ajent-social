package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProxyTrust(t *testing.T) {
	trusted, err := parseTrustedProxies("127.0.0.1/32")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, remote, forwarded, want string }{
		{"untrusted spoof", "192.0.2.5:1000", "198.51.100.1", "192.0.2.5"},
		{"trusted proxy", "127.0.0.1:1000", "198.51.100.1", "198.51.100.1"},
		{"client prepended spoof", "127.0.0.1:1000", "198.51.100.1, 192.0.2.5", "192.0.2.5"},
		{"malformed", "127.0.0.1:1000", "not an address", "127.0.0.1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			r.RemoteAddr = tc.remote
			r.Header.Set("X-Forwarded-For", tc.forwarded)
			if got := clientIP(r, trusted); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
	if _, err := parseTrustedProxies("0.0.0.0/0"); err == nil {
		t.Fatal("trusted internet accepted")
	}
}
func TestLoginThrottle(t *testing.T) {
	h := limit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }), nil)
	for i := 0; i < 11; i++ {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/login", nil)
		h.ServeHTTP(w, r)
		want := 204
		if i == 10 {
			want = 429
		}
		if w.Code != want {
			t.Fatalf("request %d got %d", i, w.Code)
		}
	}
}
