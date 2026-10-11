package linkscan

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// Limits on the lists an operator supplies.
const (
	// MaxDenyHosts and MaxAllowHosts bound the two lists. A look-up costs
	// one map read per label of the host, whatever the size of the list;
	// the bound is on what a policy holds in memory.
	MaxDenyHosts  = 5000
	MaxAllowHosts = 1000
	// MaxProtectedHosts bounds the names an operator adds to the protected
	// ones. The check costs one map read per label of a host that is not
	// ASCII, whatever the size of the list; a long list is a list nobody
	// has looked at, and every name on it is a name some honest host can
	// read as.
	MaxProtectedHosts = 200
)

// Lists are the hosts an operator denies and the ones it vouches for.
//
// An entry is a host name ("files.example.com") and covers that host and
// every host under it. "*.example.com" covers the hosts under example.com
// and not example.com itself. An IP address is an entry for that address
// alone.
//
// The third list is the protected names: hosts whose look-alikes are
// reported (see protectedSet). It always holds the built-in names; the
// operator's are added with Protect.
type Lists struct {
	deny, allow hostSet
	protect     *protectedSet
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

// A host can be written in Latin letters only and still read as a name it
// is not: gıthub.com with a dotless ı, paypał.com with a barred ł, gíthub.com
// with an accent. No letter of it is of another script, so the mixed-script
// check does not see it. Reporting every host with such a letter is not an
// option: ı is an ordinary Turkish letter, and ışık.com.tr, münchen.de and
// café.fr are honest names. What is reported is narrower: a host that,
// with its Latin look-alikes folded to plain letters, is one of a short list
// of protected names, or a host under one, while as written it is not.
//
// The same holds the other way round for a protected name that is itself
// written with such letters (an operator's şekerbank.example.com.tr): the
// plain sekerbank.example.com.tr reads as it and is reported.
//
// The check is by host, not by label: gıthub.com reads as github.com, and
// gıthub.example does not read as any protected host. A look-alike under
// another top-level domain is reported when that host is protected too.

// builtinProtected are the names protected without any policy. The list is
// short on purpose and was written by hand, from what is publicly reported;
// nothing is downloaded. A name is on it when both hold:
//
//   - it is a host where a developer's or a customer's credentials are
//     entered and that phishing imitates as a matter of record: the source
//     hosts and package registries whose users have been phished through
//     look-alike domains (github.com, gitlab.com, npmjs.com, pypi.org), the
//     consumer brands that lead the published brand-phishing rankings year
//     after year and include the two textbook homograph cases (google.com,
//     microsoft.com, apple.com, amazon.com, paypal.com), and large Turkish
//     retail banks (akbank.com, garantibbva.com.tr, ziraatbank.com.tr);
//   - its own spelling is plain ASCII in the language of its users, so that
//     an honest host does not come out of it by writing the name properly.
//     That is why isbank.com.tr, yapikredi.com.tr, vakifbank.com.tr and
//     turkiye.gov.tr are not here: İş, Yapı, Vakıf and Türkiye are how
//     those names are spelled, and a host written that way is as likely the
//     organisation's own as an imitation. An operator who wants them
//     protected adds them in output_rules.links.protected_hosts.
//
// One honest collision is known: äpple is the Swedish word for apple, so
// äpple.com reads as apple.com to this check as it does to a reader.
var builtinProtected = []string{
	"github.com", "gitlab.com", "npmjs.com", "pypi.org",
	"google.com", "microsoft.com", "apple.com", "amazon.com", "paypal.com",
	"akbank.com", "garantibbva.com.tr", "ziraatbank.com.tr",
}

// BuiltinProtectedHosts returns the names protected without any policy.
func BuiltinProtectedHosts() []string {
	return append([]string(nil), builtinProtected...)
}

// protectedSet holds the protected names by what they fold to.
type protectedSet struct {
	// byFold maps a folded name to the protected hosts that fold to it.
	byFold map[string][]string
	// plainOnly says every protected host is ASCII, so an ASCII host cannot
	// be the look-alike of one.
	plainOnly bool
}

var defaultProtected = mustProtectedSet(builtinProtected)

func mustProtectedSet(hosts []string) *protectedSet {
	ps, err := newProtectedSet(hosts)
	if err != nil {
		panic("linkscan: built-in protected host: " + err.Error())
	}
	return ps
}

func newProtectedSet(hosts []string) (*protectedSet, error) {
	ps := &protectedSet{byFold: make(map[string][]string, len(hosts)), plainOnly: true}
	for _, h := range hosts {
		host, err := parseProtected(h)
		if err != nil {
			return nil, err
		}
		folded := foldLatin(host)
		known := false
		for _, have := range ps.byFold[folded] {
			known = known || have == host
		}
		if known {
			continue
		}
		ps.byFold[folded] = append(ps.byFold[folded], host)
		sort.Strings(ps.byFold[folded])
		if !isASCII(host) {
			ps.plainOnly = false
		}
	}
	return ps, nil
}

// CheckProtectedHost reports why an entry of the protected names is not
// accepted. It has to be a registered name: a wildcard protects nothing a
// plain entry does not, and an address or a single label has no look-alike.
func CheckProtectedHost(entry string) error {
	_, err := parseProtected(entry)
	return err
}

func parseProtected(entry string) (string, error) {
	host, wildcard, err := parseEntry(entry)
	switch {
	case err != nil:
		return "", err
	case wildcard:
		return "", errors.New("a protected host is written without \"*.\": the hosts under it are covered")
	case strings.HasPrefix(host, "[") || !strings.Contains(host, "."):
		return "", errors.New("a protected host must be a domain name")
	}
	if ip, _ := ipHost(host); ip {
		return "", errors.New("a protected host must be a domain name")
	}
	return writtenHost(host), nil
}

// Protect adds the operator's names to the built-in protected ones.
func (l *Lists) Protect(hosts []string) error {
	if len(hosts) == 0 {
		return nil
	}
	ps, err := newProtectedSet(append(BuiltinProtectedHosts(), hosts...))
	if err != nil {
		return err
	}
	l.protect = ps
	return nil
}

// imitated returns the protected host that written reads as and is not, or
// "". written is a host as it stands in the address.
func (l *Lists) imitated(written string) string {
	ps := defaultProtected
	if l != nil && l.protect != nil {
		ps = l.protect
	}
	if ps.plainOnly && isASCII(written) && !strings.Contains(written, "xn--") && !strings.Contains(written, "XN--") {
		return ""
	}
	host := writtenHost(written)
	folded := foldLatin(host)
	for rest := folded; ; {
		if names := ps.byFold[rest]; len(names) > 0 {
			own := false
			for _, name := range names {
				// The protected host itself, or a host under it.
				own = own || host == name || strings.HasSuffix(host, "."+name)
			}
			if !own {
				return names[0]
			}
		}
		dot := strings.IndexByte(rest, '.')
		if dot < 0 {
			return ""
		}
		rest = rest[dot+1:]
	}
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

// writtenHost returns a host as a browser reads it, for comparing two hosts:
// punycode labels decoded and letters in lower case. A dotted capital İ is
// left as it is; a browser makes "i" and a combining dot of it, which is
// another host than the one with a plain i.
func writtenHost(host string) string {
	if isASCII(host) {
		host = strings.ToLower(host)
		if !strings.Contains(host, "xn--") {
			return host
		}
	}
	labels := strings.Split(host, ".")
	for i, label := range labels {
		if len(label) > 4 && strings.EqualFold(label[:4], "xn--") {
			if decoded, ok := punycodeDecode(label[4:]); ok {
				label = decoded
			}
		}
		labels[i] = strings.Map(func(r rune) rune {
			if r == '\u0130' {
				return r
			}
			return unicode.ToLower(r)
		}, label)
	}
	return strings.Join(labels, ".")
}

// latinAlike are the Latin letters that have no accent to take off and are
// drawn like a plain letter all the same.
var latinAlike = map[rune]rune{
	'ı': 'i', 'ɨ': 'i', 'ɩ': 'i', 'ȷ': 'j',
	'ł': 'l', 'ŀ': 'l', 'ƚ': 'l',
	'đ': 'd', 'ɗ': 'd',
	'ø': 'o', 'ɵ': 'o',
	'ħ': 'h', 'ŧ': 't', 'ƶ': 'z', 'ĸ': 'k',
	'ɡ': 'g', 'ɑ': 'a',
}

// foldLatin reads a host the way the eye does: accents and other marks are
// dropped, and the letters of latinAlike become the plain letter they are
// drawn like. Only Latin is folded; letters of other scripts stay, and are
// the business of lookalikeHost.
func foldLatin(host string) string {
	if isASCII(host) {
		return strings.ToLower(host)
	}
	var b strings.Builder
	b.Grow(len(host))
	for _, r := range norm.NFD.String(host) {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		r = unicode.ToLower(r)
		if plain, ok := latinAlike[r]; ok {
			r = plain
		}
		b.WriteRune(r)
	}
	return b.String()
}

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
