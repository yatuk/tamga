package proxy

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// The MCP gateway's configuration: which MCP servers Tamga stands in front
// of, and what it may send them. It is a file of its own, named by
// TAMGA_MCP_CONFIG, and not part of the policy. The policy can be changed
// through the API by people who are not the operator; this file names hosts
// to send traffic to and environment variables to send along, which is the
// operator's to decide (see policy.UpstreamRules for the same reasoning about
// pools). What is done with what the scanners find stays in the policy.
//
//	servers:
//	  - name: github
//	    url: https://mcp.example.com/mcp
//	    tools: [search_issues, "get_*"]
//	    headers_from_env:
//	      Authorization: TAMGA_MCP_GITHUB_AUTH
//	    tool_changes: warn
//
// It is read once, at startup.

// MCPGatewayConfig is the file TAMGA_MCP_CONFIG names.
type MCPGatewayConfig struct {
	Servers []MCPServerConfig `yaml:"servers"`
	// AllowedOrigins are the web origins whose pages may call the gateway.
	// A request with any other http(s) Origin is refused (403), as the
	// transport requires of a server against DNS rebinding.
	AllowedOrigins []string `yaml:"allowed_origins"`
}

// MCPServerConfig is one upstream MCP server, served at /mcp/<name>.
type MCPServerConfig struct {
	Name string `yaml:"name"`
	// URL is the server's MCP endpoint.
	URL string `yaml:"url"`
	// Tools, when set, are the only tools the agent is shown and may call.
	// "*" stands for any run of characters. Empty allows every tool.
	Tools []string `yaml:"tools"`
	// HeadersFromEnv are headers sent to the server, by header name, with
	// the value read from the named environment variable at startup. This
	// is how the server's own credential is given: Tamga is then the client
	// the server authorises.
	HeadersFromEnv map[string]string `yaml:"headers_from_env"`
	// ForwardAuthorization passes the agent's own Authorization header to
	// the server, and the server's WWW-Authenticate back. Off by default:
	// the header the agent sends Tamga is not taken to be meant for the
	// server.
	ForwardAuthorization bool `yaml:"forward_authorization"`
	// ToolChanges is what happens when a tool's definition changes after it
	// was first listed in a session: off, warn (the default) or block.
	ToolChanges string `yaml:"tool_changes"`
	// MaxMessageBytes is the largest JSON-RPC message read in either
	// direction. A larger one is not forwarded. Default 4 MiB.
	MaxMessageBytes int `yaml:"max_message_bytes"`

	// headers are HeadersFromEnv resolved.
	headers map[string]string
	target  *url.URL
}

// Values of tool_changes.
const (
	mcpToolChangesOff   = "off"
	mcpToolChangesWarn  = "warn"
	mcpToolChangesBlock = "block"
)

// mcpDefaultMaxMessage is max_message_bytes when it is not set, and
// mcpCeilingMaxMessage the most it may be.
const (
	mcpDefaultMaxMessage = 4 << 20
	mcpCeilingMaxMessage = 32 << 20
)

// mcpServerName is the form of a server's name: it is a path segment.
var mcpServerName = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,62}$`)

// mcpHeaderToken is an HTTP field name (RFC 9110 section 5.1).
var mcpHeaderToken = regexp.MustCompile("^[!#$%&'*+.^_`|~0-9A-Za-z-]+$")

// mcpReservedHeaders may not be set through headers_from_env: they belong to
// the transport, to Tamga, or to the connection.
var mcpReservedHeaders = map[string]bool{
	"accept": true, "content-type": true, "content-length": true, "host": true, "connection": true,
	"transfer-encoding": true, "te": true, "upgrade": true, "trailer": true, "keep-alive": true,
	"mcp-protocol-version": true, "mcp-session-id": true, "mcp-method": true, "mcp-name": true, "last-event-id": true,
}

// LoadMCPGatewayConfig reads and checks the file. Every problem fails the
// load: a gateway that starts with part of its servers missing is one an
// agent finds out about by its tools going away.
func LoadMCPGatewayConfig(file string) (*MCPGatewayConfig, error) {
	raw, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("mcp gateway config: %w", err)
	}
	return parseMCPGatewayConfig(raw, os.Getenv)
}

func parseMCPGatewayConfig(raw []byte, getenv func(string) string) (*MCPGatewayConfig, error) {
	var c MCPGatewayConfig
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("mcp gateway config: %w", err)
	}
	if len(c.Servers) == 0 {
		return nil, errors.New("mcp gateway config: no servers")
	}
	seen := map[string]bool{}
	for i := range c.Servers {
		s := &c.Servers[i]
		where := fmt.Sprintf("mcp gateway config: servers[%d]", i)
		if !mcpServerName.MatchString(s.Name) {
			return nil, fmt.Errorf("%s: name must be 1-63 lower-case letters, digits, '-' or '_'", where)
		}
		if seen[s.Name] {
			return nil, fmt.Errorf("%s: name %q is used twice", where, s.Name)
		}
		seen[s.Name] = true
		where += " (" + s.Name + ")"

		u, err := url.Parse(s.URL)
		// The URL is not repeated in an error: it can carry credentials.
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil ||
			u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" {
			return nil, fmt.Errorf("%s: url must be an absolute http(s) URL without credentials, query or fragment", where)
		}
		s.target = u

		for _, t := range s.Tools {
			if strings.TrimSpace(t) == "" {
				return nil, fmt.Errorf("%s: tools holds an empty entry", where)
			}
		}

		s.headers = map[string]string{}
		for name, env := range s.HeadersFromEnv {
			lower := strings.ToLower(name)
			if !mcpHeaderToken.MatchString(name) || mcpReservedHeaders[lower] || strings.HasPrefix(lower, "x-tamga-") ||
				strings.HasPrefix(lower, "mcp-param-") {
				return nil, fmt.Errorf("%s: headers_from_env: %q cannot be set", where, name)
			}
			v := getenv(env)
			if v == "" {
				return nil, fmt.Errorf("%s: headers_from_env: the environment variable %q for %s is empty or not set", where, env, name)
			}
			if strings.ContainsAny(v, "\r\n\x00") {
				return nil, fmt.Errorf("%s: headers_from_env: the value of %q is not a header value", where, env)
			}
			s.headers[name] = v
		}
		if s.ForwardAuthorization {
			for name := range s.headers {
				if strings.EqualFold(name, "Authorization") {
					return nil, fmt.Errorf("%s: forward_authorization and an Authorization header from the environment cannot both be set", where)
				}
			}
		}

		switch s.ToolChanges = strings.ToLower(strings.TrimSpace(s.ToolChanges)); s.ToolChanges {
		case "":
			s.ToolChanges = mcpToolChangesWarn
		case mcpToolChangesOff, mcpToolChangesWarn, mcpToolChangesBlock:
		default:
			return nil, fmt.Errorf("%s: tool_changes must be off, warn or block", where)
		}

		switch {
		case s.MaxMessageBytes == 0:
			s.MaxMessageBytes = mcpDefaultMaxMessage
		case s.MaxMessageBytes < 1024 || s.MaxMessageBytes > mcpCeilingMaxMessage:
			return nil, fmt.Errorf("%s: max_message_bytes must be between 1024 and %d", where, mcpCeilingMaxMessage)
		}
	}
	for _, o := range c.AllowedOrigins {
		if u, err := url.Parse(o); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.Path != "" {
			return nil, fmt.Errorf("mcp gateway config: allowed_origins: %q is not an origin (scheme://host[:port])", o)
		}
	}
	return &c, nil
}

// toolAllowed reports whether the server's tools list lets the agent see and
// call the tool.
func (s *MCPServerConfig) toolAllowed(name string) bool {
	if len(s.Tools) == 0 {
		return true
	}
	for _, p := range s.Tools {
		if globMatch(p, name) {
			return true
		}
	}
	return false
}

// globMatch matches name against a pattern in which "*" is any run of
// characters and every other character stands for itself.
func globMatch(pattern, name string) bool {
	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return pattern == name
	}
	if !strings.HasPrefix(name, parts[0]) {
		return false
	}
	name = name[len(parts[0]):]
	for i, p := range parts[1:] {
		last := i == len(parts)-2
		if last {
			return strings.HasSuffix(name, p)
		}
		j := strings.Index(name, p)
		if j < 0 {
			return false
		}
		name = name[j+len(p):]
	}
	return true
}
