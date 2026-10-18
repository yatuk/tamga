package proxy

import (
	"strings"

	"github.com/yatuk/tamga/internal/extract"
	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/scanner"
)

// toolDestinationFindings holds the tool calls of an answer to
// output_rules.tool_destinations: each place an argument names that the
// policy does not allow for its tool is a finding, placed on the address in
// the argument (policy/tool_destinations.go). It is called for a buffered
// answer's segments and for each value a stream guard judges whole, so a
// tool call is held to the list whether it was streamed or not.
func toolDestinationFindings(pol *policy.Policy, segs []extract.Segment) []scanner.Finding {
	rules := pol.ToolDestinationRules()
	if rules == nil {
		return nil
	}
	var out []scanner.Finding
	for i, s := range segs {
		if s.Kind != extract.KindToolArgs || s.Text == "" || !rules.Checks(s.Tool) {
			continue
		}
		key := argumentKey(s.Path)
		if s.Literal == extract.LiteralKey {
			// A key is not a value under the key its path ends in.
			key = ""
		}
		for _, d := range rules.Disallowed(s.Tool, key, s.Text) {
			meta := map[string]string{"kind": d.Kind}
			if s.Tool != "" {
				meta["tool"] = s.Tool
			}
			out = append(out, scanner.Finding{
				Type:           scanner.ToolDestinationFindingType,
				Category:       scanner.DestinationNotAllowed,
				Severity:       "high",
				Match:          d.Host,
				StartPos:       d.Start,
				EndPos:         d.End,
				Confidence:     1.0,
				Seg:            i,
				Role:           string(s.Role),
				Path:           s.Path,
				Metadata:       meta,
				ScannerVersion: scanner.ScannerVersions[scanner.ToolDestinationFindingType],
			})
		}
	}
	return out
}

// argumentKey is the name of the argument a segment holds: the last member
// of its path, without the indexes of a list ("…arguments#to[1]" is "to").
func argumentKey(path string) string {
	for strings.HasSuffix(path, "]") {
		open := strings.LastIndexByte(path, '[')
		if open < 0 {
			break
		}
		path = path[:open]
	}
	if i := strings.LastIndexAny(path, ".#"); i >= 0 {
		return path[i+1:]
	}
	return path
}

// withToolDestinations adds the tool_destination findings to a stream
// guard's segment scanner.
func withToolDestinations(pol *policy.Policy, scan func([]extract.Segment) ([]scanner.Finding, error)) func([]extract.Segment) ([]scanner.Finding, error) {
	if pol.ToolDestinationRules() == nil {
		return scan
	}
	return func(segs []extract.Segment) ([]scanner.Finding, error) {
		found, err := scan(segs)
		return append(found, toolDestinationFindings(pol, segs)...), err
	}
}
