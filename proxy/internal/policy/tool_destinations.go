package policy

import (
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/yatuk/tamga/internal/scanner"
	"github.com/yatuk/tamga/internal/scanner/linkscan"
)

// A model that has read an injected instruction does its harm through a
// tool: it mails the conversation, posts it to a webhook, fetches a URL with
// the data in it. A text classifier has to recognise the request to do
// that; a list of where each tool may send data does not. It holds the
// tool call itself, whatever text led the model there.
//
//	output_rules:
//	  enabled: true
//	  block_on: [tool_destination]
//	  tool_destinations:
//	    - tools: [send_email, "mcp__mail__*"]
//	      allow: [example.com]                 # recipients @example.com and @*.example.com
//	    - tools: [http_request, fetch_url]
//	      allow: [api.example.com, "*.internal.example"]
//
// Every tool call in an answer whose tool name matches an entry's tools is
// read for the places its arguments name: the host of every address with an
// authority (https://, sftp://, any scheme), the domain of every e-mail
// address, and a value under a key that names a place (host, to, url,
// webhook and the like, see destinationKeys) that is a host name or an IP
// address. One that no matching entry allows is a tool_destination finding
// (category destination_not_allowed), acted on by block_on and redact_on
// like any finding of an answer: in block_on the answer is withheld, or
// the stream ends before the tool call reaches the client; in redact_on the
// address is masked in the arguments. A tool that no entry names is not
// checked; a tool call whose name the proxy cannot read is checked only
// against an entry whose tools hold "*".
//
// Nothing is resolved or fetched. The list is as good as its entries: an
// allowed host that forwards or redirects is a way out.

// ToolDestination is one entry of output_rules.tool_destinations.
type ToolDestination struct {
	// Tools are tool names, matched whole. A "*" in one stands for any run
	// of characters: "mcp__mail__*", "*_send", "*".
	Tools []string `yaml:"tools" json:"tools"`
	// Allow are the hosts and e-mail domains the tools may send to, written
	// as output_rules.links entries are: an entry covers the host and every
	// host under it, "*.example.com" only what is under it, an IP address
	// that address alone. An empty list allows nowhere.
	Allow []string `yaml:"allow" json:"allow"`
}

// Limits on tool_destinations.
const (
	MaxToolDestinations       = 100
	MaxToolDestinationTools   = 100
	MaxToolDestinationPattern = 128
)

// destinationKeys are the argument names whose value, when it is a host
// name or an IP address and nothing else, is read as a place to send to.
// Elsewhere a dotted word is a file name ("report.pdf") as often as a host.
var destinationKeys = map[string]bool{
	"host": true, "hostname": true, "server": true, "domain": true, "endpoint": true,
	"address": true, "addr": true, "ip": true, "target": true, "remote": true, "destination": true,
	"url": true, "uri": true, "webhook": true, "webhook_url": true, "callback": true, "callback_url": true,
	"to": true, "cc": true, "bcc": true, "recipient": true, "recipients": true, "email": true,
}

// ToolDestinationRules are the compiled entries.
type ToolDestinationRules struct {
	entries []compiledDestination
}

type compiledDestination struct {
	tools []string
	allow *linkscan.HostList
}

type compiledDestinations struct {
	from  *OutputRules
	rules *ToolDestinationRules
}

var lastDestinations atomic.Pointer[compiledDestinations]

// ToolDestinationRules returns the compiled tool_destinations, nil when the
// policy has none or output_rules is off. A policy that loaded has entries
// that compile; for one built in code with a bad entry the result is nil.
func (p *Policy) ToolDestinationRules() *ToolDestinationRules {
	if p == nil || p.OutputRules == nil || !p.OutputRules.Enabled || len(p.OutputRules.ToolDestinations) == 0 {
		return nil
	}
	or := p.OutputRules
	if c := lastDestinations.Load(); c != nil && c.from == or {
		return c.rules
	}
	rules := &ToolDestinationRules{}
	for _, d := range or.ToolDestinations {
		allow, err := linkscan.NewHostList(d.Allow)
		if err != nil {
			rules = nil
			break
		}
		rules.entries = append(rules.entries, compiledDestination{tools: d.Tools, allow: allow})
	}
	lastDestinations.Store(&compiledDestinations{from: or, rules: rules})
	return rules
}

// Checks reports whether an entry names the tool. An empty name, a tool
// call whose name is not known, is named only by "*".
func (r *ToolDestinationRules) Checks(tool string) bool {
	if r == nil {
		return false
	}
	for _, e := range r.entries {
		if e.names(tool) {
			return true
		}
	}
	return false
}

func (e compiledDestination) names(tool string) bool {
	for _, pat := range e.tools {
		if pat == "*" || (tool != "" && globMatch(strings.TrimSpace(pat), tool)) {
			return true
		}
	}
	return false
}

// Disallowed returns the places an argument of a call to tool names that
// no entry naming the tool allows. key is the argument's name, the last
// part of its path; text is its value.
func (r *ToolDestinationRules) Disallowed(tool, key, text string) []linkscan.Destination {
	if !r.Checks(tool) {
		return nil
	}
	found := linkscan.Destinations(text)
	if len(found) == 0 && destinationKeys[strings.ToLower(key)] {
		if host, ok := linkscan.HostValue(text); ok {
			start := strings.Index(text, strings.TrimSpace(text))
			found = append(found, linkscan.Destination{Start: start, End: start + len(strings.TrimSpace(text)), Host: host, Kind: linkscan.DestHost})
		}
	}
	var out []linkscan.Destination
	for _, d := range found {
		if !r.allows(tool, d.Host) {
			out = append(out, d)
		}
	}
	return out
}

func (r *ToolDestinationRules) allows(tool, host string) bool {
	for _, e := range r.entries {
		if e.names(tool) && e.allow.Has(host) {
			return true
		}
	}
	return false
}

// globMatch matches name against a pattern in which "*" stands for any run
// of characters, and every other character for itself.
func globMatch(pattern, name string) bool {
	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return pattern == name
	}
	if !strings.HasPrefix(name, parts[0]) {
		return false
	}
	name = name[len(parts[0]):]
	for _, mid := range parts[1 : len(parts)-1] {
		i := strings.Index(name, mid)
		if i < 0 {
			return false
		}
		name = name[i+len(mid):]
	}
	return strings.HasSuffix(name, parts[len(parts)-1])
}

// toolDestinationIssues covers output_rules.tool_destinations.
func toolDestinationIssues(p *Policy) []ValidationIssue {
	or := p.OutputRules
	if or == nil || len(or.ToolDestinations) == 0 {
		return nil
	}
	const field = "output_rules.tool_destinations"
	var issues []ValidationIssue
	if n := len(or.ToolDestinations); n > MaxToolDestinations {
		issues = append(issues, boundError(field, "too_many", "%d entries, the limit is %d", n, MaxToolDestinations))
	}
	for i, d := range or.ToolDestinations {
		at := fmt.Sprintf("%s[%d]", field, i)
		if len(d.Tools) == 0 {
			issues = append(issues, boundError(at+".tools", "required", "an entry names no tool: it would check nothing"))
		}
		if n := len(d.Tools); n > MaxToolDestinationTools {
			issues = append(issues, boundError(at+".tools", "too_many", "%d tools, the limit is %d", n, MaxToolDestinationTools))
		}
		for k, t := range d.Tools {
			if strings.TrimSpace(t) == "" || len(t) > MaxToolDestinationPattern {
				issues = append(issues, boundError(fmt.Sprintf("%s.tools[%d]", at, k), "pattern", "a tool name is empty or longer than %d bytes", MaxToolDestinationPattern))
			}
		}
		if n := len(d.Allow); n > linkscan.MaxAllowHosts {
			issues = append(issues, boundError(at+".allow", "too_many", "%d hosts, the limit is %d", n, linkscan.MaxAllowHosts))
		}
		for k, h := range d.Allow {
			if err := linkscan.CheckHost(h); err != nil {
				issues = append(issues, boundError(fmt.Sprintf("%s.allow[%d]", at, k), "host", "%v", err))
			}
		}
	}
	if !or.Enabled {
		issues = append(issues, boundWarning(field, "no_effect", "tool_destinations has no effect while output_rules.enabled is false: answers are not scanned"))
	} else if !containsOutputKey(or.BlockOn, scanner.ToolDestinationFindingType, scanner.DestinationNotAllowed) &&
		!containsOutputKey(or.RedactOn, scanner.ToolDestinationFindingType, scanner.DestinationNotAllowed) {
		issues = append(issues, boundWarning(field, "record_only", "neither block_on nor redact_on names tool_destination: a tool call to a place no entry allows is recorded and let through"))
	}
	if or.Enabled && (or.Streaming == nil || !or.Streaming.Enabled) {
		issues = append(issues, boundWarning(field, "buffered_only", "output_rules.streaming is off: the tool calls of a streamed answer are not checked"))
	}
	return issues
}
