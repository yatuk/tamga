package linkscan

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Limits on the lists an operator supplies.
const (
	// MaxDenyHosts and MaxAllowHosts bound the two lists. A look-up costs
	// one map read per label of the host, whatever the size of the list;
	// the bound is on what a policy holds in memory.
	MaxDenyHosts  = 5000
	MaxAllowHosts = 1000
)

// Lists are the hosts an operator denies and the ones it vouches for.
//
// An entry is a host name ("files.example.com") and covers that host and
// every host under it. "*.example.com" covers the hosts under example.com
// and not example.com itself. An IP address is an entry for that address
// alone.
type Lists struct {
	deny, allow hostSet
}

type hostSet struct {
	// exact covers the host and what is under it; under covers only what
	// is under it.
	exact, under map[string]bool
}

// NewLists builds the lists. An entry CheckHost refuses is an error.
func NewLists(deny, allow []string) (*Lists, error) {
	l := &Lists{}
	var err error
	if l.deny, err = newHostSet(deny); err != nil {
		return nil, err
	}
	if l.allow, err = newHostSet(allow); err != nil {
		return nil, err
	}
	return l, nil
}

func newHostSet(entries []string) (hostSet, error) {
	hs := hostSet{}
	for _, e := range entries {
		host, wildcard, err := parseEntry(e)
		if err != nil {
			return hostSet{}, err
		}
		if wildcard {
			if hs.under == nil {
				hs.under = map[string]bool{}
			}
			hs.under[host] = true
			continue
		}
		if hs.exact == nil {
			hs.exact = map[string]bool{}
		}
		hs.exact[host] = true
	}
	return hs, nil
}

// CheckHost reports why an entry of a host list is not accepted.
func CheckHost(entry string) error {
	_, _, err := parseEntry(entry)
	return err
}

func parseEntry(entry string) (host string, wildcard bool, err error) {
	host = strings.ToLower(strings.TrimSpace(entry))
	if rest, ok := strings.CutPrefix(host, "*."); ok {
		host, wildcard = rest, true
	}
	host = strings.TrimSuffix(host, ".")
	switch {
	case host == "":
		return "", false, errors.New("an entry is empty")
	case len(host) > maxHostBytes:
		return "", false, fmt.Errorf("an entry is %d bytes, a host name is at most %d", len(host), maxHostBytes)
	case strings.Contains(host, "://") || strings.ContainsAny(host, "/?#@ \t"):
		return "", false, errors.New("an entry must be a host name alone: no scheme, path, user or port")
	case strings.Contains(host, "*"):
		return "", false, errors.New("\"*\" is accepted only as a leading \"*.\"")
	}
	if strings.HasPrefix(host, "[") {
		if !strings.HasSuffix(host, "]") {
			return "", false, errors.New("an IPv6 entry is written in brackets: [2001:db8::1]")
		}
		return host, wildcard, nil
	}
	if strings.ContainsAny(host, ":") {
		return "", false, errors.New("an entry must be a host name alone: no scheme, path, user or port")
	}
	if !wildcard && !strings.Contains(host, ".") {
		// A single label is a host of the operator's own network; it is
		// accepted. As a wildcard it would cover a whole top-level domain.
		return host, false, nil
	}
	if wildcard && !strings.Contains(host, ".") {
		return "", false, errors.New("a wildcard entry must name a domain, not a top-level domain")
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" {
			return "", false, errors.New("an entry has an empty label")
		}
	}
	return host, wildcard, nil
}

func (hs hostSet) has(host string) bool {
	if len(hs.exact) == 0 && len(hs.under) == 0 {
		return false
	}
	if hs.exact[host] {
		return true
	}
	for {
		dot := strings.IndexByte(host, '.')
		if dot < 0 {
			return false
		}
		host = host[dot+1:]
		if hs.exact[host] || hs.under[host] {
			return true
		}
	}
}

func (l *Lists) denied(host string) bool  { return l != nil && l.deny.has(host) }
func (l *Lists) allowed(host string) bool { return l != nil && l.allow.has(host) }

// latinLookalikes are the Cyrillic and Greek letters that are drawn like a
// Latin letter in most typefaces.
var latinLookalikes = map[rune]bool{
	// Cyrillic
	'а': true, 'е': true, 'о': true, 'р': true, 'с': true, 'у': true, 'х': true,
	'ѕ': true, 'і': true, 'ј': true, 'һ': true, 'ԁ': true, 'ԛ': true, 'ԝ': true, 'ӏ': true,
	// Greek
	'ο': true, 'α': true, 'ν': true, 'ρ': true, 'υ': true, 'ι': true, 'κ': true,
}

// lookalikeHost reports whether a label of the host is written to be read
// as a Latin name it is not: Latin letters mixed with Cyrillic or Greek
// ones, or a label made only of Cyrillic or Greek letters that look Latin.
// A label in punycode ("xn--…") is decoded first.
//
// A name wholly in another script (яндекс.рф) is a name in that script, and
// Latin letters with marks (türkiye.gov.tr) are Latin: neither is reported.
func lookalikeHost(host string) bool {
	ascii := true
	for i := 0; i < len(host); i++ {
		if host[i] >= utf8.RuneSelf {
			ascii = false
			break
		}
	}
	if ascii && !strings.Contains(host, "xn--") {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if rest, ok := strings.CutPrefix(label, "xn--"); ok {
			decoded, ok := punycodeDecode(rest)
			if !ok {
				continue
			}
			label = decoded
		}
		latin, other, alike, letters := 0, 0, 0, 0
		for _, r := range label {
			switch {
			case unicode.Is(unicode.Latin, r):
				latin++
				letters++
			case unicode.Is(unicode.Cyrillic, r), unicode.Is(unicode.Greek, r):
				other++
				letters++
				if latinLookalikes[unicode.ToLower(r)] {
					alike++
				}
			}
		}
		if other == 0 {
			continue
		}
		if latin > 0 || alike == letters {
			return true
		}
	}
	return false
}

// punycodeDecode decodes the part of a label after "xn--" (RFC 3492).
func punycodeDecode(encoded string) (string, bool) {
	const (
		base, tmin, tmax = 36, 1, 26
		skew, damp       = 38, 700
		initialBias      = 72
		initialN         = 128
	)
	var out []rune
	pos := 0
	if delim := strings.LastIndexByte(encoded, '-'); delim >= 0 {
		for _, r := range encoded[:delim] {
			if r >= utf8.RuneSelf {
				return "", false
			}
			out = append(out, r)
		}
		pos = delim + 1
	}
	if pos >= len(encoded) {
		return "", false
	}
	n, i, bias := initialN, 0, initialBias
	for pos < len(encoded) {
		oldi, w := i, 1
		for k := base; ; k += base {
			if pos >= len(encoded) {
				return "", false
			}
			c := encoded[pos]
			pos++
			var digit int
			switch {
			case c >= 'a' && c <= 'z':
				digit = int(c - 'a')
			case c >= 'A' && c <= 'Z':
				digit = int(c - 'A')
			case c >= '0' && c <= '9':
				digit = int(c-'0') + 26
			default:
				return "", false
			}
			if digit > (1<<30-i)/w {
				return "", false
			}
			i += digit * w
			t := k - bias
			if t < tmin {
				t = tmin
			} else if t > tmax {
				t = tmax
			}
			if digit < t {
				break
			}
			if w > (1<<30)/(base-t) {
				return "", false
			}
			w *= base - t
		}
		// Bias adaptation.
		delta := i - oldi
		if oldi == 0 {
			delta /= damp
		} else {
			delta /= 2
		}
		delta += delta / (len(out) + 1)
		k := 0
		for delta > ((base-tmin)*tmax)/2 {
			delta /= base - tmin
			k += base
		}
		bias = k + (base-tmin+1)*delta/(delta+skew)

		n += i / (len(out) + 1)
		i %= len(out) + 1
		if n > unicode.MaxRune || len(out) > 63 {
			return "", false
		}
		out = append(out, 0)
		copy(out[i+1:], out[i:])
		out[i] = rune(n)
		i++
	}
	return string(out), true
}
