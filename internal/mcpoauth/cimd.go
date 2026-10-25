// Adapted from Serenity internal/hosted/oauth/cimd.go at 29936ab2bc0c0c659683750cd1e935ad2ce226e6.
// Copyright Serenity contributors. Licensed under Apache-2.0.
package mcpoauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/ajent-social/go/mcpoauth"
)

// ClientMetadataResolver resolves public OAuth client metadata without registration.
type ClientMetadataResolver interface {
	Resolve(context.Context, string) (mcpoauth.Client, error)
}

// Rejected metadata identifies no usable client; it is not an internal server failure.
var errInvalidClientMetadata = fmt.Errorf("%w: invalid client metadata", mcpoauth.ErrNotFound)

type cachedMetadata struct {
	client  mcpoauth.Client
	expires time.Time
}

// CIMDResolver accepts public PKCE clients. Metadata never conveys authorization.
// Its transport pins each connection to a validated public DNS result.
type CIMDResolver struct {
	http  *http.Client
	slots chan struct{}
	mu    sync.Mutex
	cache map[string]cachedMetadata
	now   func() time.Time
}

// NewCIMDResolver constructs a resolver for public OAuth client metadata.
func NewCIMDResolver() *CIMDResolver {
	transport := &http.Transport{
		DialContext:         publicDialContext(net.DefaultResolver.LookupNetIP, (&net.Dialer{Timeout: 3 * time.Second}).DialContext),
		TLSHandshakeTimeout: 3 * time.Second, ResponseHeaderTimeout: 3 * time.Second,
		MaxResponseHeaderBytes: 16 << 10, DisableKeepAlives: true,
	}
	r := &CIMDResolver{http: &http.Client{Transport: transport, Timeout: 5 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
		slots: make(chan struct{}, 4), cache: make(map[string]cachedMetadata), now: time.Now}
	return r
}

func validClientMetadataURL(id string) bool {
	if len(id) > 2048 {
		return false
	}
	u, err := url.Parse(id)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || strings.Contains(id, "#") || u.Opaque != "" || u.Path == "" {
		return false
	}
	for _, part := range strings.Split(u.Path, "/") {
		if part == "." || part == ".." {
			return false
		}
	}
	if u.Port() != "" {
		p, e := strconv.Atoi(u.Port())
		if e != nil || p < 1 || p > 65535 {
			return false
		}
	}
	return true
}

// Resolve fetches or reads bounded cached metadata. Failures never return stale data.
func (r *CIMDResolver) Resolve(ctx context.Context, id string) (mcpoauth.Client, error) {
	if !validClientMetadataURL(id) {
		return mcpoauth.Client{}, errInvalidClientMetadata
	}
	r.mu.Lock()
	cached, ok := r.cache[id]
	r.mu.Unlock()
	if ok && r.now().Before(cached.expires) {
		return cloneMetadataClient(cached.client), nil
	}
	select {
	case r.slots <- struct{}{}:
		defer func() { <-r.slots }()
	default:
		return mcpoauth.Client{}, errors.New("client metadata resolver busy")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, id, nil)
	if err != nil {
		return mcpoauth.Client{}, errInvalidClientMetadata
	}
	req.Header.Set("Accept", "application/json")
	resp, err := r.http.Do(req)
	if err != nil {
		return mcpoauth.Client{}, fmt.Errorf("fetch client metadata: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return mcpoauth.Client{}, errInvalidClientMetadata
	}
	media, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		return mcpoauth.Client{}, errInvalidClientMetadata
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, (64<<10)+1))
	if err != nil || len(body) > 64<<10 {
		return mcpoauth.Client{}, errInvalidClientMetadata
	}
	client, err := r.parseMetadata(id, body)
	if err != nil {
		return mcpoauth.Client{}, err
	}
	ttl := metadataTTL(resp.Header)
	if ttl > 0 {
		r.mu.Lock()
		now := r.now()
		for key, item := range r.cache {
			if !now.Before(item.expires) {
				delete(r.cache, key)
			}
		}
		if len(r.cache) >= 128 {
			for key := range r.cache {
				delete(r.cache, key)
				break
			}
		}
		r.cache[id] = cachedMetadata{cloneMetadataClient(client), now.Add(ttl)}
		r.mu.Unlock()
	}
	return client, nil
}

func cloneMetadataClient(c mcpoauth.Client) mcpoauth.Client {
	c.RedirectURIs = append([]string(nil), c.RedirectURIs...)
	return c
}

func (r *CIMDResolver) parseMetadata(id string, body []byte) (mcpoauth.Client, error) {
	// Reject duplicate properties: ambiguous security metadata must not be interpreted
	// differently by this server and the client's publisher.
	dec := json.NewDecoder(strings.NewReader(string(body)))
	token, err := dec.Token()
	if err != nil || token != json.Delim('{') {
		return mcpoauth.Client{}, errInvalidClientMetadata
	}
	fields := make(map[string]json.RawMessage)
	for dec.More() {
		key, err := dec.Token()
		if err != nil {
			return mcpoauth.Client{}, errInvalidClientMetadata
		}
		name, ok := key.(string)
		if !ok {
			return mcpoauth.Client{}, errInvalidClientMetadata
		}
		// OAuth metadata field names are case-sensitive. Reject variants of
		// known keys rather than letting encoding/json apply last-value wins.
		for _, known := range []string{"client_id", "client_name", "redirect_uris", "token_endpoint_auth_method", "grant_types", "response_types", "scope", "client_secret", "client_secret_expires_at", "jwks", "jwks_uri"} {
			if name != known && strings.EqualFold(name, known) {
				return mcpoauth.Client{}, errInvalidClientMetadata
			}
		}
		if _, exists := fields[name]; exists {
			return mcpoauth.Client{}, errInvalidClientMetadata
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return mcpoauth.Client{}, errInvalidClientMetadata
		}
		fields[name] = value
	}
	if _, err := dec.Token(); err != nil {
		return mcpoauth.Client{}, errInvalidClientMetadata
	}
	if _, err := dec.Token(); err != io.EOF {
		return mcpoauth.Client{}, errInvalidClientMetadata
	}
	for _, forbidden := range []string{"client_secret", "client_secret_expires_at", "jwks", "jwks_uri"} {
		if _, ok := fields[forbidden]; ok {
			return mcpoauth.Client{}, errInvalidClientMetadata
		}
	}
	var doc struct {
		ID        string   `json:"client_id"`
		Name      string   `json:"client_name"`
		Redirects []string `json:"redirect_uris"`
		Auth      string   `json:"token_endpoint_auth_method"`
		Grants    []string `json:"grant_types"`
		Responses []string `json:"response_types"`
		Scope     string   `json:"scope"`
	}
	if json.Unmarshal(body, &doc) != nil || doc.ID != id || strings.TrimSpace(doc.Name) == "" || len(doc.Name) > 200 || len(doc.Redirects) == 0 || len(doc.Redirects) > 20 || (doc.Auth != "" && doc.Auth != "none") {
		return mcpoauth.Client{}, errInvalidClientMetadata
	}
	for _, c := range doc.Name {
		if unicode.IsControl(c) {
			return mcpoauth.Client{}, errInvalidClientMetadata
		}
	}
	for _, redirect := range doc.Redirects {
		if len(redirect) > 2048 || !validMetadataRedirect(redirect) {
			return mcpoauth.Client{}, errInvalidClientMetadata
		}
	}
	// A reusable document describes capabilities across authorization servers.
	// Require our code flow, but do not reject additional capabilities. The
	// authorization and token endpoints still enforce this server's own grants.
	if _, explicit := fields["grant_types"]; explicit && !metadataCapabilities(doc.Grants, "authorization_code", false) {
		return mcpoauth.Client{}, errInvalidClientMetadata
	}
	if _, explicit := fields["response_types"]; explicit && !metadataCapabilities(doc.Responses, "code", true) {
		return mcpoauth.Client{}, errInvalidClientMetadata
	}
	// A reusable metadata document may declare scopes for other servers.
	// Validate syntax only; the authorization server enforces its scope ceiling.
	if doc.Scope != "" {
		for _, scope := range strings.Split(doc.Scope, " ") {
			if scope == "" {
				return mcpoauth.Client{}, errInvalidClientMetadata
			}
			for _, c := range scope {
				if c < 0x21 || c > 0x7e || c == '"' || c == '\\' {
					return mcpoauth.Client{}, errInvalidClientMetadata
				}
			}
		}
	}
	return mcpoauth.Client{ID: id, Name: doc.Name, RedirectURIs: doc.Redirects}, nil
}

// metadataCapabilities checks nonempty visible-ASCII values and the required
// capability. Response types may be space-separated combinations (RFC 7591).
func metadataCapabilities(values []string, required string, allowSpace bool) bool {
	found := false
	for _, value := range values {
		if value == "" || strings.TrimSpace(value) != value {
			return false
		}
		for _, c := range value {
			if c < 0x21 || c > 0x7e {
				if c != ' ' || !allowSpace {
					return false
				}
			}
		}
		found = found || value == required
	}
	return found
}

func metadataTTL(h http.Header) time.Duration {
	ttl := time.Minute
	for _, line := range h.Values("Cache-Control") {
		for _, part := range strings.Split(line, ",") {
			key, value, _ := strings.Cut(strings.TrimSpace(part), "=")
			switch strings.ToLower(key) {
			case "no-store", "no-cache":
				return 0
			case "max-age":
				n, err := strconv.ParseInt(strings.Trim(value, "\""), 10, 64)
				if err != nil || n <= 0 {
					return 0
				}
				if n > 300 {
					n = 300
				}
				ttl = time.Duration(n) * time.Second
			}
		}
	}
	if age := h.Get("Age"); age != "" {
		n, err := strconv.ParseInt(age, 10, 64)
		if err != nil || n < 0 || n >= int64(ttl/time.Second) {
			return 0
		}
		ttl -= time.Duration(n) * time.Second
	}
	return ttl
}

type lookupIPs func(context.Context, string, string) ([]netip.Addr, error)
type dialIP func(context.Context, string, string) (net.Conn, error)

func publicDialContext(lookup lookupIPs, dial dialIP) dialIP {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		ips, err := lookup(ctx, "ip", host)
		if err != nil {
			return nil, err
		}
		if len(ips) == 0 {
			return nil, errors.New("client metadata host has no addresses")
		}
		for _, ip := range ips {
			if !publicMetadataIP(ip) {
				return nil, errors.New("client metadata host is not public")
			}
		}
		var last error
		for _, ip := range ips {
			conn, err := dial(ctx, network, net.JoinHostPort(ip.String(), port))
			if err == nil {
				return conn, nil
			}
			last = err
		}
		return nil, last
	}
}

var deniedMetadataNetworks = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"), netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"), netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"), netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"), netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001::/23"), netip.MustParsePrefix("2001:db8::/32"), netip.MustParsePrefix("2002::/16"),
}

func publicMetadataIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	if ip.Is6() && !netip.MustParsePrefix("2000::/3").Contains(ip) {
		return false
	}
	if ip.Zone() != "" || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	for _, prefix := range deniedMetadataNetworks {
		if prefix.Contains(ip) {
			return false
		}
	}
	return true
}

// Authorization rechecks these exact URIs against the server redirect policy.
func validMetadataRedirect(raw string) bool {
	for _, c := range raw {
		if unicode.IsSpace(c) || unicode.IsControl(c) {
			return false
		}
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User != nil || strings.Contains(raw, "#") || u.Opaque != "" {
		return false
	}
	if u.Port() != "" {
		p, err := strconv.Atoi(u.Port())
		if err != nil || p < 1 || p > 65535 {
			return false
		}
	}
	if u.Scheme == "https" {
		return true
	}
	ip, err := netip.ParseAddr(u.Hostname())
	return u.Scheme == "http" && err == nil && ip.IsLoopback()
}
