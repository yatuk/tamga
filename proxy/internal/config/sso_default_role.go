package config

import (
	"fmt"
	"strings"
)

// ParseSSODefaultRole reads TAMGA_SSO_DEFAULT_ROLE. Empty means that an
// account nobody assigned a role is refused; "viewer" lets it in as a
// viewer. No other role can be a default: who writes the policy or reads
// credentials is decided per person.
func ParseSSODefaultRole(raw string) (string, error) {
	switch v := strings.ToLower(strings.TrimSpace(raw)); v {
	case "", "viewer":
		return v, nil
	default:
		return "", fmt.Errorf("TAMGA_SSO_DEFAULT_ROLE is %q: leave it empty to refuse accounts without an assigned role, or set it to \"viewer\"", raw)
	}
}
