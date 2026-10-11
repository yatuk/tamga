package upstream

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/yatuk/tamga/internal/egress"
	"github.com/yatuk/tamga/internal/policy"
)

// ErrEndpointRefused is the answer for a pool endpoint the rules do not let
// the proxy send to. Nothing was sent.
var ErrEndpointRefused = errors.New("upstream: pool endpoint is not allowed")

// Egress is where the endpoints of a policy-defined pool may be sent and how
// they are connected to. The policy can be edited through the API, so a pool
// endpoint is a destination someone other than the operator chose: its URL
// is judged by the rules before every request, and its address is judged by
// the egress guard when the connection is made.
//
// Only pool endpoints go through here. The proxy's ordinary routes keep the
// handler's own transport, which must reach a loopback model server and is
// pointed only by the operator.
type Egress struct {
	rules     *policy.UpstreamRules
	transport *http.Transport
	// public connects to public addresses only, whatever the operator
	// listed. An internal pool that does not set allow_private_addresses
	// uses it, so a listed name that resolves to a private address is
	// refused for that pool when it is dialled.
	public *http.Transport
}

// NewEgress builds the guarded transport for the given rules. Nil rules are
// the strict default: vendor hosts only, public addresses only.
func NewEgress(rules *policy.UpstreamRules) *Egress {
	return NewEgressWith(rules, nil, nil)
}

// NewEgressWith is NewEgress with the name lookup and the dial replaced, for
// tests. dial receives an address the guard has already judged.
func NewEgressWith(rules *policy.UpstreamRules, resolver egress.Resolver, dial func(ctx context.Context, network, address string) (net.Conn, error)) *Egress {
	cfg := egress.Config{AllowHosts: rules.DialHosts(), Resolver: resolver, Dial: dial}
	if rules != nil {
		cfg.AllowNets = rules.AllowNets
	}
	guard := egress.New(cfg)
	strict := egress.New(egress.Config{Resolver: resolver, Dial: dial})
	return &Egress{
		rules: rules,
		// A vendor name that resolves to an internal address is refused
		// here, whatever the URL looked like.
		transport: guardedTransport(guard),
		public:    guardedTransport(strict),
	}
}

// guardedTransport has the settings of the handler's streaming transport,
// with the guard's dialer.
func guardedTransport(guard *egress.Guard) *http.Transport {
	return &http.Transport{
		// No proxy: behind one the guard would judge the proxy's address
		// and the destination would be resolved out of sight.
		Proxy:                 nil,
		DialContext:           guard.DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   32,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		// Long-lived streams: do not abort while response headers are slow to arrive.
		ResponseHeaderTimeout: 0,
	}
}

// strictEgress serves a pool that was built without one.
var strictEgress = sync.OnceValue(func() *Egress { return NewEgress(nil) })

func (e *Egress) orStrict() *Egress {
	if e == nil {
		return strictEgress()
	}
	return e
}
