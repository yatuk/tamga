package config

import (
	"fmt"
	"net"
	"strings"
)

// ParseTrustedProxies parses a comma-separated list of IPs and CIDR ranges.
// A bare address means that one host. Unlike the IP allowlist, a bad entry is
// an error: a typo here would otherwise silently change whose address the
// proxy believes.
func ParseTrustedProxies(raw string) ([]*net.IPNet, error) {
	var out []*net.IPNet
	for _, p := range strings.Split(raw, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if !strings.Contains(p, "/") {
			ip := net.ParseIP(p)
			if ip == nil {
				return nil, fmt.Errorf("TAMGA_TRUSTED_PROXIES: %q is not an IP address or CIDR range", p)
			}
			bits := 128
			if v4 := ip.To4(); v4 != nil {
				ip, bits = v4, 32
			}
			out = append(out, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
			continue
		}
		_, n, err := net.ParseCIDR(p)
		if err != nil {
			return nil, fmt.Errorf("TAMGA_TRUSTED_PROXIES: %q is not an IP address or CIDR range", p)
		}
		out = append(out, n)
	}
	return out, nil
}
