package proxy

import (
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/http"
	"strings"
)

// clientIP returns the address the request came from.
//
// X-Forwarded-For is text the caller can write, so it is read only when the
// directly connected peer is one of the trusted proxies, and then from the
// right: each trusted proxy appends the address it saw, so the first entry
// from the right that is not a trusted proxy is the last address a trusted
// hop vouched for. Anything further left is the client's own claim. With no
// trusted proxies configured the header is ignored and the peer address is
// the answer.
func clientIP(r *http.Request, trusted []*net.IPNet) string {
	peer := peerIP(r)
	if len(trusted) == 0 || !ipTrusted(peer, trusted) {
		return peer
	}
	hops := forwardedFor(r)
	for i := len(hops) - 1; i >= 0; i-- {
		ip := parseHop(hops[i])
		if ip == nil {
			// Not an address, so not something to key a limit or an
			// allowlist on. Stop at the last hop that was understood.
			return peer
		}
		if !ipNetsContain(trusted, ip) {
			return ip.String()
		}
		peer = ip.String()
	}
	return peer
}

func peerIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// forwardedFor returns every X-Forwarded-For entry in order. The header may
// be repeated; the lines are one list.
func forwardedFor(r *http.Request) []string {
	var hops []string
	for _, line := range r.Header.Values("X-Forwarded-For") {
		for _, h := range strings.Split(line, ",") {
			if h = strings.TrimSpace(h); h != "" {
				hops = append(hops, h)
			}
		}
	}
	return hops
}

// parseHop reads one forwarded entry: an address, optionally with a port or
// in brackets.
func parseHop(s string) net.IP {
	if ip := net.ParseIP(strings.Trim(s, "[]")); ip != nil {
		return ip
	}
	if host, _, err := net.SplitHostPort(s); err == nil {
		return net.ParseIP(host)
	}
	return nil
}

func ipTrusted(s string, trusted []*net.IPNet) bool {
	ip := net.ParseIP(s)
	return ip != nil && ipNetsContain(trusted, ip)
}

func ipNetsContain(nets []*net.IPNet, ip net.IP) bool {
	for _, n := range nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// rateLimitKeyForRequest names the bucket a request is counted in: the
// Tamga key when one verified, else the caller's provider key when it sent
// one, else its address. The provider key is hashed because the bucket name
// is stored in Redis and returned by the rate-limit stats endpoint.
func rateLimitKeyForRequest(r *http.Request, who caller, trusted []*net.IPNet) string {
	if who.verified() {
		return "tk:" + who.KeyID
	}
	if k := extractAPIKey(r); k != "" {
		sum := sha256.Sum256([]byte(k))
		return "key:" + hex.EncodeToString(sum[:8])
	}
	return "ip:" + clientIP(r, trusted)
}
