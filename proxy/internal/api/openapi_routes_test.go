package api

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// openAPIOperation is what the test reads of one operation in the document.
type openAPIOperation struct {
	Role       string `yaml:"x-required-role"`
	Permission string `yaml:"x-required-permission"`
}

// openAPINotManagement are the paths the document describes that the
// management API's route table does not serve: the proxy's own listener
// registers them.
var openAPINotManagement = map[string]bool{
	"/health":              true,
	"/v1/chat/completions": true,
}

// loadOpenAPIOperations reads docs/openapi.yaml and returns its operations
// under /api/v1, keyed the way the route table names a route: "GET /stats".
func loadOpenAPIOperations(t *testing.T) map[string]openAPIOperation {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "docs", "openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Paths map[string]map[string]yaml.Node `yaml:"paths"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("docs/openapi.yaml does not parse: %v", err)
	}
	methods := map[string]bool{"get": true, "post": true, "put": true, "patch": true, "delete": true, "head": true, "options": true}
	ops := map[string]openAPIOperation{}
	for path, item := range doc.Paths {
		if !strings.HasPrefix(path, "/api/v1/") {
			if !openAPINotManagement[path] {
				t.Errorf("docs/openapi.yaml: %s is outside /api/v1 and not in openAPINotManagement", path)
			}
			continue
		}
		for method, node := range item {
			if !methods[method] {
				// "parameters", "summary" and the like, shared by the path.
				continue
			}
			var op openAPIOperation
			if err := node.Decode(&op); err != nil {
				t.Fatalf("docs/openapi.yaml: %s %s: %v", method, path, err)
			}
			ops[strings.ToUpper(method)+" "+strings.TrimPrefix(path, "/api/v1")] = op
		}
	}
	if len(ops) == 0 {
		t.Fatal("docs/openapi.yaml: no operation under /api/v1")
	}
	return ops
}

// The OpenAPI document and the route table describe the same API: a route
// without an operation is one nobody can find or generate a client for, and
// an operation without a route is a promise the proxy does not keep. What the
// document says a route needs is what the table asks for.
func TestOpenAPIDescribesEveryRoute(t *testing.T) {
	ops := loadOpenAPIOperations(t)

	var problems []string
	served := map[string]bool{}
	for _, rt := range fullTable() {
		p := rt.pattern()
		served[p] = true
		op, ok := ops[p]
		if !ok {
			problems = append(problems, p+" is in routes.go and has no operation in docs/openapi.yaml")
			continue
		}
		if rt.public != "" {
			if op.Role != "" || op.Permission != "" {
				problems = append(problems, p+" is public; docs/openapi.yaml says it needs "+op.Role+"/"+op.Permission)
			}
			continue
		}
		if op.Permission != rt.scope {
			problems = append(problems, p+": x-required-permission is \""+op.Permission+"\", routes.go asks for \""+rt.scope+"\"")
		}
		if want := minimumRole(rt.scope); op.Role != want {
			problems = append(problems, p+": x-required-role is \""+op.Role+"\", the least role with "+rt.scope+" is \""+want+"\"")
		}
	}
	for p := range ops {
		if !served[p] {
			problems = append(problems, p+" has an operation in docs/openapi.yaml and no route in routes.go")
		}
	}
	sort.Strings(problems)
	for _, p := range problems {
		t.Error(p)
	}
}
