package policy

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/yatuk/tamga/internal/egress"
)

// UpstreamRules say what a policy may ask of the proxy's own environment and
// network. A policy can be changed through the API by people who are not the
// operator, and two of its fields reach outside it: a pool endpoint names a
// host to send traffic to and an environment variable to send along, and a
// rule's notify value can name an environment variable to read a URL from.
// Without limits, whoever may edit the policy could have the proxy hand its
// own secrets, and every caller's prompts and provider keys, to any server.
//
// The zero value (and a nil pointer) is the strict default: provider key
// names only, each route's own vendor hosts only, https only. Rules are set
// once and not changed afterwards.
type UpstreamRules struct {
	// KeyEnvs are further environment variable names a pool endpoint may
	// name as api_key_env, matched exactly as written.
	KeyEnvs []string
	// NotifyEnvs are further names a rule's notify value may refer to as
	// "${NAME}", matched exactly as written.
	NotifyEnvs []string
	// AllowHosts are exact host names the operator trusts as an upstream for
	// any route, on any port, over plain http too.
	AllowHosts []string
	// AllowNets are address ranges the operator trusts in the same way, for
	// an endpoint whose host is written as an IP address.
	AllowNets []netip.Prefix
	// AllowHostPorts are "host:port" entries: that host or address on that
	// port only.
	AllowHostPorts []string
	// RouteURLs are the operator's own upstream for a route (the
	// TAMGA_<PROVIDER>_URL overrides). A pool on that route, and no other,
	// may point at the same scheme, host and port.
	RouteURLs map[string]*url.URL

	once   sync.Once
	listed *egress.Guard
}

// ErrEndpointURL is a base_url that is not a plain absolute URL;
// ErrEndpointNotAllowed is one that points where a pool on that route may
// not send. Neither message repeats the URL: it can carry credentials.
var (
	ErrEndpointURL        = errors.New("base_url must be an absolute http(s) URL with a host, in ASCII, without credentials, query or fragment")
	ErrEndpointNotAllowed = errors.New("base_url is not allowed for this route")
)

// endpointRefusal is why an endpoint was turned down, with a sentence for
// whoever wrote the policy.
type endpointRefusal struct {
	kind error
	msg  string
}

func (e *endpointRefusal) Error() string { return e.msg }
func (e *endpointRefusal) Unwrap() error { return e.kind }

func notAllowed(msg string) error { return &endpointRefusal{ErrEndpointNotAllowed, msg} }

// upstreamRoutes are the proxy's routes (the keys of providerURLs in
// internal/proxy). A pool is looked up by its route, so a pool under any
// other key would never be used and has no vendor to be judged against.
var upstreamRoutes = []string{"anthropic", "azure", "bedrock", "gemini", "local", "mistral", "openai"}

// UpstreamRoutes returns the route names a pool may be defined for.
func UpstreamRoutes() []string { return append([]string(nil), upstreamRoutes...) }

func knownRoute(route string) bool {
	for _, r := range upstreamRoutes {
		if r == route {
			return true
		}
	}
	return false
}

// vendorHosts are the hosts each route's vendor serves its API from, the
// same ones the proxy forwards to when no pool is defined. Azure and Bedrock
// have one host per resource or region and are matched by VendorHost.
var vendorHosts = map[string][]string{
	"openai":    {"api.openai.com"},
	"anthropic": {"api.anthropic.com"},
	"gemini":    {"generativelanguage.googleapis.com"},
	"mistral":   {"api.mistral.ai"},
}

// localDefault is the local route's built-in upstream. The route has no
// vendor; a pool on it may point at this one address and port, or at what
// the operator named.
const (
	localDefaultScheme = "http"
	localDefaultHost   = "127.0.0.1"
	localDefaultPort   = "11434"
)

const (
	azureOpenAISuffix = ".openai.azure.com"
	bedrockPrefix     = "bedrock-runtime."
	awsSuffix         = ".amazonaws.com"
)

// awsRegion is the shape of an AWS region name: us-east-1, eu-central-1,
// us-gov-west-1.
var awsRegion = regexp.MustCompile(`^[a-z]{2}(-[a-z]+){1,2}-[0-9]{1,2}$`)

// VendorHost reports whether host is one the route's vendor serves its API
// from. host is lower case, without a trailing dot or a port. A suffix is
// matched on a label boundary and admits exactly one label in front of it,
// so none of "evilopenai.azure.com", ".openai.azure.com" and
// "a.b.openai.azure.com" is an Azure OpenAI resource.
func VendorHost(route, host string) bool {
	for _, h := range vendorHosts[route] {
		if h == host {
			return true
		}
	}
	switch route {
	case "azure":
		name, ok := strings.CutSuffix(host, azureOpenAISuffix)
		return ok && dnsLabel(name)
	case "bedrock":
		rest, ok := strings.CutPrefix(host, bedrockPrefix)
		if !ok {
			return false
		}
		region, ok := strings.CutSuffix(rest, awsSuffix)
		return ok && awsRegion.MatchString(region)
	}
	return false
}

// dnsLabel is one label of a host name: letters, digits and inner hyphens.
func dnsLabel(s string) bool {
	if s == "" || len(s) > 63 || s[0] == '-' || s[len(s)-1] == '-' {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
			return false
		}
	}
	return true
}

// envName is the only shape of variable name a policy may refer to. Upper
// case only, because the environment is not case-sensitive on Windows:
// there "tamga_admin_key" reads TAMGA_ADMIN_KEY, and a test for the TAMGA_
// prefix alone would let it through.
var envName = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

// A provider key: OPENAI_API_KEY, or a regional or numbered one such as
// ANTHROPIC_API_KEY_EU.
var (
	providerKeyName = regexp.MustCompile(`^[A-Z][A-Z0-9_]*_API_KEY(_[A-Z0-9]+)*$`)
	webhookURLName  = regexp.MustCompile(`^[A-Z][A-Z0-9_]*_WEBHOOK_URL$`)
)

// ValidEnvName reports whether name has the shape a policy may refer to.
func ValidEnvName(name string) bool { return envName.MatchString(name) }

// ReservedEnv reports whether a name is one of the proxy's own variables,
// whatever its case.
func ReservedEnv(name string) bool {
	return len(name) >= 6 && strings.EqualFold(name[:6], "TAMGA_")
}

func nameListed(list []string, name string) bool {
	for _, n := range list {
		if n == name {
			return true
		}
	}
	return false
}

// KeyEnvAllowed reports whether a pool endpoint may name this environment
// variable as its api_key_env. Never one of the proxy's own (TAMGA_*);
// otherwise a name that looks like a provider key or one the operator
// listed. Names are matched as written, and nothing is trimmed: the name
// judged is the name read.
func (r *UpstreamRules) KeyEnvAllowed(name string) bool {
	if !envName.MatchString(name) || ReservedEnv(name) {
		return false
	}
	if providerKeyName.MatchString(name) {
		return true
	}
	return r != nil && nameListed(r.KeyEnvs, name)
}

// NotifyEnvAllowed is the same judgement for the "${NAME}" a rule's notify
// value may stand for: by default only names ending in _WEBHOOK_URL.
func (r *UpstreamRules) NotifyEnvAllowed(name string) bool {
	if !envName.MatchString(name) || ReservedEnv(name) {
		return false
	}
	if webhookURLName.MatchString(name) {
		return true
	}
	return r != nil && nameListed(r.NotifyEnvs, name)
}

var strictListed = sync.OnceValue(func() *egress.Guard { return egress.New(egress.Config{}) })

// listedGuard is an egress guard holding the operator's any-port entries.
// It is used for its judgement of a URL's host, so that IP literals in
// every spelling are read here exactly as they are where alerts are sent.
func (r *UpstreamRules) listedGuard() *egress.Guard {
	if r == nil {
		return strictListed()
	}
	r.once.Do(func() {
		r.listed = egress.New(egress.Config{AllowNets: r.AllowNets, AllowHosts: r.AllowHosts})
	})
	return r.listed
}

// hostListed reports whether the operator listed this host for this port.
func (r *UpstreamRules) hostListed(host, port string) bool {
	if r == nil {
		return false
	}
	// The guard accepts plain http only for a host or address on its list,
	// which is the question asked here.
	if r.listedGuard().CheckURL("http://"+net.JoinHostPort(host, "80")) == nil {
		return true
	}
	return nameListed(r.AllowHostPorts, net.JoinHostPort(host, port))
}

// DialHosts are the hosts a pool connection may reach whatever they resolve
// to: the ones the operator listed, the hosts of the operator's own route
// overrides and the local route's built-in address. A vendor host is not
// among them, so a vendor name that resolves to an internal address is
// refused when it is dialled. Which port and which route a host may be used
// for is decided by CheckEndpoint before any connection is made.
func (r *UpstreamRules) DialHosts() []string {
	out := []string{localDefaultHost}
	if r == nil {
		return out
	}
	out = append(out, r.AllowHosts...)
	for _, hp := range r.AllowHostPorts {
		if h, _, err := net.SplitHostPort(hp); err == nil {
			out = append(out, h)
		}
	}
	routes := make([]string, 0, len(r.RouteURLs))
	for k := range r.RouteURLs {
		routes = append(routes, k)
	}
	sort.Strings(routes)
	for _, k := range routes {
		if u := r.RouteURLs[k]; u != nil && u.Hostname() != "" {
			out = append(out, normalHost(u.Hostname()))
		}
	}
	return out
}

// normalHost is a host name as it is compared: lower case, without the
// trailing dot of a fully qualified name. An IP literal is put in its one
// canonical spelling.
func normalHost(h string) string {
	h = strings.TrimSuffix(strings.ToLower(h), ".")
	if ip, err := netip.ParseAddr(h); err == nil && ip.Zone() == "" {
		return ip.Unmap().String()
	}
	return h
}

func effectivePort(u *url.URL) string {
	if p := u.Port(); p != "" {
		return p
	}
	if u.Scheme == "http" {
		return "80"
	}
	return "443"
}

// cleanBasePath accepts no path, "/" or plain segments: no "." or "..", no
// empty segment, nothing percent-encoded.
func cleanBasePath(u *url.URL) bool {
	if u.RawPath != "" {
		return false
	}
	p := strings.TrimSuffix(u.Path, "/")
	if p == "" {
		return true
	}
	if !strings.HasPrefix(p, "/") {
		return false
	}
	for _, seg := range strings.Split(p[1:], "/") {
		if seg == "" || seg == "." || seg == ".." {
			return false
		}
	}
	return true
}

// CheckBaseURL parses a pool endpoint's base_url, judges it for a route and
// returns it in the form to connect to: host in lower case, no trailing dot.
// Surrounding white space is dropped first, as the pool does; the value
// judged is the value used.
func (r *UpstreamRules) CheckBaseURL(route, raw string) (*url.URL, error) {
	u, err := parseBaseURL(raw)
	if err != nil {
		return nil, err
	}
	if err := r.CheckEndpoint(route, u); err != nil {
		return nil, err
	}
	return u, nil
}

// ParseBaseURL is the parsing half of CheckBaseURL, for the pool, which
// keeps an endpoint the rules refuse so that it can refuse it on use.
func ParseBaseURL(raw string) (*url.URL, error) { return parseBaseURL(raw) }

func parseBaseURL(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, ErrEndpointURL
	}
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		// Not ASCII: url.Parse keeps "api。openai。com" and look-alike
		// letters as written, and the transport maps them when it dials. A
		// backslash is a slash to some URL parsers and not to others.
		if c <= ' ' || c >= 0x7f || c == '\\' {
			return nil, ErrEndpointURL
		}
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, ErrEndpointURL
	}
	if u.Opaque != "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawFragment != "" {
		return nil, ErrEndpointURL
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, ErrEndpointURL
	}
	// Hostname, not Host: "https://:443" has a Host and no host, and would
	// be dialled as the proxy's own machine.
	if u.Hostname() == "" || strings.Contains(u.Hostname(), "%") {
		return nil, ErrEndpointURL
	}
	host := normalHost(u.Hostname())
	if strings.HasPrefix(host, ".") || strings.HasSuffix(host, ".") || strings.Contains(host, "..") {
		return nil, ErrEndpointURL
	}
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	if p := u.Port(); p != "" {
		host += ":" + p
	}
	u.Host = host
	return u, nil
}

// CheckEndpoint judges where a pool endpoint points. Callers of a route send
// their own provider credential and their prompts, and the endpoint adds a
// key from the proxy's environment, so the host must be one the operator can
// be taken to trust for that route:
//
//   - a host or address the operator listed, on the port listed if one was
//     (any route, http or https);
//   - the operator's own upstream for this route, or the local route's
//     built-in one: same scheme, host and port;
//   - the route's vendor host, over https on the default port, with no path.
//
// It is called when a policy is loaded and again before every request.
func (r *UpstreamRules) CheckEndpoint(route string, u *url.URL) error {
	if u == nil || u.Opaque != "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawFragment != "" {
		return ErrEndpointURL
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return ErrEndpointURL
	}
	if u.Hostname() == "" {
		return ErrEndpointURL
	}
	for i := 0; i < len(u.Host); i++ {
		if u.Host[i] <= ' ' || u.Host[i] >= 0x7f || u.Host[i] == '%' {
			return ErrEndpointURL
		}
	}
	host, port := normalHost(u.Hostname()), effectivePort(u)
	// One trailing dot is the fully qualified spelling; a second one, or an
	// empty label anywhere, is not a host name.
	if strings.HasPrefix(host, ".") || strings.HasSuffix(host, ".") || strings.Contains(host, "..") {
		return ErrEndpointURL
	}

	trusted := r.hostListed(host, port)
	if !trusted && r != nil {
		if o := r.RouteURLs[route]; o != nil && o.Scheme == u.Scheme &&
			normalHost(o.Hostname()) == host && effectivePort(o) == port {
			trusted = true
		}
	}
	if !trusted && route == "local" && u.Scheme == localDefaultScheme && host == localDefaultHost && port == localDefaultPort {
		trusted = true
	}
	if trusted {
		if !cleanBasePath(u) {
			return notAllowed("base_url path must be plain segments, without \".\", \"..\", empty or encoded segments")
		}
		return nil
	}

	if !VendorHost(route, host) {
		return notAllowed("base_url host is not an upstream of this route: allowed are the route's own vendor hosts, " +
			"the host and port of the route's TAMGA_<PROVIDER>_URL and what the operator listed in TAMGA_UPSTREAM_ALLOW")
	}
	if u.Scheme != "https" {
		return notAllowed("base_url must use https; plain http is accepted only for what the operator listed in TAMGA_UPSTREAM_ALLOW")
	}
	if port != "443" {
		return notAllowed("base_url must use the default https port on a vendor host")
	}
	if (u.Path != "" && u.Path != "/") || u.RawPath != "" {
		return notAllowed("base_url must not have a path on a vendor host")
	}
	return nil
}

const keyEnvMessage = "api_key_env names an environment variable a policy may not read: allowed are upper-case names ending in " +
	"_API_KEY (or _API_KEY_<SUFFIX>) and names the operator listed in TAMGA_UPSTREAM_KEY_ENVS, never a TAMGA_ variable"

const notifyEnvMessage = "webhook refers to an environment variable a policy may not read, so it resolves to no URL: allowed are " +
	"upper-case names ending in _WEBHOOK_URL and names the operator listed in TAMGA_NOTIFY_URL_ENVS, never a TAMGA_ variable"

// upstreamIssues are the pools of a policy that the rules refuse. Each is an
// error: such a policy does not load. The values judged are the decoded
// ones, the same strings the pool is built from, whatever the YAML looked
// like. The messages name the field and never a value, neither the URL nor
// the variable.
func upstreamIssues(p *Policy, rules *UpstreamRules) []ValidationIssue {
	if p == nil || p.Providers == nil || len(p.Providers.Pools) == 0 {
		return nil
	}
	keys := make([]string, 0, len(p.Providers.Pools))
	for k := range p.Providers.Pools {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var issues []ValidationIssue
	for _, route := range keys {
		if !knownRoute(route) {
			issues = append(issues, ValidationIssue{
				Field:    fmt.Sprintf("providers.pools[%q]", route),
				Rule:     "unknown_route",
				Message:  "a pool must be defined under one of the proxy's routes, written exactly: " + strings.Join(upstreamRoutes, ", "),
				Severity: "error",
			})
			continue
		}
		for i, ep := range p.Providers.Pools[route].Endpoints {
			prefix := fmt.Sprintf("providers.pools.%s.providers[%d]", route, i)
			if strings.TrimSpace(ep.BaseURL) != "" {
				if _, err := rules.CheckBaseURL(route, ep.BaseURL); err != nil {
					rule := "not_allowed"
					if errors.Is(err, ErrEndpointURL) {
						rule = "url"
					}
					issues = append(issues, ValidationIssue{Field: prefix + ".base_url", Rule: rule, Message: err.Error(), Severity: "error"})
				}
			}
			if ep.APIKeyEnv != "" && !rules.KeyEnvAllowed(ep.APIKeyEnv) {
				issues = append(issues, ValidationIssue{Field: prefix + ".api_key_env", Rule: "not_allowed", Message: keyEnvMessage, Severity: "error"})
			}
		}
	}
	return issues
}

// notifyEnvName returns the variable a notify value refers to, and whether
// the value is such a reference at all ("${NAME}" and nothing else).
func notifyEnvName(v string) (string, bool) {
	v = strings.TrimSpace(v)
	if !strings.HasPrefix(v, "${") || !strings.HasSuffix(v, "}") {
		return "", false
	}
	return v[2 : len(v)-1], true
}

// notifyIssues warns about notify values that name a variable the rules do
// not let a policy read. The policy still loads; the reference gives no URL.
func notifyIssues(p *Policy, rules *UpstreamRules) []ValidationIssue {
	if p == nil {
		return nil
	}
	names := make([]string, 0, len(p.Rules))
	for k := range p.Rules {
		names = append(names, k)
	}
	sort.Strings(names)
	var issues []ValidationIssue
	for _, name := range names {
		for i, n := range p.Rules[name].Notify {
			if env, ok := notifyEnvName(n.Webhook); ok && !rules.NotifyEnvAllowed(env) {
				issues = append(issues, ValidationIssue{
					Field:    fmt.Sprintf("rules.%s.notify[%d].webhook", name, i),
					Rule:     "not_allowed",
					Message:  notifyEnvMessage,
					Severity: "warning",
				})
			}
		}
	}
	return issues
}

// activeUpstreamRules are the rules every policy is loaded and validated
// under. main sets them once from the environment before the first load.
var activeUpstreamRules atomic.Pointer[UpstreamRules]

// SetUpstreamRules installs the rules policies are loaded under and returns
// a function that puts the previous ones back (for tests). Nil restores the
// strict default.
func SetUpstreamRules(r *UpstreamRules) (restore func()) {
	old := activeUpstreamRules.Swap(r)
	return func() { activeUpstreamRules.Store(old) }
}

// ActiveUpstreamRules returns the rules in force; nil is the strict default
// and every method accepts it.
func ActiveUpstreamRules() *UpstreamRules { return activeUpstreamRules.Load() }
