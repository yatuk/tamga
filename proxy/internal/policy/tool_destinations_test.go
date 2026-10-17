package policy

import "testing"

func TestGlobMatch(t *testing.T) {
	for _, c := range []struct {
		pattern, name string
		want          bool
	}{
		{"send_email", "send_email", true},
		{"send_email", "send_emails", false},
		{"mcp__mail__*", "mcp__mail__send", true},
		{"mcp__mail__*", "mcp__drive__send", false},
		{"*_send", "slack_send", true},
		{"*mail*", "gmail_send", true},
		{"a*b*c", "aXbYc", true},
		{"a*b*c", "aXcYb", false},
		{"*", "anything", true},
	} {
		if got := globMatch(c.pattern, c.name); got != c.want {
			t.Errorf("globMatch(%q, %q) = %v", c.pattern, c.name, got)
		}
	}
}

func TestToolDestinationRules(t *testing.T) {
	p, err := LoadFromBytes([]byte(`
version: "1.0"
output_rules:
  enabled: true
  block_on: [tool_destination]
  streaming: {enabled: true}
  tool_destinations:
    - tools: [send_email]
      allow: [example.com]
    - tools: [send_email, upload]
      allow: [files.example.net]
`))
	if err != nil {
		t.Fatal(err)
	}
	r := p.ToolDestinationRules()
	if r == nil || !r.Checks("send_email") || !r.Checks("upload") || r.Checks("search") || r.Checks("") {
		t.Fatalf("rules = %+v", r)
	}
	// Entries that name the same tool add up.
	if got := r.Disallowed("send_email", "to", "a@example.com, b@files.example.net, c@leak.example"); len(got) != 1 || got[0].Host != "leak.example" {
		t.Fatalf("Disallowed = %+v", got)
	}
	if got := r.Disallowed("upload", "to", "a@example.com"); len(got) != 1 {
		t.Fatalf("upload may not send to example.com: %+v", got)
	}
	// A bare host is read under a key that names a place, and not elsewhere.
	if got := r.Disallowed("upload", "host", "leak.example"); len(got) != 1 || got[0].Kind != "host" {
		t.Fatalf("host value: %+v", got)
	}
	if got := r.Disallowed("upload", "file_name", "report.pdf"); len(got) != 0 {
		t.Fatalf("a file name was read as a host: %+v", got)
	}
	// Off with output_rules.
	p.OutputRules.Enabled = false
	if p.ToolDestinationRules() != nil {
		t.Fatal("rules while output_rules is off")
	}
}
