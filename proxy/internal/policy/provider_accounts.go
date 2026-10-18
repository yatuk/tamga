package policy

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// A company that pays for the business edition of a provider wants its
// people to use that account and no other: a personal key holds the same
// prompts under terms the company did not sign. The provider_accounts
// section says which provider accounts a request may use on each route.
//
//	provider_accounts:
//	  enabled: true
//	  action: BLOCK                     # or LOG while the lists are filled in
//	  providers:
//	    openai:
//	      organizations: [org-EXAMPLE]  # OpenAI-Organization
//	      projects: [proj_EXAMPLE]      # OpenAI-Project
//	      keys: ["sha256:<64 hex>"]     # fingerprints of the keys that may be sent
//	    anthropic:
//	      workspaces: [wrkspc_EXAMPLE]  # anthropic-workspace-id
//	      replace_key_env: ANTHROPIC_API_KEY
//
// For a route with an entry, every list that is set must hold what the
// request carries: each provider credential it sends (keys), its
// OpenAI-Organization and OpenAI-Project headers, its anthropic-workspace-id
// header. A header a list is set for must be present, once: without it the
// provider uses the key's default, which the proxy cannot see. The provider
// checks that the key belongs to the account the header names, so a header
// on the list with a key from elsewhere is refused there. A request that
// fails is answered 403 provider_account_not_allowed, or, with action LOG,
// forwarded and logged.
//
// replace_key_env names the environment variable that holds the
// organisation's own key for the route. The proxy then drops every
// credential the caller sent and sends that key in the header the provider
// reads, so a personal key never leaves; keys is not consulted. Only a
// request with a verified Tamga key is served this way, whatever action
// says: without one the proxy would lend the organisation's key to anyone
// who can reach it.
//
// A route without an entry is not checked. providers.allowed is the way to
// close a route.

// ProviderAccountsConfig is the provider_accounts section. It is left out of
// the JSON form when absent, so that a policy without it keeps its digest.
type ProviderAccountsConfig struct {
	Enabled bool `yaml:"enabled" json:"enabled"`
	// Action is BLOCK (the default) or LOG.
	Action string `yaml:"action" json:"action,omitempty"`
	// Providers are the entries, keyed by route name as written in
	// providers.pools: openai, anthropic, gemini, azure, mistral, local.
	Providers map[string]ProviderAccounts `yaml:"providers" json:"providers,omitempty"`
}

// ProviderAccounts is the entry of one route.
type ProviderAccounts struct {
	// Keys are fingerprints of the provider keys that may be sent:
	// "sha256:" and the lower-case hex SHA-256 of the key as sent.
	Keys []string `yaml:"keys" json:"keys,omitempty"`
	// Organizations are the OpenAI-Organization values allowed (openai).
	Organizations []string `yaml:"organizations" json:"organizations,omitempty"`
	// Projects are the OpenAI-Project values allowed (openai).
	Projects []string `yaml:"projects" json:"projects,omitempty"`
	// Workspaces are the anthropic-workspace-id values allowed (anthropic).
	Workspaces []string `yaml:"workspaces" json:"workspaces,omitempty"`
	// ReplaceKeyEnv names the variable holding the organisation's key for
	// the route. It is judged as a pool endpoint's api_key_env is.
	ReplaceKeyEnv string `yaml:"replace_key_env" json:"replace_key_env,omitempty"`
}

// Limits on provider_accounts.
const (
	MaxProviderAccountEntries = 500
	MaxProviderAccountValue   = 128
)

// The headers the lists are checked against.
const (
	OpenAIOrganizationHeader  = "OpenAI-Organization"
	OpenAIProjectHeader       = "OpenAI-Project"
	AnthropicWorkspaceHeader  = "Anthropic-Workspace-Id"
	providerAccountsField     = "provider_accounts"
	keyFingerprintPrefix      = "sha256:"
	providerAccountsRouteHelp = "openai, anthropic, gemini, azure, mistral, local"
)

// Reasons a request fails the check. They are returned to the caller.
const (
	AccountKeyMissing             = "key_missing"
	AccountKeyNotAllowed          = "key_not_allowed"
	AccountOrganizationMissing    = "organization_missing"
	AccountOrganizationNotAllowed = "organization_not_allowed"
	AccountProjectMissing         = "project_missing"
	AccountProjectNotAllowed      = "project_not_allowed"
	AccountWorkspaceMissing       = "workspace_missing"
	AccountWorkspaceNotAllowed    = "workspace_not_allowed"
)

// accountRoutes are the routes an entry may be written for. Bedrock is
// signed with the host's AWS credentials (SigV4) and carries no key to
// check or replace.
var accountRoutes = map[string]bool{"openai": true, "anthropic": true, "gemini": true, "azure": true, "mistral": true, "local": true}

var (
	keyFingerprint = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	// An account identifier: what the providers print (org-…, proj_…,
	// wrkspc_…), and nothing that could split or end a header.
	accountValue = regexp.MustCompile(`^[A-Za-z0-9._:-]+$`)
)

// KeyFingerprint is the fingerprint a provider key is listed under.
func KeyFingerprint(key string) string {
	sum := sha256.Sum256([]byte(key))
	return keyFingerprintPrefix + hex.EncodeToString(sum[:])
}

// AccountRequest is what a request carries that the check reads.
type AccountRequest struct {
	// Credentials are the provider credentials the request sends, every
	// one of them: whichever the provider reads must be on the list.
	Credentials []string
	// The values of each header, as sent.
	Organizations []string
	Projects      []string
	Workspaces    []string
}

// ProviderAccountRule is the compiled entry of one route.
type ProviderAccountRule struct {
	Route         string
	Log           bool
	keys          map[string]bool
	organizations map[string]bool
	projects      map[string]bool
	workspaces    map[string]bool
	ReplaceKeyEnv string
}

// ProviderAccountRule returns the entry for a route, nil when the section is
// off or the route has none.
func (p *Policy) ProviderAccountRule(route string) *ProviderAccountRule {
	if p == nil || p.ProviderAccounts == nil || !p.ProviderAccounts.Enabled {
		return nil
	}
	e, ok := p.ProviderAccounts.Providers[route]
	if !ok {
		return nil
	}
	set := func(list []string) map[string]bool {
		if len(list) == 0 {
			return nil
		}
		m := make(map[string]bool, len(list))
		for _, v := range list {
			m[strings.TrimSpace(v)] = true
		}
		return m
	}
	return &ProviderAccountRule{
		Route:         route,
		Log:           strings.EqualFold(strings.TrimSpace(p.ProviderAccounts.Action), string(ActionLog)),
		keys:          set(e.Keys),
		organizations: set(e.Organizations),
		projects:      set(e.Projects),
		workspaces:    set(e.Workspaces),
		ReplaceKeyEnv: strings.TrimSpace(e.ReplaceKeyEnv),
	}
}

// Replaces reports whether the route sends the organisation's own key.
func (r *ProviderAccountRule) Replaces() bool { return r != nil && r.ReplaceKeyEnv != "" }

// Check returns why the request may not use the route, or "" when it may.
// Under replacement the caller's credentials are dropped and not judged.
func (r *ProviderAccountRule) Check(req AccountRequest) string {
	if r == nil {
		return ""
	}
	if r.keys != nil && !r.Replaces() {
		if len(req.Credentials) == 0 {
			return AccountKeyMissing
		}
		for _, c := range req.Credentials {
			if !r.keys[KeyFingerprint(c)] {
				return AccountKeyNotAllowed
			}
		}
	}
	if why := oneOf(r.organizations, req.Organizations, AccountOrganizationMissing, AccountOrganizationNotAllowed); why != "" {
		return why
	}
	if why := oneOf(r.projects, req.Projects, AccountProjectMissing, AccountProjectNotAllowed); why != "" {
		return why
	}
	return oneOf(r.workspaces, req.Workspaces, AccountWorkspaceMissing, AccountWorkspaceNotAllowed)
}

// oneOf judges one header against its list: it must be sent once, with a
// value on the list. Sent twice, the value the provider reads is not known.
func oneOf(allowed map[string]bool, values []string, missing, notAllowed string) string {
	if allowed == nil {
		return ""
	}
	var got []string
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			got = append(got, v)
		}
	}
	if len(got) == 0 {
		return missing
	}
	if len(got) > 1 || !allowed[got[0]] {
		return notAllowed
	}
	return ""
}

// providerAccountIssues covers provider_accounts. Errors fail the load.
func providerAccountIssues(p *Policy, rules *UpstreamRules) []ValidationIssue {
	pa := p.ProviderAccounts
	if pa == nil {
		return nil
	}
	var issues []ValidationIssue
	switch strings.ToUpper(strings.TrimSpace(pa.Action)) {
	case "", string(ActionBlock), string(ActionLog):
	default:
		issues = append(issues, boundError(providerAccountsField+".action", "enum", "action is BLOCK or LOG"))
	}
	if pa.Enabled && len(pa.Providers) == 0 {
		issues = append(issues, boundWarning(providerAccountsField+".providers", "no_effect", "provider_accounts is enabled and names no route: nothing is checked"))
	}
	routes := make([]string, 0, len(pa.Providers))
	for k := range pa.Providers {
		routes = append(routes, k)
	}
	sort.Strings(routes)
	for _, route := range routes {
		e := pa.Providers[route]
		at := fmt.Sprintf("%s.providers.%s", providerAccountsField, route)
		if !accountRoutes[route] {
			msg := "an entry is written under one of the routes, exactly: " + providerAccountsRouteHelp
			if route == "bedrock" {
				msg = "bedrock requests are signed with the host's AWS credentials and carry no key to check or replace"
			}
			issues = append(issues, boundError(at, "unknown_route", "%s", msg))
			continue
		}
		lists := []struct {
			name   string
			values []string
			only   string
		}{
			{"keys", e.Keys, ""},
			{"organizations", e.Organizations, "openai"},
			{"projects", e.Projects, "openai"},
			{"workspaces", e.Workspaces, "anthropic"},
		}
		empty := e.ReplaceKeyEnv == ""
		for _, l := range lists {
			if len(l.values) == 0 {
				continue
			}
			empty = false
			if l.only != "" && route != l.only {
				issues = append(issues, boundError(at+"."+l.name, "route", "%s is read on the %s route only", l.name, l.only))
				continue
			}
			if n := len(l.values); n > MaxProviderAccountEntries {
				issues = append(issues, boundError(at+"."+l.name, "too_many", "%d entries, the limit is %d", n, MaxProviderAccountEntries))
			}
			for i, v := range l.values {
				v = strings.TrimSpace(v)
				field := fmt.Sprintf("%s.%s[%d]", at, l.name, i)
				if l.name == "keys" {
					if !keyFingerprint.MatchString(v) {
						issues = append(issues, boundError(field, "fingerprint", "a key is listed by its fingerprint, \"sha256:\" and 64 lower-case hex digits, never as the key itself"))
					}
					continue
				}
				if len(v) > MaxProviderAccountValue || !accountValue.MatchString(v) {
					issues = append(issues, boundError(field, "value", "an identifier is 1 to %d letters, digits and . _ : -", MaxProviderAccountValue))
				}
			}
		}
		if e.ReplaceKeyEnv != "" {
			if !rules.KeyEnvAllowed(e.ReplaceKeyEnv) {
				issues = append(issues, boundError(at+".replace_key_env", "not_allowed", "%s", strings.Replace(keyEnvMessage, "api_key_env", "replace_key_env", 1)))
			}
			if len(e.Keys) > 0 {
				issues = append(issues, boundWarning(at+".keys", "no_effect", "keys is not consulted while replace_key_env is set: the caller's credentials are dropped"))
			}
			if p.Providers != nil {
				for i, ep := range p.Providers.Pools[route].Endpoints {
					if ep.APIKeyEnv != "" {
						issues = append(issues, boundError(fmt.Sprintf("providers.pools.%s.providers[%d].api_key_env", route, i), "conflict",
							"provider_accounts.providers.%s.replace_key_env is set as well: say which key the route sends in one place", route))
					}
				}
			}
		}
		if empty {
			issues = append(issues, boundWarning(at, "no_effect", "the entry sets no list and no replace_key_env: the route is not checked"))
		}
	}
	return issues
}
