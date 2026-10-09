package egress

import (
	"context"
	"errors"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"testing"

	"github.com/yatuk/tamga/internal/egress/egresstest"
)

func TestGuard_AddressJudgement(t *testing.T) {
	cases := []struct {
		addr    string
		blocked bool
		why     string
	}{
		{"127.0.0.1", true, "loopback"},
		{"127.255.255.254", true, "loopback"},
		{"::1", true, "loopback"},
		{"10.0.0.1", true, "private"},
		{"10.255.255.255", true, "private"},
		{"172.16.0.1", true, "private"},
		{"172.31.255.255", true, "private"},
		{"192.168.0.1", true, "private"},
		{"fc00::1", true, "unique local"},
		{"fdff:ffff::1", true, "unique local"},
		{"169.254.169.254", true, "metadata"},
		{"169.254.0.1", true, "link-local"},
		{"fe80::1", true, "link-local"},
		{"fe80::1%eth0", true, "link-local with a zone"},
		{"febf::1", true, "link-local"},
		{"100.64.0.1", true, "CGNAT"},
		{"100.127.255.255", true, "CGNAT"},
		{"0.0.0.0", true, "unspecified"},
		{"0.1.2.3", true, "this network"},
		{"::", true, "unspecified"},
		{"224.0.0.1", true, "multicast"},
		{"239.255.255.255", true, "multicast"},
		{"ff02::1", true, "multicast"},
		{"ff0e::1", true, "multicast"},
		{"255.255.255.255", true, "broadcast"},
		{"240.0.0.1", true, "reserved"},
		{"192.0.2.1", true, "documentation"},
		{"198.51.100.7", true, "documentation"},
		{"203.0.113.9", true, "documentation"},
		{"2001:db8::1", true, "documentation"},
		{"198.18.0.1", true, "benchmarking"},
		{"192.0.0.1", true, "protocol assignments"},
		{"fec0::1", true, "site-local"},
		{"::ffff:127.0.0.1", true, "mapped loopback"},
		{"::ffff:7f00:1", true, "mapped loopback, hex"},
		{"::ffff:10.0.0.1", true, "mapped private"},
		{"::ffff:192.168.1.1", true, "mapped private"},
		{"::ffff:169.254.169.254", true, "mapped metadata"},
		{"::ffff:100.64.0.1", true, "mapped CGNAT"},
		{"::ffff:0.0.0.0", true, "mapped unspecified"},
		{"::ffff:224.0.0.1", true, "mapped multicast"},
		{"::ffff:255.255.255.255", true, "mapped broadcast"},
		{"::127.0.0.1", true, "IPv4-compatible loopback"},
		{"::10.0.0.1", true, "IPv4-compatible private"},
		{"::8.8.8.8", true, "IPv4-compatible, retired form"},
		{"64:ff9b::7f00:1", true, "NAT64 of loopback"},
		{"64:ff9b::a9fe:a9fe", true, "NAT64 of metadata"},
		{"64:ff9b::a00:1", true, "NAT64 of private"},
		{"64:ff9b:1::a00:1", true, "local-use NAT64"},
		{"2002:7f00:1::1", true, "6to4 of loopback"},
		{"2002:a00:1::1", true, "6to4 of private"},
		{"2002:c0a8:101::1", true, "6to4 of private"},
		{"2001:0:4136:e378:8000:63bf:3fff:fdd2", true, "Teredo"},
		{"100::1", true, "discard"},

		{"1.1.1.1", false, "public"},
		{"8.8.8.8", false, "public"},
		{"93.184.216.34", false, "public"},
		{"172.15.255.255", false, "just below private"},
		{"172.32.0.1", false, "just above private"},
		{"100.63.255.255", false, "just below CGNAT"},
		{"100.128.0.1", false, "just above CGNAT"},
		{"169.253.255.255", false, "just below link-local"},
		{"2606:4700:4700::1111", false, "public"},
		{"::ffff:8.8.8.8", false, "mapped public"},
		{"64:ff9b::808:808", false, "NAT64 of public"},
		{"2002:808:808::1", false, "6to4 of public"},
	}
	g := New(Config{})
	for _, c := range cases {
		a := netip.MustParseAddr(c.addr)
		if got := BlockedAddr(a); got != c.blocked {
			t.Errorf("%s (%s): blocked = %v, want %v", c.addr, c.why, got, c.blocked)
		}
		if got := g.AddrAllowed(a); got == c.blocked {
			t.Errorf("%s (%s): the strict guard allows = %v", c.addr, c.why, got)
		}
	}
	if !BlockedAddr(netip.Addr{}) || g.AddrAllowed(netip.Addr{}) {
		t.Error("the zero address must be refused")
	}
}

// What happens to a URL when it is saved and when it is dialled. The dial
// column goes through the guard's HTTP client without the save-time check,
// with a resolver that reads odd spellings the way a permissive system
// resolver does: as the address they spell.
func TestGuard_URLSpellings(t *testing.T) {
	const (
		ok      = "ok"
		bad     = "bad_url"
		blocked = "blocked"
		none    = ""
	)
	res := egresstest.NewResolver().
		Add("2130706433", "127.0.0.1").
		Add("0x7f.1", "127.0.0.1").
		Add("0x7f000001", "127.0.0.1").
		Add("0177.0.0.1", "127.0.0.1").
		Add("017700000001", "127.0.0.1").
		Add("127.1", "127.0.0.1").
		Add("10.1", "10.0.0.1").
		Add("localhost", "127.0.0.1", "::1").
		Add("app.localhost", "127.0.0.1").
		Add("internal.example", "10.0.0.5").
		Add("metadata.example", "169.254.169.254").
		Add("mapped.example", "::ffff:10.0.0.5").
		Add("example.com", "93.184.216.34")
	cases := []struct {
		url  string
		save string
		// dial is what the dialer does: blocked, ok (it connects, and to
		// wantDial), or none when the URL cannot be requested at all.
		dial     string
		wantDial string
	}{
		// Internal addresses as literals.
		{"https://127.0.0.1/", blocked, blocked, ""},
		{"https://127.0.0.1:8443/hook?x=1", blocked, blocked, ""},
		{"https://10.0.0.1/", blocked, blocked, ""},
		{"https://172.16.0.1/", blocked, blocked, ""},
		{"https://192.168.1.1/", blocked, blocked, ""},
		{"https://169.254.169.254/latest/meta-data/", blocked, blocked, ""},
		{"https://100.64.0.1/", blocked, blocked, ""},
		{"https://0.0.0.0/", blocked, blocked, ""},
		{"https://255.255.255.255/", blocked, blocked, ""},
		{"https://224.0.0.1/", blocked, blocked, ""},
		{"https://192.0.2.1/", blocked, blocked, ""},
		{"https://[::1]/", blocked, blocked, ""},
		{"https://[::]/", blocked, blocked, ""},
		{"https://[fc00::1]/", blocked, blocked, ""},
		{"https://[fe80::1]/", blocked, blocked, ""},
		{"https://[fe80::1%25eth0]/", blocked, blocked, ""},
		{"https://[ff02::1]/", blocked, blocked, ""},
		{"https://[2001:db8::1]/", blocked, blocked, ""},
		{"https://0.1.2.3/", blocked, blocked, ""},
		{"http://169.254.169.254/", blocked, blocked, ""},
		{"http://127.0.0.1/", blocked, blocked, ""},
		{"http://[::1]/", blocked, blocked, ""},
		{"http://100.64.0.1/", blocked, blocked, ""},
		{"http://allowed@127.0.0.1/", blocked, blocked, ""},
		{"https://[2002:7f00:1::]/", blocked, blocked, ""},
		{"https://[64:ff9b:1::7f00:1]/", blocked, blocked, ""},
		{"https://[2001:0:4136:e378:8000:63bf:3fff:fdd2]/", blocked, blocked, ""},
		// IPv4 inside IPv6.
		{"https://[::ffff:127.0.0.1]/", blocked, blocked, ""},
		{"https://[::ffff:7f00:1]/", blocked, blocked, ""},
		{"https://[::ffff:a9fe:a9fe]/", blocked, blocked, ""},
		{"https://[::ffff:10.0.0.1]/", blocked, blocked, ""},
		{"https://[::127.0.0.1]/", blocked, blocked, ""},
		{"https://[64:ff9b::7f00:1]/", blocked, blocked, ""},
		{"https://[2002:a00:1::1]/", blocked, blocked, ""},
		{"https://[0:0:0:0:0:ffff:7f00:1]/", blocked, blocked, ""},
		// Spellings Go takes for host names. The save-time check turns them
		// down as malformed; the dialer judges what they resolve to.
		{"https://2130706433/", bad, blocked, ""},
		{"https://0x7f.1/", bad, blocked, ""},
		{"https://0x7f000001/", bad, blocked, ""},
		{"https://0177.0.0.1/", bad, blocked, ""},
		{"https://017700000001/", bad, blocked, ""},
		{"https://127.1/", bad, blocked, ""},
		{"https://10.1/", bad, blocked, ""},
		{"http://2130706433/", bad, blocked, ""},
		{"http://0x7f.1/", bad, blocked, ""},
		{"http://0177.0.0.1/", bad, blocked, ""},
		{"http://127.1/", bad, blocked, ""},
		// Trailing dots, case, userinfo, fragments.
		{"https://127.0.0.1./", blocked, blocked, ""},
		{"https://localhost/", blocked, blocked, ""},
		{"https://localhost./", blocked, blocked, ""},
		{"https://LOCALHOST/", blocked, blocked, ""},
		{"https://app.localhost/", blocked, blocked, ""},
		{"HTTPS://127.0.0.1/", blocked, blocked, ""},
		{"https://x@127.0.0.1/", blocked, blocked, ""},
		{"https://user:pass@10.0.0.1/", blocked, blocked, ""},
		{"https://example.com@127.0.0.1/", blocked, blocked, ""},
		{"https://127.0.0.1/#frag", blocked, blocked, ""},
		{"https://127.0.0.1#@example.com/", blocked, blocked, ""},
		{"http://x@127.0.0.1/", blocked, blocked, ""},
		{"http://[::ffff:7f00:1]/", blocked, blocked, ""},
		// Unicode look-alikes that the HTTP client folds to ASCII before it
		// dials: a circled "l", circled digits, ideographic full stops.
		{"https://ⓛocalhost/", ok, blocked, ""},
		{"https://①②⑦.⓪.⓪.①/", ok, blocked, ""},
		{"https://127。0。0。1/", ok, blocked, ""},
		// Names are not looked up when saved; the dialer catches them.
		{"https://internal.example/", ok, blocked, ""},
		{"https://INTERNAL.EXAMPLE./", ok, blocked, ""},
		{"https://metadata.example/", ok, blocked, ""},
		{"https://mapped.example/", ok, blocked, ""},
		// Whitespace and control characters.
		{" https://example.com/", bad, none, ""},
		{"https://example.com/ ", bad, none, ""},
		{"https://exa mple.com/", bad, none, ""},
		{"https://example.com/\n", bad, none, ""},
		{"\thttps://127.0.0.1/", bad, none, ""},
		{"https://example.com/a b", bad, none, ""},
		// Schemes and shapes.
		{"ftp://example.com/", bad, none, ""},
		{"file:///etc/passwd", bad, none, ""},
		{"gopher://127.0.0.1:6379/_x", bad, none, ""},
		{"javascript:alert(1)", bad, none, ""},
		{"//example.com/hook", bad, none, ""},
		{"example.com/hook", bad, none, ""},
		{"https:example.com", bad, none, ""},
		{"https://", bad, none, ""},
		{"https://:443/", bad, none, ""},
		{"https://.example.com/", bad, none, ""},
		{"https://exa..mple.com/", bad, none, ""},
		{"://invalid", bad, none, ""},
		{"", bad, none, ""},
		// Plain http to a public host: refused for the scheme.
		{"http://example.com/hook", blocked, none, ""},
		{"http://93.184.216.34/hook", blocked, none, ""},
		// What must keep working.
		{"https://example.com/services/T0/B0/x", ok, ok, "93.184.216.34:443"},
		{"https://example.com:8443/hook", ok, ok, "93.184.216.34:8443"},
		{"https://EXAMPLE.com./hook#frag", ok, ok, "93.184.216.34:443"},
		{"https://user:pass@example.com/hook", ok, ok, "93.184.216.34:443"},
		{"https://93.184.216.34/hook", ok, ok, "93.184.216.34:443"},
		{"https://[2606:4700:4700::1111]/hook", ok, ok, "[2606:4700:4700::1111]:443"},
		{"https://[::ffff:93.184.216.34]:9443/", ok, ok, "93.184.216.34:9443"},
	}
	for _, c := range cases {
		rec := &egresstest.DialRecorder{}
		g := New(Config{Resolver: res, Dial: rec.Dial})

		got := ok
		if err := g.CheckURL(c.url); err != nil {
			got = Class(err)
			if strings.Contains(err.Error(), "://") {
				t.Errorf("%q: the save-time error repeats the URL: %v", c.url, err)
			}
		}
		if got != c.save {
			t.Errorf("%q at save time: %s, want %s", c.url, got, c.save)
		}

		if c.dial == none {
			continue
		}
		req, err := http.NewRequest(http.MethodPost, c.url, strings.NewReader("{}"))
		if err != nil {
			t.Errorf("%q: cannot build the request to dial with: %v", c.url, err)
			continue
		}
		_, err = g.client.Do(req)
		switch c.dial {
		case blocked:
			if !errors.Is(err, ErrNotAllowed) {
				t.Errorf("%q at dial time: %v, want the guard's refusal", c.url, err)
			}
			if d := rec.All(); len(d) != 0 {
				t.Errorf("%q: a connection was attempted to %v", c.url, d)
			}
		case ok:
			if !errors.Is(err, egresstest.ErrDialed) {
				t.Errorf("%q at dial time: %v, want a connection attempt", c.url, err)
			}
			if d := rec.All(); len(d) != 1 || d[0] != c.wantDial {
				t.Errorf("%q: dialled %v, want %s", c.url, d, c.wantDial)
			}
		}
	}
}

// A name with a public and an internal address is refused whole, in either
// order: nothing is dialled.
func TestGuard_MixedAnswersAreRefused(t *testing.T) {
	for _, ips := range [][]string{
		{"93.184.216.34", "10.0.0.5"},
		{"10.0.0.5", "93.184.216.34"},
		{"93.184.216.34", "2606:4700:4700::1111", "::ffff:127.0.0.1"},
		{"93.184.216.34", "fe80::1"},
	} {
		rec := &egresstest.DialRecorder{}
		g := New(Config{Resolver: egresstest.NewResolver().Add("mixed.example", ips...), Dial: rec.Dial})
		_, err := g.Post(context.Background(), "https://mixed.example/hook", nil, nil)
		if !errors.Is(err, ErrNotAllowed) {
			t.Errorf("%v: %v, want the guard's refusal", ips, err)
		}
		if d := rec.All(); len(d) != 0 {
			t.Errorf("%v: dialled %v", ips, d)
		}
	}

	// All public: each is tried in turn, as a literal.
	rec := &egresstest.DialRecorder{}
	g := New(Config{Resolver: egresstest.NewResolver().Add("two.example", "93.184.216.34", "2606:4700:4700::1111"), Dial: rec.Dial})
	if _, err := g.Post(context.Background(), "https://two.example:8443/hook", nil, nil); !errors.Is(err, egresstest.ErrDialed) {
		t.Fatalf("all public: %v", err)
	}
	if d := rec.All(); len(d) != 2 || d[0] != "93.184.216.34:8443" || d[1] != "[2606:4700:4700::1111]:8443" {
		t.Errorf("all public: dialled %v", d)
	}
}

// DNS rebinding: the name is public when first looked up and internal the
// next time. Saving looks nothing up. Each connection is judged on the
// answer it will itself connect to, and connects to that answer as a
// literal, so there is no second lookup for the name to change under.
func TestGuard_Rebinding(t *testing.T) {
	res := egresstest.NewResolver().Add("rebind.example", "93.184.216.34").Add("rebind.example", "127.0.0.1")
	rec := &egresstest.DialRecorder{}
	g := New(Config{Resolver: res, Dial: rec.Dial})

	if err := g.CheckURL("https://rebind.example/hook"); err != nil {
		t.Fatalf("save: %v", err)
	}
	if n := res.Lookups(); n != 0 {
		t.Fatalf("saving made %d lookups, want none", n)
	}

	_, err := g.Post(context.Background(), "https://rebind.example/hook", nil, nil)
	if !errors.Is(err, egresstest.ErrDialed) {
		t.Fatalf("first request: %v, want a connection attempt", err)
	}
	if d := rec.All(); len(d) != 1 || d[0] != "93.184.216.34:443" {
		t.Fatalf("first request dialled %v, want the literal it judged", d)
	}
	if n := res.Lookups(); n != 1 {
		t.Fatalf("first request made %d lookups, want exactly one", n)
	}

	_, err = g.Post(context.Background(), "https://rebind.example/hook", nil, nil)
	if !errors.Is(err, ErrNotAllowed) {
		t.Fatalf("second request: %v, want the guard's refusal", err)
	}
	if d := rec.All(); len(d) != 1 {
		t.Fatalf("the rebound address was dialled: %v", d)
	}
}

func TestGuard_RedirectToBlockedIsRefused(t *testing.T) {
	// A listener on loopback that a redirect must never reach.
	victim := egresstest.NewServer(t, nil)
	var target string
	origin := egresstest.NewServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target, http.StatusTemporaryRedirect)
	})
	res := egresstest.NewResolver().
		Add("front.test", "127.0.0.1").
		Add("internal.example", "10.0.0.9").
		Add("mixed.example", "93.184.216.34", "192.168.1.1").
		Add("public.example", "93.184.216.34").
		Add("localhost", "127.0.0.1").
		Add("2130706433", "127.0.0.1")
	header := http.Header{}
	header.Set("Authorization", "Bearer SECRET")
	header.Set("X-Api-Key", "SECRET")
	for _, to := range []string{
		"http://127.0.0.1:" + victim.Port() + "/x",  // loopback, as a literal
		"https://127.0.0.1:" + victim.Port() + "/x", // and over https
		"http://[::1]:" + victim.Port() + "/x",      //
		"https://10.0.0.9/admin",                    // a literal: refused by the URL check
		"https://169.254.169.254/latest/",           // metadata
		"https://[::ffff:127.0.0.1]/",               // mapped loopback
		"https://internal.example/admin",            // a name: refused by the dialer
		"https://mixed.example/",                    // a name with one internal answer
		"http://public.example/hook",                // down to plain http
		"https://2130706433:" + victim.Port() + "/", // an odd spelling
		"ftp://public.example/",                     // another scheme
		"https://localhost:" + victim.Port(),        // loopback by name
	} {
		target = to
		// The origin is reached by a name the operator allowed. Nothing
		// else is allowed, and the recorder would let only that through.
		rec := &egresstest.DialRecorder{Routes: map[string]string{"127.0.0.1:" + origin.Port(): origin.Addr()}}
		g := New(Config{AllowHosts: []string{"front.test"}, Resolver: res, Dial: rec.Dial})
		before := len(origin.All())
		_, err := g.Post(context.Background(), "http://front.test:"+origin.Port()+"/hook", header, []byte("{}"))
		if !errors.Is(err, ErrNotAllowed) || Class(err) != "blocked" {
			t.Errorf("redirect to %s: %v (%s), want the guard's refusal", to, err, Class(err))
		}
		var ue *url.Error
		if errors.As(err, &ue) && ue.Err != ErrNotAllowed {
			t.Errorf("redirect to %s: the error says more than the fixed outcome: %v", to, ue.Err)
		}
		if n := len(origin.All()) - before; n != 1 {
			t.Errorf("redirect to %s: the origin saw %d requests, want 1", to, n)
		}
		for _, d := range rec.All() {
			if d != "127.0.0.1:"+origin.Port() {
				t.Errorf("redirect to %s: dialled %s", to, d)
			}
		}
	}
	if n := len(victim.All()); n != 0 {
		t.Errorf("the loopback listener got %d requests", n)
	}
}

// A redirect stays on the host the operator named. Within the host the
// request goes along whole, without a Referer, which would be the webhook's
// own URL. A hop to another host is refused before anything is sent there:
// the body is the alert, and can hold a key of its own.
func TestGuard_RedirectStaysOnItsHost(t *testing.T) {
	other := egresstest.NewServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	origin := egresstest.NewServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/same":
			http.Redirect(w, r, "/same-landed", http.StatusTemporaryRedirect)
		case "/cross":
			http.Redirect(w, r, "http://other.test:"+other.Port()+"/landed", http.StatusTemporaryRedirect)
		case "/found":
			http.Redirect(w, r, "/found-landed", http.StatusFound)
		default:
			w.WriteHeader(http.StatusOK)
		}
	})
	res := egresstest.NewResolver().Add("first.test", "127.0.0.1").Add("other.test", "127.0.0.1")
	g := New(Config{AllowHosts: []string{"first.test", "other.test"}, Resolver: res})
	base := "http://first.test:" + origin.Port()
	header := func() http.Header {
		h := http.Header{}
		h.Set("Content-Type", "application/json")
		h.Set("Authorization", "GenieKey SECRET-TOKEN")
		h.Set("X-Api-Key", "SECRET-KEY")
		return h
	}
	landed := func(s *egresstest.Server, path string) (egresstest.Request, bool) {
		for _, r := range s.All() {
			if r.Path == path {
				return r, true
			}
		}
		return egresstest.Request{}, false
	}

	// Same host: the request goes along whole.
	if status, err := g.Post(context.Background(), base+"/same", header(), []byte(`{"a":1}`)); err != nil || status != http.StatusOK {
		t.Fatalf("same host: %d %v", status, err)
	}
	r, ok := landed(origin, "/same-landed")
	if !ok || r.Header.Get("Authorization") == "" || r.Header.Get("X-Api-Key") == "" || r.Body != `{"a":1}` {
		t.Errorf("a redirect within the host lost its headers or body: %v %q", r.Header, r.Body)
	}
	if r.Header.Get("Referer") != "" {
		t.Errorf("Referer sent: %q", r.Header.Get("Referer"))
	}

	// Another host: refused, and that host hears nothing, though it is allowlisted.
	if _, err := g.Post(context.Background(), base+"/cross", header(), []byte(`{"routing_key":"SECRET-BODY"}`)); !errors.Is(err, ErrNotAllowed) {
		t.Fatalf("a redirect to another host: %v, want ErrNotAllowed", err)
	}
	if n := len(other.All()); n != 0 {
		t.Errorf("the other host got %d requests", n)
	}

	// A 302 would turn the POST into a GET with no body: not followed, not a delivery.
	if _, err := g.Post(context.Background(), base+"/found", header(), []byte(`{"a":1}`)); !errors.Is(err, ErrNotAllowed) {
		t.Fatalf("a 302: %v, want ErrNotAllowed", err)
	}
	if _, ok := landed(origin, "/found-landed"); ok {
		t.Error("the 302 was followed as a GET")
	}
}

func TestGuard_RedirectCap(t *testing.T) {
	loop := egresstest.NewServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/again", http.StatusTemporaryRedirect)
	})
	g := New(Config{AllowNets: []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}})
	_, err := g.Post(context.Background(), loop.URL+"/start", nil, []byte("{}"))
	if !errors.Is(err, ErrTooManyRedirects) || Class(err) != "redirects" {
		t.Fatalf("a redirect loop: %v (%s)", err, Class(err))
	}
	if n := len(loop.All()); n != MaxRedirects+1 {
		t.Errorf("%d requests were made, want the first and %d redirects", n, MaxRedirects)
	}

	// A chain within the cap is followed to its end.
	hops := 0
	chain := egresstest.NewServer(t, func(w http.ResponseWriter, r *http.Request) {
		if hops < MaxRedirects {
			hops++
			http.Redirect(w, r, "/next", http.StatusTemporaryRedirect)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	if status, err := g.Post(context.Background(), chain.URL, nil, []byte("{}")); err != nil || status != http.StatusNoContent {
		t.Fatalf("a chain of %d: %d %v", MaxRedirects, status, err)
	}
}

// The allowlist admits what it names and nothing next to it.
func TestGuard_Allowlist(t *testing.T) {
	res := egresstest.NewResolver().
		Add("splunk.internal", "192.168.5.5").
		Add("other.internal", "192.168.5.5").
		Add("sub.splunk.internal", "192.168.5.5").
		Add("splunk.internal.evil.example", "192.168.5.5").
		Add("in-range.example", "10.20.3.4").
		Add("next-door.example", "10.21.0.1").
		Add("half.example", "10.20.3.4", "10.21.0.1").
		Add("loop.example", "127.0.0.1")
	cfg := Config{
		AllowNets:  []netip.Prefix{netip.MustParsePrefix("10.20.0.0/16")},
		AllowHosts: []string{"Splunk.Internal."},
		Resolver:   res,
	}
	cases := []struct {
		url      string
		save     string
		dial     string
		wantDial string
	}{
		// The range.
		{"https://10.20.1.1:8088/collector", "ok", "ok", "10.20.1.1:8088"},
		{"http://10.20.1.1:8088/collector", "ok", "ok", "10.20.1.1:8088"},
		{"http://10.20.255.255/", "ok", "ok", "10.20.255.255:80"},
		{"https://[::ffff:10.20.1.1]/", "ok", "ok", "10.20.1.1:443"},
		{"https://10.21.0.1/", "blocked", "blocked", ""},
		{"https://10.19.255.255/", "blocked", "blocked", ""},
		{"https://10.0.0.1/", "blocked", "blocked", ""},
		{"https://127.0.0.1/", "blocked", "blocked", ""},
		{"https://169.254.169.254/", "blocked", "blocked", ""},
		{"https://[64:ff9b::a14:101]/", "blocked", "blocked", ""}, // NAT64 of 10.20.1.1 is not the range
		// A name that resolves into the range: reachable, but over https,
		// because the name itself is not listed.
		{"https://in-range.example/", "ok", "ok", "10.20.3.4:443"},
		{"http://in-range.example/", "blocked", "", ""},
		{"https://next-door.example/", "ok", "blocked", ""},
		{"https://half.example/", "ok", "blocked", ""},
		{"https://loop.example/", "ok", "blocked", ""},
		// The name.
		{"http://splunk.internal:8088/collector", "ok", "ok", "192.168.5.5:8088"},
		{"https://SPLUNK.internal./x", "ok", "ok", "192.168.5.5:443"},
		{"https://other.internal/", "ok", "blocked", ""},
		{"http://other.internal/", "blocked", "", ""},
		{"https://sub.splunk.internal/", "ok", "blocked", ""},
		{"https://splunk.internal.evil.example/", "ok", "blocked", ""},
		{"https://192.168.5.5/", "blocked", "blocked", ""}, // the name is listed, not its address
		// The public internet is as before.
		{"http://93.184.216.34/", "blocked", "", ""},
		{"https://93.184.216.34/", "ok", "ok", "93.184.216.34:443"},
	}
	for _, c := range cases {
		rec := &egresstest.DialRecorder{}
		cfg.Dial = rec.Dial
		g := New(cfg)
		got := "ok"
		if err := g.CheckURL(c.url); err != nil {
			got = Class(err)
		}
		if got != c.save {
			t.Errorf("%s at save time: %s, want %s", c.url, got, c.save)
		}
		if c.dial == "" {
			continue
		}
		req, _ := http.NewRequest(http.MethodPost, c.url, strings.NewReader("{}"))
		_, err := g.client.Do(req)
		if c.dial == "blocked" {
			if !errors.Is(err, ErrNotAllowed) || len(rec.All()) != 0 {
				t.Errorf("%s at dial time: %v, dialled %v; want a refusal", c.url, err, rec.All())
			}
			continue
		}
		if d := rec.All(); !errors.Is(err, egresstest.ErrDialed) || len(d) != 1 || d[0] != c.wantDial {
			t.Errorf("%s at dial time: %v, dialled %v; want %s", c.url, err, d, c.wantDial)
		}
	}
}

// The control hook repeats the judgement on the address the socket is about
// to be connected to.
func TestGuard_ControlJudgesTheSocketAddress(t *testing.T) {
	g := New(Config{AllowNets: []netip.Prefix{netip.MustParsePrefix("10.20.0.0/16")}})
	for addr, want := range map[string]bool{
		"127.0.0.1:80":          false,
		"[::1]:443":             false,
		"10.0.0.1:5432":         false,
		"169.254.169.254:80":    false,
		"[::ffff:10.0.0.1]:80":  false,
		"not-an-address:80":     false,
		"10.20.1.1:8088":        true,
		"93.184.216.34:443":     true,
		"[2606:4700::1111]:443": true,
	} {
		err := g.control(false)("tcp", addr, nil)
		if (err == nil) != want {
			t.Errorf("control(%s) = %v, want allowed %v", addr, err, want)
		}
	}
	if err := g.control(true)("tcp", "192.168.5.5:8088", nil); err != nil {
		t.Errorf("an allowlisted host's address was refused: %v", err)
	}
}
