// Package egresstest holds the stand-ins tests use in place of DNS and the
// network when they exercise the egress guard. Nothing here reaches a real
// address.
package egresstest

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"testing"
)

// Resolver answers from a table. A name with several answers gives them in
// turn, one per lookup, and repeats the last: that is a rebinding DNS server.
// A name that is not in the table does not exist.
type Resolver struct {
	mu      sync.Mutex
	answers map[string][][]netip.Addr
	calls   map[string]int
}

func NewResolver() *Resolver {
	return &Resolver{answers: map[string][][]netip.Addr{}, calls: map[string]int{}}
}

// Add appends one answer for a name: the addresses of one lookup.
func (r *Resolver) Add(name string, ips ...string) *Resolver {
	addrs := make([]netip.Addr, len(ips))
	for i, ip := range ips {
		addrs[i] = netip.MustParseAddr(ip)
	}
	r.answers[name] = append(r.answers[name], addrs)
	return r
}

func (r *Resolver) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	key := strings.TrimSuffix(strings.ToLower(host), ".")
	r.mu.Lock()
	defer r.mu.Unlock()
	n := r.calls[key]
	r.calls[key]++
	as, ok := r.answers[key]
	if !ok {
		return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
	}
	if n >= len(as) {
		n = len(as) - 1
	}
	return as[n], nil
}

// Lookups is how many lookups were made in all.
func (r *Resolver) Lookups() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, c := range r.calls {
		n += c
	}
	return n
}

// ErrDialed is what DialRecorder returns for an address it has no route for.
var ErrDialed = errors.New("dial recorded, not made")

// DialRecorder stands where the network would be. It records every address
// the guard decided to connect to. An address in Routes is connected to the
// local listener it names; anything else fails with ErrDialed.
type DialRecorder struct {
	mu     sync.Mutex
	dialed []string
	Routes map[string]string
}

func (d *DialRecorder) Dial(ctx context.Context, network, address string) (net.Conn, error) {
	d.mu.Lock()
	d.dialed = append(d.dialed, address)
	to, ok := d.Routes[address]
	d.mu.Unlock()
	if !ok {
		return nil, ErrDialed
	}
	var nd net.Dialer
	return nd.DialContext(ctx, network, to)
}

// All is every address that was dialled, in order.
func (d *DialRecorder) All() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.dialed...)
}

// Request is what a Server was sent.
type Request struct {
	Path   string
	Method string
	Header http.Header
	Body   string
}

// Server is an httptest server that keeps what it was sent.
type Server struct {
	*httptest.Server
	mu   sync.Mutex
	reqs []Request
}

// NewServer starts a server that records each request and then calls h, or
// answers 200 when h is nil.
func NewServer(t *testing.T, h func(w http.ResponseWriter, r *http.Request)) *Server {
	t.Helper()
	s := &Server{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		s.reqs = append(s.reqs, Request{Path: r.URL.Path, Method: r.Method, Header: r.Header.Clone(), Body: string(b)})
		s.mu.Unlock()
		if h != nil {
			h(w, r)
		}
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *Server) All() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Request(nil), s.reqs...)
}

// Port is the port the server listens on, on 127.0.0.1.
func (s *Server) Port() string {
	u, _ := url.Parse(s.URL)
	return u.Port()
}

// Addr is the server's own address, for DialRecorder.Routes.
func (s *Server) Addr() string { return s.Listener.Addr().String() }
