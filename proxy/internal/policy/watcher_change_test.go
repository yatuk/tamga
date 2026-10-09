package policy

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// The watcher hands the policy before and after a reload to onChange, which
// is how an edit of the file on disk gets into the audit log.
func TestWatchPolicyChanges_ReportsBeforeAndAfter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tamga-policy.yaml")
	v1 := "version: \"1.0\"\nname: watched\nproviders:\n  allowed: [openai]\n"
	if err := os.WriteFile(path, []byte(v1), 0o600); err != nil {
		t.Fatal(err)
	}
	p0, err := LoadFromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	store := NewPolicyStore(p0)

	type pair struct{ old, new *Policy }
	seen := make(chan pair, 8)
	stop, err := WatchPolicyChanges(path, store, nil, func(old, new *Policy) { seen <- pair{old, new} })
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	v2 := "version: \"1.0\"\nname: watched\nproviders:\n  allowed: [openai, anthropic]\nvault:\n  enabled: true\n"
	if err := os.WriteFile(path, []byte(v2), 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-seen:
		if got.old != p0 {
			t.Error("old is not the policy that was active before the reload")
		}
		if got.new != store.GetPolicy() {
			t.Error("new is not the policy now active")
		}
		want := []string{"providers.allowed", "vault.enabled"}
		if paths := ChangedPaths(Diff(got.old, got.new)); !reflect.DeepEqual(paths, want) {
			t.Errorf("diff = %v, want %v", paths, want)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("onChange was not called")
	}
}
