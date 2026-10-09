package config

import (
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
)

// ParseUpstreamAllow reads TAMGA_UPSTREAM_ALLOW: a comma-separated list of
// CIDR ranges, single IP addresses and exact host names that a provider pool
// in the policy may point at besides the vendors' own hosts. It is read the
// way TAMGA_ALERT_ALLOW_PRIVATE is, and is a separate list on purpose: a
// receiver of alerts is not thereby somewhere to send prompts and provider
// keys.
//
// An entry may end in ":port" (a host name or an address, "[::1]:8080" for
// IPv6); it then allows that port only and is returned in hostPorts. An
// entry without one allows every port. A listed host or address may be used
// by a pool on any route, over plain http, and is connected to whatever it
// resolves to. There is no wildcard: 0.0.0.0/0 and ::/0 are refused.
func ParseUpstreamAllow(raw string) (nets []netip.Prefix, hosts, hostPorts []string, err error) {
	const name = "TAMGA_UPSTREAM_ALLOW"
	for _, p := range strings.Split(raw, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if h, port, ok := splitAllowPort(p); ok {
			n, err := strconv.Atoi(port)
			if err != nil || n < 1 || n > 65535 {
				return nil, nil, nil, fmt.Errorf("%s: %q does not end in a port number", name, p)
			}
			if ip, err := netip.ParseAddr(h); err == nil && ip.Zone() == "" {
				h = ip.Unmap().String()
			} else if h = strings.TrimSuffix(strings.ToLower(h), "."); !validAlertHost(h) {
				return nil, nil, nil, fmt.Errorf("%s: %q is not a CIDR range, an IP address or a host name", name, p)
			}
			hostPorts = append(hostPorts, net.JoinHostPort(h, strconv.Itoa(n)))
			continue
		}
		n, h, err := parseAllowList(name, "list the hosts or ranges the model servers are in", p)
		if err != nil {
			return nil, nil, nil, err
		}
		nets = append(nets, n...)
		hosts = append(hosts, h...)
	}
	return nets, hosts, hostPorts, nil
}

// splitAllowPort splits "host:port" and "[v6]:port". A bare IPv6 address has
// colons of its own and is not such an entry.
func splitAllowPort(p string) (host, port string, ok bool) {
	if strings.Contains(p, "/") {
		return "", "", false
	}
	if _, err := netip.ParseAddr(p); err == nil {
		return "", "", false
	}
	host, port, err := net.SplitHostPort(p)
	if err != nil || host == "" {
		return "", "", false
	}
	return host, port, true
}

// ParseEnvNames reads a comma-separated list of environment variable names
// that a policy may refer to (TAMGA_UPSTREAM_KEY_ENVS for a pool endpoint's
// api_key_env, TAMGA_NOTIFY_URL_ENVS for a rule's notify value). Names are
// kept exactly as written, and the policy has to spell a name the same way
// to match.
//
// One of the proxy's own variables (TAMGA_*) is an error rather than
// something to skip: listing it would hand the admin key, the vault key or
// the database URL to whoever may edit the policy.
func ParseEnvNames(variable, raw string) ([]string, error) {
	var names []string
	for _, n := range strings.Split(raw, ",") {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		if len(n) >= 6 && strings.EqualFold(n[:6], "TAMGA_") {
			return nil, fmt.Errorf("%s: %q is one of the proxy's own variables and cannot be listed", variable, n)
		}
		if !validEnvName(n) {
			return nil, fmt.Errorf("%s: %q cannot be listed: a name must be upper-case letters, digits and underscores, starting with a letter", variable, n)
		}
		names = append(names, n)
	}
	return names, nil
}

// validEnvName accepts upper-case letters, digits and underscores, starting
// with a letter. Upper case only: on Windows the environment is not
// case-sensitive, and a lower-case name would read its upper-case twin.
func validEnvName(n string) bool {
	if n == "" || n[0] < 'A' || n[0] > 'Z' {
		return false
	}
	for _, c := range n {
		if !(c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_') {
			return false
		}
	}
	return true
}
