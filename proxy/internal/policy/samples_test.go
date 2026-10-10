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
// sample.
var policyTopLevelKeys = map[string]bool{
	"version": true, "name": true, "rules": true, "output_rules": true, "rate_limit": true,
	"providers": true, "custom_entities": true, "competitors": true, "governance": true,
	"data": true, "cost": true, "cache": true, "body_limits": true,
	"exceptions": true, "operator_state": true, "vault": true, "canary": true, "scan": true,
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
			isPolicy := true
			for k := range top {
				if !policyTopLevelKeys[k] {
					isPolicy = false
				}
			}
			if isPolicy {
				out = append(out, docSample{file: f, line: start, text: text})
			}
		}
	}
	return out
}

// Every policy sample in the documentation must still load: a bound that
// refuses what the docs tell operators to write would stop their proxy.
func TestDocPolicySamplesLoad(t *testing.T) {
	samples := docPolicySamples(t)
	if len(samples) == 0 {
		t.Skip("no docs directory next to the module")
	}
	for _, s := range samples {
		if _, err := LoadFromBytes([]byte(s.text)); err != nil {
			t.Errorf("%s:%d does not load: %v", s.file, s.line, err)
		}
	}
	t.Logf("%d policy samples loaded", len(samples))
}
