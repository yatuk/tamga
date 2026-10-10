package buildinfo

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestDescribe(t *testing.T) {
	const rev = "3f9c2ab41d07e5a8b6c4d2e0f1a3b5c7d9e8f0a1"
	stamped := func(r string, dirty bool) func() (string, bool) {
		return func() (string, bool) { return r, dirty }
	}
	cases := []struct {
		name              string
		version, revision string
		vcs               func() (string, bool)
		want              string
	}{
		{"a release names itself", "v0.9.0", rev, stamped(rev, true), "v0.9.0"},
		{"a release without a revision", "v0.9.0", "", nil, "v0.9.0"},
		{"nothing known", "", "", stamped("", false), "dev"},
		{"no build info at all", "", "", nil, "dev"},
		{"the revision the build passed", "", rev, stamped("ffffffffffffffff", true), "dev+3f9c2ab41d07"},
		{"the revision the Go tool stamped", "", "", stamped(rev, false), "dev+3f9c2ab41d07"},
		{"uncommitted changes", "", "", stamped(rev, true), "dev+3f9c2ab41d07.dirty"},
		{"a short revision is kept whole", "", "3f9c2ab", nil, "dev+3f9c2ab"},
	}
	for _, c := range cases {
		if got := describe(c.version, c.revision, c.vcs); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

// A binary that was not given a version does not report a release. The test
// binary is one: no -ldflags set Version (the run that checks -X sets
// TAMGA_TEST_WANT_VERSION, see TestLinkerSetsTheVersion).
func TestStringOfAnUnnamedBuild(t *testing.T) {
	if os.Getenv("TAMGA_TEST_WANT_VERSION") != "" {
		t.Skip("this run links a version in")
	}
	if Version != "" {
		t.Fatalf("Version is %q in the source: it is set by the linker only", Version)
	}
	if got := String(); got != "dev" && !strings.HasPrefix(got, "dev+") {
		t.Fatalf("String() = %q for a build without a version, want dev or dev+<revision>", got)
	}
}

// Run with the flags a release build uses to see that they reach String:
//
//	TAMGA_TEST_WANT_VERSION=v9.9.9 go test ./internal/buildinfo \
//	  -ldflags "-X github.com/yatuk/tamga/internal/buildinfo.Version=v9.9.9"
func TestLinkerSetsTheVersion(t *testing.T) {
	want := os.Getenv("TAMGA_TEST_WANT_VERSION")
	if want == "" {
		t.Skip("TAMGA_TEST_WANT_VERSION is not set")
	}
	if got := String(); got != want {
		t.Fatalf("String() = %q, want %q: the -X path does not match this package", got, want)
	}
}

var versionLiteral = regexp.MustCompile(`^"v?\d+\.\d+\.\d+[-+.\w]*"$`)

// versionNames are the identifiers and keys under which a release version of
// the binary would be written down by hand.
func versionName(name string) bool {
	switch strings.ToLower(name) {
	case "version", "appversion", "serviceversion", "buildversion", "proxyversion", "tamgaversion":
		return true
	}
	return false
}

// The binary's version is written nowhere in the module: main.go said
// "v0.1.0" for the banner and the health endpoint and "v0.1.1" for tracing
// while the repository was at v0.9.0. A literal that looks like a release
// version, assigned to something called a version or passed where one is
// expected, fails here.
func TestNoVersionIsWrittenByHand(t *testing.T) {
	root := filepath.Join("..", "..")
	fset := token.NewFileSet()
	checked := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if n := d.Name(); n == "testdata" || n == "bin" || n == "data" || n == "proto" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		checked++
		report := func(lit *ast.BasicLit, what string) {
			t.Errorf("%s: %s is written as %s; take it from buildinfo.String()", fset.Position(lit.Pos()), what, lit.Value)
		}
		isVersion := func(e ast.Expr) (*ast.BasicLit, bool) {
			lit, ok := e.(*ast.BasicLit)
			return lit, ok && lit.Kind == token.STRING && versionLiteral.MatchString(lit.Value)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.AssignStmt:
				for i, lhs := range n.Lhs {
					id, ok := lhs.(*ast.Ident)
					if !ok || i >= len(n.Rhs) || !versionName(id.Name) {
						continue
					}
					if lit, ok := isVersion(n.Rhs[i]); ok {
						report(lit, id.Name)
					}
				}
			case *ast.ValueSpec:
				for i, id := range n.Names {
					if i >= len(n.Values) || !versionName(id.Name) {
						continue
					}
					if lit, ok := isVersion(n.Values[i]); ok {
						report(lit, id.Name)
					}
				}
			case *ast.KeyValueExpr:
				name := ""
				switch k := n.Key.(type) {
				case *ast.Ident:
					name = k.Name
				case *ast.BasicLit:
					name = strings.Trim(k.Value, `"`)
				}
				if lit, ok := isVersion(n.Value); ok && versionName(name) {
					report(lit, name)
				}
			case *ast.CallExpr:
				// telemetry.ConfigFromEnv("tamga-proxy", "v0.1.1") and the like.
				sel, ok := n.Fun.(*ast.SelectorExpr)
				if !ok || !strings.Contains(sel.Sel.Name, "Config") && !strings.Contains(sel.Sel.Name, "Version") {
					return true
				}
				for _, arg := range n.Args {
					if lit, ok := isVersion(arg); ok {
						report(lit, "an argument of "+sel.Sel.Name)
					}
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if checked < 50 {
		t.Fatalf("only %d files were read: the walk no longer covers the module", checked)
	}
}
