package config

import (
	"fmt"
	"net/netip"
	"strings"
)

// ParseAlertAllowPrivate reads TAMGA_ALERT_ALLOW_PRIVATE: a comma-separated
// list of CIDR ranges, single IP addresses and exact host names that alerts
// may be sent to although they are internal. A value that is none of those
// is an error rather than something to skip: a mistyped entry would either
// leave a receiver unreachable or open more than was meant.
//
// A host name matches exactly, without regard to case or a trailing dot: it
// does not admit its subdomains or a longer name that ends the same way. A
// name entry trusts whatever that name resolves to, so list only names whose
// DNS the operator controls.
//
// There is no wildcard and no "allow everything" switch: 0.0.0.0/0 and ::/0
// are refused. An operator who wants more has to write the ranges out, and
// the proxy logs them when it starts.
func ParseAlertAllowPrivate(raw string) ([]netip.Prefix, []string, error) {
	var nets []netip.Prefix
	var hosts []string
	for _, p := range strings.Split(raw, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if strings.Contains(p, "/") {
			n, err := netip.ParsePrefix(p)
			if err != nil {
				return nil, nil, fmt.Errorf("TAMGA_ALERT_ALLOW_PRIVATE: %q is not a CIDR range, an IP address or a host name", p)
			}
			if n.Bits() == 0 {
				return nil, nil, fmt.Errorf("TAMGA_ALERT_ALLOW_PRIVATE: %q would allow every address; list the ranges the receivers are in", p)
			}
			// An IPv4-mapped range is kept as the IPv4 range it stands for,
			// which is the form addresses are judged in.
			if n.Addr().Is4In6() && n.Bits() >= 96 {
				n = netip.PrefixFrom(n.Addr().Unmap(), n.Bits()-96)
				if n.Bits() == 0 {
					return nil, nil, fmt.Errorf("TAMGA_ALERT_ALLOW_PRIVATE: %q would allow every address; list the ranges the receivers are in", p)
				}
			}
			if n.Addr().Zone() != "" {
				return nil, nil, fmt.Errorf("TAMGA_ALERT_ALLOW_PRIVATE: %q is not a CIDR range, an IP address or a host name", p)
			}
			nets = append(nets, n.Masked())
			continue
		}
		if ip, err := netip.ParseAddr(p); err == nil {
			ip = ip.WithZone("").Unmap()
			nets = append(nets, netip.PrefixFrom(ip, ip.BitLen()))
			continue
		}
		h := strings.TrimSuffix(strings.ToLower(p), ".")
		if !validAlertHost(h) {
			return nil, nil, fmt.Errorf("TAMGA_ALERT_ALLOW_PRIVATE: %q is not a CIDR range, an IP address or a host name", p)
		}
		hosts = append(hosts, h)
	}
	return nets, hosts, nil
}

// validAlertHost accepts a plain host name: letters, digits, hyphens and
// underscores in dot-separated labels. No scheme, port, path or wildcard,
// and not a name whose last label is a number, which is a mistyped address.
func validAlertHost(h string) bool {
	if h == "" || len(h) > 253 {
		return false
	}
	labels := strings.Split(h, ".")
	for _, l := range labels {
		if l == "" || len(l) > 63 {
			return false
		}
		for _, c := range l {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
				return false
			}
		}
	}
	last := labels[len(labels)-1]
	return strings.Trim(last, "0123456789") != "" && !strings.HasPrefix(last, "0x")
}
