package proxy

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"

	"github.com/rs/zerolog/log"
)

// NewIPAllowlistMiddleware returns an HTTP middleware that enforces IP allowlist.
// When allowlistRaw is empty, the returned middleware is a no-op (passes all traffic).
// Otherwise, it parses the comma-separated CIDR ranges and rejects any request whose
// client IP does not match at least one range.
//
// Invalid CIDR entries are logged as warnings and skipped rather than causing a
// hard failure — this keeps the proxy running even when an operator fat-fingers
// a range, but the misconfiguration is visible in logs.
func NewIPAllowlistMiddleware(allowlistRaw string) func(http.Handler) http.Handler {
	cidrs := parseIPAllowlist(allowlistRaw)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if len(cidrs) == 0 {
				next.ServeHTTP(w, r)
				return
			}

			ip := clientIP(r)
			parsed := net.ParseIP(ip)
			if parsed == nil {
				log.Warn().
					Str("component", "ip_allowlist").
					Str("ip", ip).
					Str("remote_addr", r.RemoteAddr).
					Msg("cannot parse client IP; rejecting")
				writeIPRejected(w)
				return
			}

			for i := range cidrs {
				if cidrs[i].Contains(parsed) {
					next.ServeHTTP(w, r)
					return
				}
			}

			log.Warn().
				Str("component", "ip_allowlist").
				Str("ip", ip).
				Msg("request rejected by IP allowlist")
			writeIPRejected(w)
		})
	}
}

// parseIPAllowlist parses a comma-separated CIDR string into []net.IPNet.
// Invalid entries are logged and skipped.
func parseIPAllowlist(raw string) []net.IPNet {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	cidrs := make([]net.IPNet, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		_, cidr, err := net.ParseCIDR(p)
		if err != nil {
			log.Warn().
				Str("component", "ip_allowlist").
				Str("cidr", p).
				Err(err).
				Msg("invalid CIDR in IP allowlist; skipping")
			continue
		}
		cidrs = append(cidrs, *cidr)
	}
	return cidrs
}

func writeIPRejected(w http.ResponseWriter) {
	w.Header().Set("X-Tamga-Reject-Reason", "ip_not_allowed")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]interface{}{
			"message": "IP not allowed",
			"type":    "ip_not_allowed",
		},
	})
}
