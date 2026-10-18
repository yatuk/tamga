// Package tenantheaders adds the tenant-restriction headers that AI and
// identity vendors document for a corporate proxy. Each header tells the
// vendor which of its tenants (organisations, workspaces, enterprises,
// domains) may be signed in to from this network; the vendor enforces it.
// A personal account is refused by the vendor itself, on its own sign-in or
// API, with no guess about the page on the proxy's side.
//
// The package does not open connections and does not decide which requests
// it sees. It is for a mode in which the proxy reads the plain HTTP requests
// of a whole network: an ICAP REQMOD service behind a TLS-inspecting
// gateway. That service hands each request's host and headers to Apply,
// and sends back the modified headers. The vendor docs, the dates they were
// read and what is verified are in docs/enterprise-accounts.md.
//
// Every header is set, never added to: a value the client sent is removed
// first, so a client cannot widen the list (the vendors ask for this).
package tenantheaders

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

// The documented header names.
const (
	// Microsoft Entra tenant restrictions v2, on the Microsoft sign-in hosts.
	MicrosoftHeader = "Sec-Restrict-Tenant-Access-Policy"
	// ChatGPT Enterprise corporate network controls.
	ChatGPTHeader = "ChatGPT-Allowed-Workspace-Id"
	// Anthropic tenant restrictions (claude.ai and the API), with
	// continuation headers anthropic-allowed-org-ids1 … -ids9.
	AnthropicHeader = "Anthropic-Allowed-Org-Ids"
	// Google Workspace: block consumer accounts.
	GoogleHeader = "X-GoogApps-Allowed-Domains"
	// GitHub Enterprise Cloud with managed users: enterprise access
	// restrictions.
	GitHubHeader = "Sec-GitHub-Allowed-Enterprise"
)

// Limits the vendors document.
const (
	maxAnthropicLines = 10
	maxAnthropicOrgs  = 500
	maxGitHubIDs      = 20
	// anthropicPerLine is how many organisation ids go on one header line.
	// The vendor sets no per-line limit; this keeps a line well under the
	// 8 KiB many servers allow for one header.
	anthropicPerLine = 50
)

// Config is what a deployment allows. A vendor left empty gets no header.
type Config struct {
	// MicrosoftTenantID and MicrosoftPolicyID are the Entra tenant and the
	// object id of its cross-tenant access policy; both or neither.
	MicrosoftTenantID string
	MicrosoftPolicyID string
	// ChatGPTWorkspaces are ChatGPT workspace ids (UUIDs).
	ChatGPTWorkspaces []string
	// AnthropicOrgs are Anthropic organisation ids (UUIDs).
	AnthropicOrgs []string
	// GoogleDomains are Workspace domains, and the vendor's keywords
	// (consumer_accounts, visitor_accounts) when wanted.
	GoogleDomains []string
	// GitHubEnterprises are GitHub enterprise ids.
	GitHubEnterprises []string
}

var (
	uuid       = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	domainOrKw = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)*$|^[a-z_]+$`)
	token      = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)
)

// Validate reports the first value that would not make a valid header.
func (c Config) Validate() error {
	if (c.MicrosoftTenantID == "") != (c.MicrosoftPolicyID == "") {
		return errors.New("microsoft: the tenant id and the policy id are set together")
	}
	if c.MicrosoftTenantID != "" && (!uuid.MatchString(c.MicrosoftTenantID) || !uuid.MatchString(c.MicrosoftPolicyID)) {
		return errors.New("microsoft: the tenant id and the policy id are UUIDs")
	}
	for _, v := range c.ChatGPTWorkspaces {
		if !uuid.MatchString(v) {
			return fmt.Errorf("chatgpt: workspace id %q is not a UUID", v)
		}
	}
	if len(c.AnthropicOrgs) > maxAnthropicOrgs {
		return fmt.Errorf("anthropic: %d organisation ids, the vendor allows %d", len(c.AnthropicOrgs), maxAnthropicOrgs)
	}
	for _, v := range c.AnthropicOrgs {
		if !uuid.MatchString(v) {
			return fmt.Errorf("anthropic: organisation id %q is not a UUID", v)
		}
	}
	for _, v := range c.GoogleDomains {
		if !domainOrKw.MatchString(v) {
			return fmt.Errorf("google: %q is not a lower-case domain or keyword", v)
		}
	}
	if len(c.GitHubEnterprises) > maxGitHubIDs {
		return fmt.Errorf("github: %d enterprise ids, the vendor allows %d", len(c.GitHubEnterprises), maxGitHubIDs)
	}
	for _, v := range c.GitHubEnterprises {
		if !token.MatchString(v) {
			return fmt.Errorf("github: enterprise id %q has characters outside letters, digits and . _ -", v)
		}
	}
	return nil
}

// Hosts on which each vendor reads its header, from the vendor docs.
var (
	microsoftHosts = []string{"login.live.com", "login.microsoft.com", "login.microsoftonline.com", "login.windows.net"}
	chatgptHosts   = []string{"chatgpt.com"}
	anthropicHosts = []string{"claude.ai", "api.anthropic.com", "claude.com", "anthropic.com"}
	githubHosts    = []string{"github.com", "api.github.com"}
)

// hostIs reports whether host is one of hosts, or under one when sub is set.
func hostIs(host string, hosts []string, sub bool) bool {
	for _, h := range hosts {
		if host == h || (sub && strings.HasSuffix(host, "."+h)) {
			return true
		}
	}
	return false
}

// normalHost lower-cases a host and takes off a port and a trailing dot.
func normalHost(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	if i := strings.LastIndexByte(host, ':'); i >= 0 && !strings.Contains(host[i:], "]") {
		host = host[:i]
	}
	return strings.TrimSuffix(strings.Trim(host, "[]"), ".")
}

// Apply sets on h the headers c calls for on a request to host, after
// removing any copy the client sent, and returns the names it set. A host
// no vendor reads a header on is left alone, and nil is returned. c is
// expected to have passed Validate.
func (c Config) Apply(host string, h http.Header) []string {
	host = normalHost(host)
	var set []string
	put := func(name, value string) {
		h.Del(name)
		h.Set(name, value)
		set = append(set, name)
	}
	switch {
	case hostIs(host, microsoftHosts, false):
		if c.MicrosoftTenantID != "" {
			put(MicrosoftHeader, c.MicrosoftTenantID+":"+c.MicrosoftPolicyID)
		}
	case hostIs(host, chatgptHosts, true):
		if len(c.ChatGPTWorkspaces) > 0 {
			// Comma-separated, no spaces.
			put(ChatGPTHeader, strings.Join(c.ChatGPTWorkspaces, ","))
		}
	case hostIs(host, anthropicHosts, true):
		if len(c.AnthropicOrgs) > 0 {
			set = append(set, c.applyAnthropic(h)...)
		}
	case host == "google.com" || strings.HasSuffix(host, ".google.com"):
		if len(c.GoogleDomains) > 0 {
			put(GoogleHeader, strings.Join(c.GoogleDomains, ","))
		}
	case hostIs(host, githubHosts, false) || strings.HasSuffix(host, ".githubcopilot.com"):
		if len(c.GitHubEnterprises) > 0 {
			put(GitHubHeader, strings.Join(c.GitHubEnterprises, ","))
		}
	}
	return set
}

// applyAnthropic writes the organisation ids over as many lines as they
// need. With more than one line, the first carries ";n=K", K being the
// number of lines, and the others are anthropic-allowed-org-ids1 … K-1.
// Every line the client may have sent is removed first: a stray
// continuation line makes the vendor answer 400.
func (c Config) applyAnthropic(h http.Header) []string {
	h.Del(AnthropicHeader)
	for i := 1; i < maxAnthropicLines; i++ {
		h.Del(fmt.Sprintf("%s%d", AnthropicHeader, i))
	}
	var lines []string
	for i := 0; i < len(c.AnthropicOrgs); i += anthropicPerLine {
		end := i + anthropicPerLine
		if end > len(c.AnthropicOrgs) {
			end = len(c.AnthropicOrgs)
		}
		lines = append(lines, strings.Join(c.AnthropicOrgs[i:end], ","))
	}
	if len(lines) > maxAnthropicLines {
		// Validate keeps the list within 500 ids, which is 10 lines.
		lines = lines[:maxAnthropicLines]
	}
	if len(lines) == 1 {
		h.Set(AnthropicHeader, lines[0])
		return []string{AnthropicHeader}
	}
	h.Set(AnthropicHeader, fmt.Sprintf("%s;n=%d", lines[0], len(lines)))
	names := []string{AnthropicHeader}
	for i, line := range lines[1:] {
		name := fmt.Sprintf("%s%d", AnthropicHeader, i+1)
		h.Set(name, line)
		names = append(names, name)
	}
	return names
}
