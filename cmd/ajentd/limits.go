package main

import (
	"ajent/internal/ratelimit"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"
)

func parseTrustedProxies(value string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	if value == "" {
		return out, nil
	}
	for _, part := range strings.Split(value, ",") {
		p, err := netip.ParsePrefix(strings.TrimSpace(part))
		if err != nil {
			return nil, fmt.Errorf("invalid AJENT_TRUSTED_PROXIES CIDR: %w", err)
		}
		if p.Bits() == 0 {
			return nil, fmt.Errorf("AJENT_TRUSTED_PROXIES cannot trust every address")
		}
		out = append(out, p)
	}
	return out, nil
}
func clientIP(r *http.Request, trusted []netip.Prefix) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return host
	}
	ip = ip.Unmap()
	isTrusted := func(ip netip.Addr) bool {
		for _, p := range trusted {
			if p.Contains(ip) {
				return true
			}
		}
		return false
	}
	if !isTrusted(ip) {
		return ip.String()
	}
	parts := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	for i := len(parts) - 1; i >= 0; i-- {
		next, err := netip.ParseAddr(strings.TrimSpace(parts[i]))
		if err != nil {
			return host
		}
		ip = next.Unmap()
		if !isTrusted(ip) {
			return ip.String()
		}
	}
	return ip.String()
}
func limit(next http.Handler, trusted []netip.Prefix) http.Handler {
	requests := ratelimit.New(600, time.Minute, 10000)
	logins := ratelimit.New(10, time.Minute, 10000)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r, trusted)
		if !requests.Allow(ip, time.Now()) {
			w.Header().Set("Retry-After", "1")
			http.Error(w, "Request allowance exceeded.", 429)
			return
		}
		if (r.URL.Path == "/login" || r.URL.Path == "/v1/signup") && r.Method == "POST" && !logins.Allow(ip, time.Now()) {
			w.Header().Set("Retry-After", "6")
			http.Error(w, "Please try again shortly.", 429)
			return
		}
		r = r.Clone(r.Context())
		r.RemoteAddr = net.JoinHostPort(ip, "0")
		next.ServeHTTP(w, r)
	})
}
