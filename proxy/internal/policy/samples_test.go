package policy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// policyTopLevelKeys are the keys a policy document may start with. A fenced
// block in the docs whose top-level keys are all among them is a policy
// sample. So is one that has a key only a policy has (policyOnlyKeys) next
// to a key no policy has: that is a sample with a mistake in it, and
// leaving it out would be the one way a wrong sample passed.
var policyTopLevelKeys = map[string]bool{
	"version": true, "name": true, "rules": true, "output_rules": true, "rate_limit": true,
	"providers": true, "custom_entities": true, "competitors": true, "governance": true,
	"data": true, "cost": true, "cache": true, "body_limits": true,
	"exceptions": true, "operator_state": true, "vault": true, "canary": true, "scan": true,
}

// policyOnlyKeys are top-level keys that say "this is a policy" on their
// own. version and name are not among them: a compose file and a workflow
// start with those too.
var policyOnlyKeys = map[string]bool{
	"output_rules": true, "custom_entities": true, "competitors": true,
	"body_limits": true, "operator_state": true, "vault": true, "canary": true,
	"exceptions": true, "rate_limit": true,
}

// docFilesWithSamples are the documents an operator copies policy from. The
// walk below reads every Markdown file under docs/ and the README; these are
// named so that moving or renaming one cannot quietly take its samples out
// of the test. The number is the least count of policy samples in it.
var docFilesWithSamples = map[string]int{
	"README.md":                   1,
	"docs/operations.md":          1,
	"docs/agents.md":              1,
	"docs/faq.md":                 0,
	"docs/use-cases.md":           0,
	"docs/architecture/README.md": 0,
}

type docSample struct {
	file string
	line int
	text string
}

// docPolicySamples returns every fenced yaml block under docs/ and in the
// README that reads as a policy or a part of one.
func docPolicySamples(t *testing.T) []docSample {
	t.Helper()
	root := filepath.Join("..", "..", "..")
	files := []string{filepath.Join(root, "README.md")}
	_ = filepath.Walk(filepath.Join(root, "docs"), func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(path, ".md") {
			files = append(files, path)
		}
		return nil
	})
	var out []docSample
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
		for i := 0; i < len(lines); i++ {
			open := strings.TrimSpace(lines[i])
			if open != "```yaml" && open != "```yml" {
				continue
			}
			start := i + 1
			var block []string
			for i++; i < len(lines) && strings.TrimSpace(lines[i]) != "```"; i++ {
				block = append(block, lines[i])
			}
			text := strings.Join(block, "\n")
			var top map[string]interface{}
			if err := yaml.Unmarshal([]byte(text), &top); err != nil || len(top) == 0 {
				continue
			}
			isPolicy, policyOnly := true, false
			for k := range top {
				if !policyTopLevelKeys[k] {
					isPolicy = false
				}
				if policyOnlyKeys[k] {
					policyOnly = true
				}
			}
			if isPolicy || policyOnly {
				out = append(out, docSample{file: f, line: start, text: text})
			}
		}
	}
	return out
}

// Every policy sample in the documentation must still load: a bound that
// refuses what the docs tell operators to write would stop their proxy. It
// must also pass the checks an installed policy passes (ValidateSemantics):
// a sample the editor would refuse is as wrong as one that does not parse.
func TestDocPolicySamplesLoad(t *testing.T) {
	samples := docPolicySamples(t)
	if len(samples) == 0 {
		t.Skip("no docs directory next to the module")
	}
	// The worked example of ROUTE in docs/operations.md has the operator set
	// TAMGA_UPSTREAM_ALLOW=10.0.4.12:8000 before the policy is written, and
	// its sample loads only with that. Nothing else is listed here, so every
	// other sample is still judged by the strict default.
	t.Cleanup(SetUpstreamRules(&UpstreamRules{AllowHostPorts: []string{"10.0.4.12:8000"}}))
	root := filepath.Join("..", "..", "..")
	perFile := map[string]int{}
	for _, s := range samples {
		rel, _ := filepath.Rel(root, s.file)
		perFile[filepath.ToSlash(rel)]++
		p, err := LoadFromBytes([]byte(s.text))
		if err != nil {
			t.Errorf("%s:%d does not load: %v", s.file, s.line, err)
			continue
		}
		for _, is := range ValidateSemantics(p) {
			// A sample is often a part of a policy, and a part has no name.
			if is.Severity != "error" || (is.Field == "name" && is.Rule == "required") {
				continue
			}
			t.Errorf("%s:%d is refused by validation: %s: %s", s.file, s.line, is.Field, is.Message)
		}
	}
	for file, least := range docFilesWithSamples {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(file))); err != nil {
			t.Errorf("%s is not where this test reads it: %v", file, err)
			continue
		}
		if perFile[file] < least {
			t.Errorf("%s: %d policy samples found, expected at least %d", file, perFile[file], least)
		}
	}
	t.Logf("%d policy samples loaded: %v", len(samples), perFile)
}
