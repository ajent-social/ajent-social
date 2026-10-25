// Adapted with the resolver from Serenity at 29936ab2bc0c0c659683750cd1e935ad2ce226e6 (Apache-2.0).
package mcpoauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type metadataRoundTrip func(*http.Request) (*http.Response, error)

func (f metadataRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

const testMetadataID = "https://client.example/oauth.json"

func metadataBody(t *testing.T, extra map[string]any) string {
	t.Helper()
	doc := map[string]any{"client_id": testMetadataID, "client_name": "Example client", "redirect_uris": []string{"https://client.example/callback"}}
	for key, value := range extra {
		doc[key] = value
	}
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
func metadataResponse(body string) *http.Response {
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}
}
func TestCIMDMetadataValidation(t *testing.T) {
	r := NewCIMDResolver()
	for _, tc := range []struct {
		name  string
		extra map[string]any
		valid bool
	}{
		{"public default", nil, true}, {"public explicit", map[string]any{"token_endpoint_auth_method": "none", "scope": "memory:read"}, true},
		{"ID mismatch", map[string]any{"client_id": testMetadataID + "?v=2"}, false},
		{"missing name", map[string]any{"client_name": ""}, false}, {"control name", map[string]any{"client_name": "Hello\nadmin"}, false},
		{"missing redirects", map[string]any{"redirect_uris": nil}, false}, {"insecure redirect", map[string]any{"redirect_uris": []string{"http://client.example/callback"}}, false},
		{"native loopback", map[string]any{"redirect_uris": []string{"http://127.0.0.1:1234/callback"}}, true},
		{"secret auth", map[string]any{"token_endpoint_auth_method": "client_secret_post"}, false},
		{"secret material", map[string]any{"client_secret": ""}, false}, {"private keys", map[string]any{"jwks": map[string]any{}}, false},
		{"foreign declared scope", map[string]any{"scope": "openid memory:read"}, true}, {"password grant", map[string]any{"grant_types": []string{"password"}}, false},
		{"Claude additional JWT capability", map[string]any{"grant_types": []string{"authorization_code", "refresh_token", "urn:ietf:params:oauth:grant-type:jwt-bearer"}}, true},
		{"additional response capability", map[string]any{"response_types": []string{"code", "id_token"}}, true},
		{"empty grants", map[string]any{"grant_types": []string{}}, false},
		{"empty responses", map[string]any{"response_types": []string{}}, false},
		{"malformed grant", map[string]any{"grant_types": []string{"authorization_code", "bad grant"}}, false},
		{"implicit response", map[string]any{"response_types": []string{"token"}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := r.parseMetadata(testMetadataID, []byte(metadataBody(t, tc.extra)))
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v err=%v", tc.valid, err)
			}
		})
	}
	for _, body := range []string{`null`, `[]`, metadataBody(t, nil) + ` {}`, `{"client_id":"a","client_id":"b"}`} {
		if _, err := r.parseMetadata(testMetadataID, []byte(body)); err == nil {
			t.Fatalf("accepted %s", body)
		}
	}
}
func TestCIMDURLValidation(t *testing.T) {
	for _, id := range []string{"plain-id", "http://client.example/meta", "https://client.example", "https://user@client.example/meta", "https://client.example/meta#", "https://client.example/a/../meta", "https://client.example/%2e/meta", "https://client.example:65536/meta"} {
		if validClientMetadataURL(id) {
			t.Errorf("accepted %s", id)
		}
	}
	if !validClientMetadataURL(testMetadataID) {
		t.Fatal("rejected metadata URL")
	}
}
func TestCIMDCacheAndFetchBounds(t *testing.T) {
	r := NewCIMDResolver()
	now := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	r.now = func() time.Time { return now }
	calls := 0
	r.http.Transport = metadataRoundTrip(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.Header.Get("Accept") != "application/json" {
			t.Error("missing Accept")
		}
		resp := metadataResponse(metadataBody(t, nil))
		resp.Header.Set("Cache-Control", "max-age=600")
		return resp, nil
	})
	c, err := r.Resolve(context.Background(), testMetadataID)
	if err != nil {
		t.Fatal(err)
	}
	c.RedirectURIs[0] = "https://attacker.example/"
	c, err = r.Resolve(context.Background(), testMetadataID)
	if err != nil || calls != 1 || c.RedirectURIs[0] != "https://client.example/callback" {
		t.Fatalf("cache mutated: %+v %v calls=%d", c, err, calls)
	}
	now = now.Add(301 * time.Second)
	if _, err = r.Resolve(context.Background(), testMetadataID); err != nil || calls != 2 {
		t.Fatalf("expired cache: %v calls=%d", err, calls)
	}
	for _, tc := range []struct {
		name        string
		status      int
		media, body string
	}{
		{"redirect", 302, "application/json", metadataBody(t, nil)}, {"html", 200, "text/html", metadataBody(t, nil)}, {"large", 200, "application/json", strings.Repeat(" ", 65537)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := NewCIMDResolver()
			r.http.Transport = metadataRoundTrip(func(*http.Request) (*http.Response, error) {
				resp := metadataResponse(tc.body)
				resp.StatusCode = tc.status
				resp.Header.Set("Content-Type", tc.media)
				return resp, nil
			})
			if _, err := r.Resolve(context.Background(), testMetadataID); err == nil {
				t.Fatal("accepted invalid response")
			}
		})
	}
}
func TestCIMDCacheControl(t *testing.T) {
	for _, tc := range []struct {
		value, age string
		want       time.Duration
	}{{"", "", time.Minute}, {"max-age=30", "", 30 * time.Second}, {"max-age=99999", "", 5 * time.Minute}, {"no-store, max-age=30", "", 0}, {"no-cache", "", 0}, {"max-age=0", "", 0}, {"max-age=-1", "", 0}, {"max-age=invalid", "", 0}, {"max-age=30", "20", 10 * time.Second}, {"max-age=30", "31", 0}} {
		h := http.Header{}
		h.Set("Cache-Control", tc.value)
		h.Set("Age", tc.age)
		if got := metadataTTL(h); got != tc.want {
			t.Errorf("%q age %s: got %v want %v", tc.value, tc.age, got, tc.want)
		}
	}
}
func TestCIMDPublicDialPinsValidatedIP(t *testing.T) {
	for _, address := range []string{"127.0.0.1", "10.0.0.1", "172.16.1.2", "192.168.1.1", "169.254.169.254", "100.100.100.200", "0.0.0.1", "224.0.0.1", "240.1.1.1", "::1", "::ffff:127.0.0.1", "fc00::1", "fe80::1", "64:ff9b::7f00:1", "2002:7f00:1::1", "2001:db8::1"} {
		if publicMetadataIP(netip.MustParseAddr(address)) {
			t.Errorf("accepted nonpublic %s", address)
		}
	}
	for _, mixed := range []bool{false, true} {
		t.Run(fmt.Sprint(mixed), func(t *testing.T) {
			lookupCount, dialCount := 0, 0
			dial := publicDialContext(func(context.Context, string, string) ([]netip.Addr, error) {
				lookupCount++
				ips := []netip.Addr{netip.MustParseAddr("8.8.8.8")}
				if mixed {
					ips = append(ips, netip.MustParseAddr("127.0.0.1"))
				}
				return ips, nil
			}, func(_ context.Context, _ string, addr string) (net.Conn, error) {
				dialCount++
				if addr != "8.8.8.8:443" {
					t.Errorf("dialed hostname/unvalidated IP %s", addr)
				}
				return nil, errors.New("test connection")
			})
			_, _ = dial(context.Background(), "tcp", "client.example:443")
			if lookupCount != 1 || (!mixed && dialCount != 1) || (mixed && dialCount != 0) {
				t.Fatalf("lookup=%d dial=%d mixed=%v", lookupCount, dialCount, mixed)
			}
		})
	}
}
func TestCIMDConcurrencyBound(t *testing.T) {
	r := NewCIMDResolver()
	entered := make(chan struct{}, 4)
	release := make(chan struct{})
	done := make(chan struct{}, 4)
	var calls atomic.Int32
	r.http.Transport = metadataRoundTrip(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		entered <- struct{}{}
		<-release
		return metadataResponse(metadataBody(t, nil)), nil
	})
	for range 4 {
		go func() { defer func() { done <- struct{}{} }(); _, _ = r.Resolve(context.Background(), testMetadataID) }()
	}
	for range 4 {
		<-entered
	}
	_, err := r.Resolve(context.Background(), testMetadataID)
	close(release)
	for range 4 {
		<-done
	}
	if err == nil || calls.Load() != 4 {
		t.Fatalf("unbounded requests: calls=%d err=%v", calls.Load(), err)
	}
}
func TestCIMDProductionTransport(t *testing.T) {
	r := NewCIMDResolver()
	tr := r.http.Transport.(*http.Transport)
	if tr.Proxy != nil || tr.TLSClientConfig != nil || !tr.DisableKeepAlives || r.http.Timeout != 5*time.Second {
		t.Fatal("unsafe transport defaults")
	}
	if err := r.http.CheckRedirect(nil, nil); err != http.ErrUseLastResponse {
		t.Fatal("redirect enabled")
	}
	if _, err := r.Resolve(context.Background(), "https://127.0.0.1/metadata.json"); err == nil {
		t.Fatal("loopback fetched")
	}
}

func TestCIMDCacheCapacityAndFailureDoesNotServeStale(t *testing.T) {
	r := NewCIMDResolver()
	now := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	r.now = func() time.Time { return now }
	fail := false
	r.http.Transport = metadataRoundTrip(func(req *http.Request) (*http.Response, error) {
		if fail {
			return nil, errors.New("fetch unavailable")
		}
		return metadataResponse(metadataBody(t, map[string]any{"client_id": req.URL.String()})), nil
	})
	for i := range 130 {
		if _, err := r.Resolve(context.Background(), fmt.Sprintf("%s?client=%d", testMetadataID, i)); err != nil {
			t.Fatal(err)
		}
	}
	if len(r.cache) != 128 {
		t.Fatalf("cache contains %d entries", len(r.cache))
	}
	now = now.Add(2 * time.Minute)
	fail = true
	if _, err := r.Resolve(context.Background(), testMetadataID+"?client=129"); err == nil {
		t.Fatal("stale metadata served on failure")
	}
}

func TestCIMDExactFieldNames(t *testing.T) {
	r := NewCIMDResolver()
	for _, body := range []string{
		`{"CLIENT_ID":"` + testMetadataID + `","client_name":"Client","redirect_uris":["https://client.example/callback"]}`,
		`{"client_id":"wrong","CLIENT_ID":"` + testMetadataID + `","client_name":"Client","redirect_uris":["https://client.example/callback"]}`,
		`{"client_id":"` + testMetadataID + `","client_name":"Client","redirect_uris":["https://client.example/callback"],"REDIRECT_URIS":["https://evil.example/callback"]}`,
	} {
		if _, err := r.parseMetadata(testMetadataID, []byte(body)); err == nil {
			t.Fatal("accepted ambiguous case variant")
		}
	}
	if !validClientMetadataURL("https://client.example/") {
		t.Fatal("root path is permitted by CIMD")
	}
}
