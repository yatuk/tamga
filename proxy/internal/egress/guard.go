// Package egress guards outbound HTTP requests whose destination comes from
// configuration or an API: it decides which addresses may be connected to,
// and makes that decision when the connection is made.
package egress

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"syscall"
	"time"
)

// ErrNotAllowed is the one answer for a request the guard refused
// because of where it would go. It says nothing about why (a literal, what a
// name resolved to, a redirect), so the probe cannot be used to map a network.
var ErrNotAllowed = errors.New("destination address is not allowed")

// ErrInsecureScheme is the answer for a plain http URL whose host is not on
// the allowlist. The caller typed the scheme, so naming it gives nothing away.
var ErrInsecureScheme = errors.New("destination must use https")

// ErrBadURL is a URL that cannot be requested at all; ErrTooManyRedirects is
// a chain longer than MaxRedirects.
var (
	ErrBadURL           = errors.New("url is not valid")
	ErrTooManyRedirects = errors.New("too many redirects")
)

const (
	// MaxRedirects is how many redirects one request follows. Receivers
	// redirect to add a slash or to move to a regional host; nothing honest
	// needs more than a few.
	MaxRedirects = 3
	// connectTimeout bounds one connection attempt, so a name with several
	// addresses gets to try more than the first within one delivery attempt.
	connectTimeout = 3 * time.Second
	// probeTimeout bounds a request whose caller set no deadline.
	probeTimeout = 5 * time.Second
	// headerTimeout bounds the wait for a receiver's answer once the request
	// is written, and maxHeaderBytes and maxBodyBytes what is read of it. A
	// receiver's answer is a status line; nothing here streams.
	headerTimeout  = 5 * time.Second
	maxHeaderBytes = 64 << 10
	maxBodyBytes   = 64 << 10
)

// Resolver is the part of net.Resolver the guard uses. Tests replace it.
type Resolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

// Config says what a Guard lets through beyond public addresses. The
// zero value is the strict default: nothing internal, https only.
type Config struct {
	// AllowNets are address ranges that may be connected to although they
	// are internal. A URL whose host is an IP literal inside one may also
	// use plain http.
	AllowNets []netip.Prefix
	// AllowHosts are exact host names that may be connected to whatever
	// they resolve to, and may use plain http.
	AllowHosts []string
	// Resolver looks names up. Nil is the system resolver.
	Resolver Resolver
	// Dial connects to an address the guard has already judged; address is
	// always an IP literal and a port. Nil is a net.Dialer.
	Dial func(ctx context.Context, network, address string) (net.Conn, error)
}

// Guard is the only way out of this package: every request to a saved
// destination or a rule's notify URL goes through its client.
//
// The address is judged when the connection is made, not when the URL is
// saved. A name can resolve differently later and a redirect can lead
// anywhere, so the dialer resolves the name itself, judges every address it
// got and connects to the literal it judged. Each redirect hop dials through
// the same dialer.
type Guard struct {
	allowNets  []netip.Prefix
	allowHosts map[string]struct{}
	resolver   Resolver
	dial       func(ctx context.Context, network, address string) (net.Conn, error)
	client     *http.Client
}

// New builds a guard and its HTTP client.
func New(cfg Config) *Guard {
	g := &Guard{
		allowHosts: make(map[string]struct{}, len(cfg.AllowHosts)),
		resolver:   cfg.Resolver,
		dial:       cfg.Dial,
	}
	for _, p := range cfg.AllowNets {
		if p.IsValid() {
			g.allowNets = append(g.allowNets, netip.PrefixFrom(p.Addr().Unmap(), unmappedBits(p)).Masked())
		}
	}
	for _, h := range cfg.AllowHosts {
		if h = normalHost(h); h != "" {
			g.allowHosts[h] = struct{}{}
		}
	}
	if g.resolver == nil {
		g.resolver = net.DefaultResolver
	}
	g.client = &http.Client{
		Transport: &http.Transport{
			// No proxy, whatever HTTP_PROXY and HTTPS_PROXY say: behind one,
			// the address connected to would be the proxy's and the
			// destination would be resolved out of sight, so the guard would
			// judge the wrong thing.
			Proxy:                  nil,
			DialContext:            g.DialContext,
			ForceAttemptHTTP2:      true,
			MaxIdleConns:           32,
			MaxIdleConnsPerHost:    2,
			IdleConnTimeout:        90 * time.Second,
			TLSHandshakeTimeout:    5 * time.Second,
			ExpectContinueTimeout:  time.Second,
			ResponseHeaderTimeout:  headerTimeout,
			MaxResponseHeaderBytes: maxHeaderBytes,
		},
		CheckRedirect: g.checkRedirect,
	}
	return g
}

// Client is the guarded HTTP client, for a caller that builds its own
// requests. Every connection it makes, redirects included, is judged by
// DialContext. It does not apply CheckURL to the first URL: call that, or
// use Post.
func (g *Guard) Client() *http.Client { return g.client }

// unmappedBits is the prefix length of p once an IPv4-mapped address has been
// turned into plain IPv4.
func unmappedBits(p netip.Prefix) int {
	if p.Addr().Is4In6() && p.Bits() >= 96 {
		return p.Bits() - 96
	}
	return p.Bits()
}

// normalHost is a host name as the guard compares it: lower case, without
// the trailing dot of a fully qualified name.
func normalHost(h string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(h)), ".")
}

// Ranges that are refused unless allowlisted. netip's own predicates cover
// loopback, private, link-local and multicast; this table is the rest, and
// repeats those so the whole list can be read in one place.
var blockedNets = mustPrefixes(
	"0.0.0.0/8",       // "this network", and the unspecified address
	"10.0.0.0/8",      // private
	"100.64.0.0/10",   // carrier-grade NAT
	"127.0.0.0/8",     // loopback
	"169.254.0.0/16",  // link-local, cloud metadata
	"172.16.0.0/12",   // private
	"192.0.0.0/24",    // IETF protocol assignments
	"192.0.2.0/24",    // documentation
	"192.88.99.0/24",  // 6to4 relay anycast, retired
	"192.168.0.0/16",  // private
	"198.18.0.0/15",   // benchmarking
	"198.51.100.0/24", // documentation
	"203.0.113.0/24",  // documentation
	"224.0.0.0/4",     // multicast
	"240.0.0.0/4",     // reserved, and broadcast
	"::/96",           // unspecified, loopback, IPv4-compatible
	"::ffff:0:0:0/96", // IPv4-translated
	"64:ff9b:1::/48",  // local-use NAT64
	"100::/64",        // discard
	"2001::/23",       // IETF protocol assignments, Teredo among them
	"2001:db8::/32",   // documentation
	"3fff::/20",       // documentation
	"fc00::/7",        // unique local
	"fe80::/10",       // link-local
	"fec0::/10",       // site-local, retired
	"ff00::/8",        // multicast
)

// Prefixes whose addresses carry an IPv4 address that is the real target.
var (
	nat64Net  = netip.MustParsePrefix("64:ff9b::/96")
	sixToFour = netip.MustParsePrefix("2002::/16")
)

func mustPrefixes(ss ...string) []netip.Prefix {
	out := make([]netip.Prefix, len(ss))
	for i, s := range ss {
		out[i] = netip.MustParsePrefix(s)
	}
	return out
}

// BlockedAddr reports whether an address is internal or reserved. The
// IPv4-mapped form is judged as the IPv4 address it stands for, and so are
// the NAT64 and 6to4 forms.
func BlockedAddr(a netip.Addr) bool {
	if !a.IsValid() {
		return true
	}
	a = a.WithZone("").Unmap()
	if a.IsUnspecified() || a.IsLoopback() || a.IsPrivate() || a.IsLinkLocalUnicast() ||
		a.IsMulticast() || a.IsLinkLocalMulticast() || a.IsInterfaceLocalMulticast() {
		return true
	}
	for _, p := range blockedNets {
		if p.Contains(a) {
			return true
		}
	}
	if a.Is6() {
		b := a.As16()
		switch {
		case nat64Net.Contains(a):
			return BlockedAddr(netip.AddrFrom4([4]byte{b[12], b[13], b[14], b[15]}))
		case sixToFour.Contains(a):
			return BlockedAddr(netip.AddrFrom4([4]byte{b[2], b[3], b[4], b[5]}))
		}
	}
	return false
}

// AddrAllowed is the judgement of one address: public, or inside a range the
// operator listed.
func (g *Guard) AddrAllowed(a netip.Addr) bool {
	if !a.IsValid() {
		return false
	}
	a = a.WithZone("").Unmap()
	for _, p := range g.allowNets {
		if p.Contains(a) {
			return true
		}
	}
	return !BlockedAddr(a)
}

func (g *Guard) addrListed(a netip.Addr) bool {
	a = a.WithZone("").Unmap()
	for _, p := range g.allowNets {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

func (g *Guard) hostListed(host string) bool {
	_, ok := g.allowHosts[normalHost(host)]
	return ok
}

// refusal is why a URL was turned down, with a sentence for whoever saved it.
type refusal struct {
	kind error
	msg  string
}

func (e *refusal) Error() string { return e.msg }
func (e *refusal) Unwrap() error { return e.kind }

func refuse(kind error, msg string) error { return &refusal{kind: kind, msg: msg} }

// CheckURL is the cheap check made when a destination is saved: the URL
// parses, the scheme is https (or http for an allowlisted host), and a host
// written as an IP literal is not internal. A host name is not looked up
// here: an answer now proves nothing about the answer later, and looking it
// up would tell the caller what resolves inside the network.
func (g *Guard) CheckURL(raw string) error {
	_, err := g.judgeURL(raw)
	return err
}

func (g *Guard) judgeURL(raw string) (*url.URL, error) {
	if raw == "" {
		return nil, refuse(ErrBadURL, "url required")
	}
	for _, r := range raw {
		if r <= ' ' || r == 0x7f {
			return nil, refuse(ErrBadURL, "url must not contain spaces or control characters")
		}
	}
	u, err := url.Parse(raw)
	if err != nil {
		// The parser's own message repeats the URL, which is a credential.
		return nil, refuse(ErrBadURL, "url is not valid")
	}
	if err := g.judgeParsed(u); err != nil {
		return nil, err
	}
	return u, nil
}

func (g *Guard) judgeParsed(u *url.URL) error {
	if u.Scheme != "http" && u.Scheme != "https" {
		return refuse(ErrBadURL, "url scheme must be https")
	}
	host := normalHost(u.Hostname())
	if host == "" || u.Opaque != "" {
		return refuse(ErrBadURL, "url has no host")
	}
	listed := false
	if ip, err := netip.ParseAddr(host); err == nil {
		if !g.AddrAllowed(ip) {
			return refuse(ErrNotAllowed, "url points to an internal or reserved address; an operator can allow it with TAMGA_ALERT_ALLOW_PRIVATE")
		}
		listed = g.addrListed(ip)
	} else {
		listed = g.hostListed(host)
		switch {
		case listed:
		case endsInNumber(host):
			// "2130706433", "0x7f.1", "0177.0.0.1", "127.1": not IP literals
			// to Go, but some resolvers read them as addresses.
			return refuse(ErrBadURL, "url host is not a valid host name or IP address")
		case host == "localhost" || strings.HasSuffix(host, ".localhost"):
			return refuse(ErrNotAllowed, "url points to an internal or reserved address; an operator can allow it with TAMGA_ALERT_ALLOW_PRIVATE")
		case strings.Contains(host, "..") || strings.HasPrefix(host, "."):
			return refuse(ErrBadURL, "url host is not a valid host name or IP address")
		}
	}
	if u.Scheme == "http" && !listed {
		return refuse(ErrInsecureScheme, "url must use https; plain http is accepted only for hosts an operator listed in TAMGA_ALERT_ALLOW_PRIVATE")
	}
	return nil
}

// endsInNumber reports whether the last label of a host name is a decimal,
// octal or hex number. Such a name is an IPv4 address in some spelling and
// never a real domain: no top-level domain is a number.
func endsInNumber(host string) bool {
	last := host[strings.LastIndexByte(host, '.')+1:]
	if last == "" {
		return false
	}
	if strings.HasPrefix(last, "0x") {
		for _, c := range last[2:] {
			if !strings.ContainsRune("0123456789abcdef", c) {
				return false
			}
		}
		return true
	}
	for _, c := range last {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// DialContext is where the decision is made. It resolves the host, judges
// every address the lookup returned and connects to an address it judged,
// written as a literal, so no second lookup can come between the judgement
// and the connection.
//
// A name with any address that is not allowed is refused outright rather
// than connected to at one of its good addresses: a name that answers with
// both public and internal addresses is what a rebinding attempt looks like,
// and an operator whose receiver really is like that can list the name.
func (g *Guard) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, ErrNotAllowed
	}
	exempt := g.hostListed(host)
	var addrs []netip.Addr
	if ip, err := netip.ParseAddr(normalHost(host)); err == nil {
		addrs = []netip.Addr{ip}
	} else {
		addrs, err = g.resolver.LookupNetIP(ctx, "ip", host)
		if err != nil {
			return nil, err
		}
		if len(addrs) == 0 {
			return nil, &net.DNSError{Err: "no such host", Name: "destination", IsNotFound: true}
		}
	}
	if !exempt {
		for _, a := range addrs {
			if !g.AddrAllowed(a) {
				return nil, ErrNotAllowed
			}
		}
	}
	dial := g.dial
	if dial == nil {
		d := &net.Dialer{Timeout: connectTimeout, KeepAlive: 30 * time.Second, Control: g.control(exempt)}
		dial = d.DialContext
	}
	var lastErr error
	for _, a := range addrs {
		cctx, cancel := context.WithTimeout(ctx, connectTimeout)
		conn, err := dial(cctx, network, net.JoinHostPort(a.WithZone("").Unmap().String(), port))
		cancel()
		if err == nil {
			return conn, nil
		}
		lastErr = err
		if ctx.Err() != nil {
			break
		}
	}
	return nil, lastErr
}

// control runs on the socket just before it connects, with the address the
// kernel is about to be given. It repeats the judgement there, so that
// nothing between DialContext and the system call can change the outcome.
func (g *Guard) control(exempt bool) func(network, address string, _ syscall.RawConn) error {
	return func(_, address string, _ syscall.RawConn) error {
		if exempt {
			return nil
		}
		ap, err := netip.ParseAddrPort(address)
		if err != nil || !g.AddrAllowed(ap.Addr()) {
			return ErrNotAllowed
		}
		return nil
	}
}

// checkRedirect applies the URL rules to every hop (the dialer applies the
// address rules) and keeps a destination's headers on its own host.
func (g *Guard) checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) > MaxRedirects {
		return ErrTooManyRedirects
	}
	if err := g.judgeParsed(req.URL); err != nil {
		// One answer whatever the reason: where a receiver redirects to is
		// not the caller's to learn.
		return ErrNotAllowed
	}
	// The Referer would be the URL that was redirected from, and a webhook
	// URL is a credential.
	req.Header.Del("Referer")
	// A redirect stays on the host the operator named. The request is theirs
	// to that host: its headers carry an Authorization or an API key, and
	// its body can carry one too (a PagerDuty routing key) along with the
	// alert itself. Stripping headers would still hand the body to a host
	// nobody chose, so a hop to another host is refused.
	if normalHost(req.URL.Hostname()) != normalHost(via[0].URL.Hostname()) {
		return ErrNotAllowed
	}
	// A 301, 302 or 303 turns a POST into a GET without its body. Following
	// it would report an alert as sent that never arrived.
	if req.Method != via[0].Method {
		return ErrNotAllowed
	}
	return nil
}

// Post sends one request through the guard and returns the status of the
// answer. A caller's deadline is kept; a request without one is given
// probeTimeout. The answer's body is read so the connection can be reused,
// and thrown away: what a receiver says is not ours to keep.
func (g *Guard) Post(ctx context.Context, rawURL string, header http.Header, body []byte) (int, error) {
	if _, err := g.judgeURL(rawURL); err != nil {
		return 0, err
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, probeTimeout)
		defer cancel()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, bytes.NewReader(body))
	if err != nil {
		return 0, refuse(ErrBadURL, "url is not valid")
	}
	req.Header = header.Clone()
	if req.Header == nil {
		req.Header = http.Header{}
	}
	resp, err := g.client.Do(req)
	if err != nil {
		return 0, err
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxBodyBytes))
	_ = resp.Body.Close()
	return resp.StatusCode, nil
}

// Class names what went wrong with a request in one short word: blocked,
// bad_url, redirects, canceled, timeout, dns, tls or network. The error text
// itself is never to be kept or shown: it contains the URL, the address and
// sometimes what the peer sent.
func Class(err error) string {
	var netErr net.Error
	var dnsErr *net.DNSError
	var certErr *tls.CertificateVerificationError
	switch {
	case errors.Is(err, ErrNotAllowed), errors.Is(err, ErrInsecureScheme):
		return "blocked"
	case errors.Is(err, ErrBadURL):
		return "bad_url"
	case errors.Is(err, ErrTooManyRedirects):
		return "redirects"
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.As(err, &dnsErr):
		return "dns"
	case errors.As(err, &certErr):
		return "tls"
	case errors.As(err, &netErr) && netErr.Timeout():
		return "timeout"
	}
	return "network"
}
