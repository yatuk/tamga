package linkscan

import "strings"

// A tool that sends data out (mail, a webhook, an HTTP request, an upload)
// is told where to send it in its arguments. Destinations reads those
// places out of a piece of text, so that a policy can hold them to a list
// of the places it allows. Like the rest of this package it reads text and
// nothing else.

// Kinds of destination.
const (
	// DestURL is the host of an address with an authority: scheme://host.
	DestURL = "url"
	// DestEmail is the domain of an e-mail address.
	DestEmail = "email"
	// DestHost is a value that is a host name or an IP address and nothing
	// else ("host": "files.example.net").
	DestHost = "host"
)

// Destination is one place a text names.
type Destination struct {
	// Start and End delimit what names it in the text: the address, the
	// e-mail address, or the host.
	Start, End int
	// Host is the host or the e-mail domain, lower-cased, without a port
	// or a trailing dot.
	Host string
	Kind string
}

// Destinations returns the addresses with an authority and the e-mail
// addresses in text, in order. An address of any scheme counts
// (sftp://, smtp://, s3://): what matters here is where it points, not how
// a reader would follow it. An address without a host (file:///, a
// relative path) is no destination.
func Destinations(text string) []Destination {
	var out []Destination
	for at := 0; ; {
		j := strings.Index(text[at:], "://")
		if j < 0 {
			break
		}
		j += at
		at = j + 3
		from := j
		for from > 0 && isSchemeByte(text[from-1]) {
			from--
		}
		for from < j && !isLetter(text[from]) {
			from++
		}
		if from == j {
			continue
		}
		to := bareEnd(text, j+3)
		if host, ok := authorityHost(text[j+3 : to]); ok {
			out = append(out, Destination{Start: from, End: to, Host: host, Kind: DestURL})
		}
		at = to
	}
	urls := len(out)
	for _, e := range emailAddresses(text) {
		// The user information of an address (sftp://user@host) is not
		// a second destination.
		inUser := false
		for _, u := range out[:urls] {
			if u.Start <= e.Start && e.End <= u.End && u.Host == e.Host {
				inUser = true
				break
			}
		}
		if !inUser {
			out = append(out, e)
		}
	}
	sortDestinations(out)
	return out
}

func sortDestinations(d []Destination) {
	for i := 1; i < len(d); i++ {
		for k := i; k > 0 && d[k].Start < d[k-1].Start; k-- {
			d[k], d[k-1] = d[k-1], d[k]
		}
	}
}

// authorityHost reads the host out of what follows "scheme://".
func authorityHost(rest string) (string, bool) {
	end := len(rest)
	for k := 0; k < len(rest); k++ {
		if c := rest[k]; c == '/' || c == '\\' || c == '?' || c == '#' {
			end = k
			break
		}
	}
	authority := rest[:end]
	if at := strings.LastIndexByte(authority, '@'); at >= 0 {
		authority = authority[at+1:]
	}
	return hostPart(percentDecode(authority))
}

// hostPart takes the port off a host and checks what is left is one.
func hostPart(s string) (string, bool) {
	host := s
	if strings.HasPrefix(host, "[") {
		close := strings.IndexByte(host, ']')
		if close < 0 {
			return "", false
		}
		host = host[:close+1]
	} else if c := strings.LastIndexByte(host, ':'); c >= 0 {
		port := host[c+1:]
		for _, ch := range []byte(port) {
			if ch < '0' || ch > '9' {
				return "", false
			}
		}
		host = host[:c]
	}
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if host == "" || len(host) > maxHostBytes || strings.ContainsAny(host, " \t\r\n\"'<>`{}|") {
		return "", false
	}
	return host, true
}

// HostValue reports whether value, without the white space around it, is
// a host name with at least one dot, or an IP address, with or without a
// port, and returns the host.
func HostValue(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, " /@?#") {
		return "", false
	}
	host, ok := hostPart(value)
	if !ok {
		return "", false
	}
	if ip, _ := ipHost(host); ip {
		return host, true
	}
	labels := strings.Split(host, ".")
	if len(labels) < 2 {
		return "", false
	}
	for _, l := range labels {
		if l == "" {
			return "", false
		}
		for _, r := range l {
			if r == '-' || r == '_' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || r > 127 {
				continue
			}
			return "", false
		}
	}
	// A top-level domain is letters: "report.pdf" passes, "v1.2" does not.
	tld := labels[len(labels)-1]
	for _, r := range tld {
		if r >= '0' && r <= '9' {
			return "", false
		}
	}
	return host, true
}

// emailAddresses finds local@domain with a dotted domain.
func emailAddresses(text string) []Destination {
	var out []Destination
	for at := 0; at < len(text); {
		k := strings.IndexByte(text[at:], '@')
		if k < 0 {
			break
		}
		k += at
		at = k + 1
		from := k
		for from > 0 && isLocalByte(text[from-1]) {
			from--
		}
		to := k + 1
		for to < len(text) && isDomainByte(text[to]) {
			to++
		}
		for to > k+1 && (text[to-1] == '.' || text[to-1] == '-') {
			to--
		}
		if from == k || to == k+1 {
			continue
		}
		domain := strings.ToLower(text[k+1 : to])
		dot := strings.LastIndexByte(domain, '.')
		if dot <= 0 || len(domain)-dot-1 < 2 {
			continue
		}
		out = append(out, Destination{Start: from, End: to, Host: domain, Kind: DestEmail})
		at = to
	}
	return out
}

func isLocalByte(c byte) bool {
	return isLetter(c) || (c >= '0' && c <= '9') || strings.IndexByte("._%+-'", c) >= 0
}

func isDomainByte(c byte) bool {
	return isLetter(c) || (c >= '0' && c <= '9') || c == '.' || c == '-' || c >= 0x80
}

// HostList is a list of hosts in the form of the deny and allow lists: an
// entry covers the host and every host under it, "*.example.com" only what
// is under it, an IP address that address alone.
type HostList struct{ set hostSet }

// NewHostList builds a list. An entry CheckHost refuses is an error.
func NewHostList(entries []string) (*HostList, error) {
	hs, err := newHostSet(entries)
	if err != nil {
		return nil, err
	}
	return &HostList{set: hs}, nil
}

// Has reports whether the list covers host.
func (l *HostList) Has(host string) bool {
	return l != nil && l.set.has(strings.ToLower(strings.TrimSuffix(host, ".")))
}
