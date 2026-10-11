package policy

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// ROUTE is the action for a request that holds something the organisation
// will not send to an outside provider and still wants answered. The request
// is not refused and not masked: it is sent, as written, to a pool of model
// servers the organisation runs itself, and nothing of it reaches the
// provider the caller addressed.
//
//	rules:
//	  pii_detection:
//	    action: ROUTE
//	    route_to: internal-llm
//	providers:
//	  pools:
//	    internal-llm:
//	      internal: true
//	      dialect: openai
//	      model: llama-3.1-70b
//	      allow_private_addresses: true
//	      providers:
//	        - name: vllm-1
//	          base_url: http://10.0.4.12:8000
//	          api_key_env: INTERNAL_LLM_API_KEY
//	          priority: 1
//
// What is decided here, and checked when the policy is loaded:
//
//   - Order: BLOCK > ROUTE > STRIP > REDACT > WARN > LOG > PASS. A request
//     another rule blocks is blocked; one that would otherwise be masked or
//     stripped is routed instead.
//   - The target is an internal pool. A pool is internal because the policy
//     says so (internal: true); it is named freely, never after one of the
//     proxy's routes, and nothing but ROUTE uses it. ROUTE to a pool that is
//     not internal does not load: sending what a rule held back to another
//     outside provider is not what the action is for.
//   - One pool per policy. Every ROUTE rule and entry of a policy names the
//     same pool; two different ones do not load. Rules are a map and have no
//     order to break a tie with, and which model reads a request should not
//     depend on which of its findings happened to be looked at first.
//   - Where the pool points is the operator's decision: every endpoint must
//     be listed in TAMGA_UPSTREAM_ALLOW (UpstreamRules.CheckInternalEndpoint),
//     and a private address needs the pool's own allow_private_addresses too.
//   - The dialect is the API the pool's servers speak. A request is routed
//     only when the route it came in on speaks the same one (RouteDialect);
//     nothing is translated between dialects.
//
// What happens to the request is in internal/proxy (route.go there).

// Dialects an internal pool may speak.
const (
	DialectOpenAI    = "openai"
	DialectAnthropic = "anthropic"
)

// routeDialects maps each of the proxy's routes to the API dialect its
// callers speak, for the routes whose requests name the model in the body
// and use the same paths as the dialect's own API. Azure names a deployment
// in the path, Gemini the model, and Bedrock signs the request: none of them
// can be handed to another server as it is, so none has a dialect here.
var routeDialects = map[string]string{
	"openai":    DialectOpenAI,
	"mistral":   DialectOpenAI,
	"local":     DialectOpenAI,
	"anthropic": DialectAnthropic,
}

// RouteDialect returns the dialect a route's callers speak, or "" for a
// route whose requests cannot be routed to an internal pool.
func RouteDialect(route string) string { return routeDialects[route] }

func knownDialect(d string) bool { return d == DialectOpenAI || d == DialectAnthropic }

// poolName is the shape of an internal pool's name. It is written to a
// response header, a metric label and the request log.
var poolName = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,62}$`)

// MaxRouteModelBytes bounds an internal pool's model name.
const MaxRouteModelBytes = 128

// InternalPool returns the internal pool of that name.
func (p *Policy) InternalPool(name string) (ProviderUpstreamPool, bool) {
	if p == nil || p.Providers == nil || name == "" {
		return ProviderUpstreamPool{}, false
	}
	pool, ok := p.Providers.Pools[name]
	if !ok || !pool.Internal {
		return ProviderUpstreamPool{}, false
	}
	return pool, true
}

// routeRef is one place in the policy that says ROUTE.
type routeRef struct {
	field string // of the route_to that goes with it
	pool  string
}

// routeRefs lists every ROUTE of the policy in a fixed order: the rules by
// name, then the custom entities and the competitors as they are listed.
func routeRefs(p *Policy) []routeRef {
	if p == nil {
		return nil
	}
	var refs []routeRef
	for _, name := range sortedRuleNames(p.Rules) {
		rule := p.Rules[name]
		if rule.Action == ActionRoute || (rule.OverrideAction != nil && *rule.OverrideAction == ActionRoute) {
			refs = append(refs, routeRef{"rules." + name + ".route_to", strings.TrimSpace(rule.RouteTo)})
		}
	}
	for i, ce := range p.CustomEntities {
		if a, ok := parseEntryAction(ce.Action); ok && a == ActionRoute {
			refs = append(refs, routeRef{fmt.Sprintf("custom_entities[%d].route_to", i), strings.TrimSpace(ce.RouteTo)})
		}
	}
	for i, c := range p.Competitors {
		if a, ok := parseEntryAction(c.Action); ok && a == ActionRoute {
			refs = append(refs, routeRef{fmt.Sprintf("competitors[%d].route_to", i), strings.TrimSpace(c.RouteTo)})
		}
	}
	return refs
}

// RoutePool returns the pool a request is sent to when its verdict is ROUTE,
// and "" when the policy routes nowhere. A loaded policy names one pool in
// every ROUTE (routeIssues); for a policy built some other way the first in
// routeRefs' order counts.
func (p *Policy) RoutePool() string {
	for _, ref := range routeRefs(p) {
		if ref.pool != "" {
			return ref.pool
		}
	}
	return ""
}

// routeIssues are the ROUTE rules and internal pools the policy cannot be
// loaded with. Every one is an error: a ROUTE that cannot be carried out
// would block every request it matches, or worse, be taken for a rule that
// lets them through.
func routeIssues(p *Policy, rules *UpstreamRules) []ValidationIssue {
	if p == nil {
		return nil
	}
	issues := internalPoolIssues(p, rules)

	first := ""
	for _, ref := range routeRefs(p) {
		switch {
		case ref.pool == "":
			issues = append(issues, boundError(ref.field, "required",
				"the action ROUTE needs route_to: the name of an internal pool under providers.pools"))
			continue
		case p.Providers == nil:
			issues = append(issues, boundError(ref.field, "unknown_pool",
				"route_to names a pool that providers.pools does not define"))
			continue
		}
		pool, ok := p.Providers.Pools[ref.pool]
		switch {
		case !ok:
			issues = append(issues, boundError(ref.field, "unknown_pool",
				"route_to names a pool that providers.pools does not define"))
		case !pool.Internal:
			issues = append(issues, boundError(ref.field, "not_internal",
				"route_to names a pool that is not marked internal: true. ROUTE keeps a request inside the organisation; "+
					"it does not send it to another outside provider"))
		case first == "":
			first = ref.pool
		case ref.pool != first:
			issues = append(issues, boundError(ref.field, "two_pools",
				"route_to names a second pool: every ROUTE of a policy sends to the same internal pool, so that which "+
					"model reads a request does not depend on which of its findings is looked at first"))
		}
	}

	// route_to with an action that is not ROUTE does nothing. It is refused
	// rather than ignored: whoever wrote it expects the request to stay
	// inside, and it would go out.
	for _, name := range sortedRuleNames(p.Rules) {
		rule := p.Rules[name]
		routes := rule.Action == ActionRoute || (rule.OverrideAction != nil && *rule.OverrideAction == ActionRoute)
		if strings.TrimSpace(rule.RouteTo) != "" && !routes {
			issues = append(issues, boundError("rules."+name+".route_to", "no_route",
				"route_to is set and the rule's action is not ROUTE: the request would be sent to the provider. Set action: ROUTE or remove route_to"))
		}
	}
	entry := func(field, action, routeTo string) {
		if strings.TrimSpace(routeTo) == "" {
			return
		}
		if a, ok := parseEntryAction(action); !ok || a != ActionRoute {
			issues = append(issues, boundError(field, "no_route",
				"route_to is set and the action is not ROUTE: the request would be sent to the provider. Set action: ROUTE or remove route_to"))
		}
	}
	for i, ce := range p.CustomEntities {
		entry(fmt.Sprintf("custom_entities[%d].route_to", i), ce.Action, ce.RouteTo)
	}
	for i, c := range p.Competitors {
		entry(fmt.Sprintf("competitors[%d].route_to", i), c.Action, c.RouteTo)
	}
	return issues
}

// internalPoolIssues judges the pools marked internal, and refuses the
// internal pool's own fields on a pool that is not.
func internalPoolIssues(p *Policy, rules *UpstreamRules) []ValidationIssue {
	if p.Providers == nil || len(p.Providers.Pools) == 0 {
		return nil
	}
	names := make([]string, 0, len(p.Providers.Pools))
	for k := range p.Providers.Pools {
		names = append(names, k)
	}
	sort.Strings(names)
	var issues []ValidationIssue
	for _, name := range names {
		pool := p.Providers.Pools[name]
		field := fmt.Sprintf("providers.pools[%q]", name)
		if !pool.Internal {
			// Meaningless on a route's own pool, and a sign the pool was
			// meant to be internal.
			for _, set := range []struct {
				on  bool
				key string
			}{
				{strings.TrimSpace(pool.Dialect) != "", "dialect"},
				{strings.TrimSpace(pool.Model) != "", "model"},
				{pool.AllowPrivateAddresses, "allow_private_addresses"},
			} {
				if set.on {
					issues = append(issues, boundError(field+"."+set.key, "internal_only",
						"%s applies to an internal pool only: mark the pool internal: true, or remove it", set.key))
				}
			}
			continue
		}
		switch {
		case knownRoute(name):
			issues = append(issues, boundError(field, "route_name",
				"an internal pool cannot be named after one of the proxy's routes (%s): that name is the route's own pool", strings.Join(upstreamRoutes, ", ")))
			continue
		case !poolName.MatchString(name):
			issues = append(issues, boundError(field, "name",
				"an internal pool's name is lower-case letters, digits, hyphens and underscores, at most 63, starting with a letter or digit"))
			continue
		}
		switch d := pool.Dialect; {
		case d == "":
			issues = append(issues, boundError(field+".dialect", "required",
				"an internal pool says which API its servers speak: dialect: openai or dialect: anthropic"))
		case !knownDialect(d):
			issues = append(issues, boundError(field+".dialect", "enum",
				"unknown dialect %q (use openai or anthropic, in lower case)", printableValue(d)))
		}
		if m := pool.Model; m != "" {
			switch {
			case len(m) > MaxRouteModelBytes:
				issues = append(issues, boundError(field+".model", "too_long", "model is %d bytes, the limit is %d", len(m), MaxRouteModelBytes))
			case strings.TrimSpace(m) != m || strings.ContainsFunc(m, func(r rune) bool { return r < 0x20 || r == 0x7f }):
				issues = append(issues, boundError(field+".model", "format", "model must not have surrounding white space or control characters"))
			}
		}
		for i, ep := range pool.Endpoints {
			prefix := fmt.Sprintf("providers.pools.%s.providers[%d]", name, i)
			if strings.TrimSpace(ep.BaseURL) != "" {
				u, err := parseBaseURL(ep.BaseURL)
				if err == nil {
					err = rules.CheckInternalEndpoint(u, pool.AllowPrivateAddresses)
				}
				if err != nil {
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
