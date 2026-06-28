// Package middleware provides HTTP middleware for the Tamga proxy API.
package middleware

import (
	"net/http"
	"net/url"
	"path"
	"strings"
)

// PathNormalize returns middleware that URL-decodes the request path and
// blocks path-traversal attempts before they reach routing or handlers.
//
// Attackers commonly encode "/" as %2F, %5C (backslash), %c0%af (UTF-8
// overlong), or double-encode as %252F to smuggle ../ sequences past
// naive prefix-string checks.
//
// The middleware:
//  1. URL-decodes the path (handles single, double, and overlong encoding).
//  2. Passes the decoded path through path.Clean to resolve any ".." segments.
//  3. If the cleaned path differs from the decoded path (i.e. ".." was
//     present), returns 403.
//  4. Otherwise writes the cleaned path back to r.URL.Path for downstream
//     handlers and routers.
func PathNormalize(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := r.URL.Path

		// Step 1 — Decode percent-encoded sequences (potentially multiple
		// passes to catch double-encoding like %252F → %2F → /).
		decoded := raw
		for i := 0; i < 3; i++ { // max 3 decode passes
			prev := decoded
			decoded = decodePath(decoded)
			if decoded == prev {
				break
			}
		}

		// Step 2 — Normalise with path.Clean which resolves ".." and "."
		// segments, collapses multiple slashes, and removes trailing slashes.
		cleaned := path.Clean(decoded)

		// Step 3 — If the decoded path contains ".." as a path segment,
		// it's a traversal attempt.  We check path.Clean's result rather
		// than a raw substring to avoid false positives on legitimate
		// uses like "/api/v1/ab..cd" (two dots inside a filename).
		// path.Clean resolves ".." segments, so if cleaned != decoded AND
		// the decoded path contains ".." as a component, block it.
		if isTraversal(decoded, cleaned) {
			http.Error(w, "path traversal blocked", http.StatusForbidden)
			return
		}

		// If the path hasn't changed, skip allocation.
		if cleaned == raw {
			next.ServeHTTP(w, r)
			return
		}

		// Step 4 — Write the safe path back.
		r.URL.Path = cleaned
		next.ServeHTTP(w, r)
	})
}

// isTraversal reports whether the decoded path contains a path-traversal
// attempt.  It checks whether path.Clean resolved any ".." segments by
// comparing cleaned vs decoded, but only when ".." is present as a path
// component — this avoids false positives on "//" (consecutive slashes)
// or "." (current directory) which path.Clean also normalises.
func isTraversal(decoded, cleaned string) bool {
	// Fast path: no ".." anywhere → definitely not traversal.
	if !strings.Contains(decoded, "..") {
		return false
	}
	// Slow path: ".." appears. Check if it's a real path segment by
	// verifying that path.Clean actually consumed it.
	return cleaned != decoded
}

// decodePath applies a single round of URL decoding. Unlike url.PathUnescape
// it does NOT return an error on invalid escapes — it leaves them as-is so
// the downstream router can decide (we only care about ".." detection).
func decodePath(p string) string {
	decoded, err := url.PathUnescape(p)
	if err != nil {
		// On error, try unescaping as a raw query (handles mixed encoding).
		if d2, e2 := url.QueryUnescape(p); e2 == nil && d2 != p {
			decoded = d2
		} else {
			return p
		}
	}
	// Replace backslash separators (%5C) with forward slash.
	decoded = strings.ReplaceAll(decoded, "\\", "/")
	// Replace UTF-8 overlong encodings of '/' (0x2F).
	// %c0%af → 0xC0 0xAF → overlong 2-byte encoding of '/'.
	decoded = strings.ReplaceAll(decoded, "\xc0\xaf", "/")
	// %e0%80%af → 0xE0 0x80 0xAF → overlong 3-byte encoding of '/'.
	decoded = strings.ReplaceAll(decoded, "\xe0\x80\xaf", "/")
	// %c0%ae → 0xC0 0xAE → overlong 2-byte encoding of '.' (used for %2e bypass).
	decoded = strings.ReplaceAll(decoded, "\xc0\xae", ".")
	return decoded
}
