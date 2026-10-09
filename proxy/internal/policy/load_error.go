package policy

// IssuesError is what LoadFromBytes returns for a policy that decodes but
// holds values the proxy refuses: a pool endpoint the upstream rules do not
// allow, or a value outside the bounds of bounds.go. It carries every such
// issue, so an editor can show them all by field; Error names the first, as
// the load error always did.
type IssuesError struct {
	Issues []ValidationIssue
}

func (e *IssuesError) Error() string {
	if e == nil || len(e.Issues) == 0 {
		return "parsing policy YAML: invalid policy"
	}
	return "parsing policy YAML: " + e.Issues[0].Field + ": " + e.Issues[0].Message
}

// loadBlockingIssues are the issues that fail a load, in the order they
// were always checked: the pools first, then the bounds.
func loadBlockingIssues(p *Policy) []ValidationIssue {
	var out []ValidationIssue
	out = append(out, upstreamIssues(p, ActiveUpstreamRules())...)
	for _, is := range boundsIssues(p) {
		if is.Severity == "error" {
			out = append(out, is)
		}
	}
	return out
}
