package policy

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWatchPolicy_ReloadsOnFileChange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tamga-policy.yaml")

	v1 := []byte(`
version: "1.0"
name: policy-v1
rules: {}
providers:
  allowed: [openai]
`)
	if err := os.WriteFile(path, v1, 0o600); err != nil {
		t.Fatal(err)
	}

	p0, err := LoadFromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	store := NewPolicyStore(p0)
	if store.GetPolicy().Name != "policy-v1" {
		t.Fatalf("initial name: %q", store.GetPolicy().Name)
	}

	stop, err := WatchPolicy(path, store, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	v2 := []byte(`
version: "1.0"
name: policy-v2
rules: {}
providers:
  allowed: [openai, anthropic]
`)
	if err := os.WriteFile(path, v2, 0o600); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for store.GetPolicy().Name != "policy-v2" {
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for reload; still %q", store.GetPolicy().Name)
		}
		time.Sleep(50 * time.Millisecond)
	}

	if !store.GetPolicy().ProviderAllowed("anthropic") {
		t.Fatal("expected v2 to allow anthropic")
	}
}

func TestWatchPolicy_InvalidYAMLKeepsPreviousPolicy(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tamga-policy.yaml")

	good := []byte(`
version: "1.0"
name: good-policy
rules: {}
`)
	if err := os.WriteFile(path, good, 0o600); err != nil {
		t.Fatal(err)
	}
	p0, err := LoadFromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	store := NewPolicyStore(p0)

	stop, err := WatchPolicy(path, store, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	if err := os.WriteFile(path, []byte("version: [\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if store.GetPolicy().Name != "good-policy" {
			t.Fatalf("policy should not change on parse error, got %q", store.GetPolicy().Name)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func TestPolicyStore_Reload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "p.yaml")
	if err := os.WriteFile(path, []byte("version: \"1.0\"\nname: a\nrules: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := LoadFromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	store := NewPolicyStore(p)
	if err := os.WriteFile(path, []byte("version: \"1.0\"\nname: b\nrules: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.Reload(path); err != nil {
		t.Fatal(err)
	}
	if store.GetPolicy().Name != "b" {
		t.Fatalf("got %q", store.GetPolicy().Name)
	}
}

func TestWatcher_NonYamlFiles(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "tamga-policy.yaml")

	v1 := []byte(`
version: "1.0"
name: initial
rules: {}
`)
	if err := os.WriteFile(policyPath, v1, 0o600); err != nil {
		t.Fatal(err)
	}

	p0, err := LoadFromFile(policyPath)
	if err != nil {
		t.Fatal(err)
	}
	store := NewPolicyStore(p0)

	stop, err := WatchPolicy(policyPath, store, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	// Write a non-.yaml file in the same directory. Its base name differs
	// from the watched policy file, so eventTargetsFile should filter it out.
	nonYamlPath := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(nonYamlPath, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Also try writing a file with the .yaml extension but a different base name.
	otherYaml := filepath.Join(dir, "other.yaml")
	if err := os.WriteFile(otherYaml, []byte("version: \"1.0\"\nname: other\nrules: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Wait a bit and verify the policy name is still "initial"
	time.Sleep(1 * time.Second)
	if store.GetPolicy().Name != "initial" {
		t.Fatalf("non-target files should not trigger reload, got name %q", store.GetPolicy().Name)
	}
}

func TestWatcher_DeletedFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tamga-policy.yaml")

	v1 := []byte(`
version: "1.0"
name: before-delete
rules: {}
`)
	if err := os.WriteFile(path, v1, 0o600); err != nil {
		t.Fatal(err)
	}

	p0, err := LoadFromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	store := NewPolicyStore(p0)

	stop, err := WatchPolicy(path, store, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	// Delete the watched file. The watcher should receive a Remove event,
	// try to reload, fail (file gone), and keep the previous policy.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		// Policy should remain unchanged since reload fails on missing file.
		if store.GetPolicy().Name != "before-delete" {
			t.Fatalf("policy should not change when file is deleted, got %q", store.GetPolicy().Name)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func TestWatcher_DirectoryRenames(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "configs")
	if err := os.MkdirAll(subDir, 0o750); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(subDir, "tamga-policy.yaml")

	v1 := []byte(`
version: "1.0"
name: dir-rename-test
rules: {}
`)
	if err := os.WriteFile(path, v1, 0o600); err != nil {
		t.Fatal(err)
	}

	p0, err := LoadFromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	store := NewPolicyStore(p0)

	stop, err := WatchPolicy(path, store, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	// Rename the parent directory. This should cause fsnotify events on the
	// original directory path. The watcher should handle this gracefully.
	newSubDir := filepath.Join(dir, "configs-renamed")
	if err := os.Rename(subDir, newSubDir); err != nil {
		t.Fatal(err)
	}

	// Give watcher time to process events, then verify no panic/crash.
	time.Sleep(1 * time.Second)

	// Policy should still be the original (or unchanged) since the path moved.
	// NOTE: current behavior — watcher may see a Rename event on the dir but
	// since the file no longer exists at the watched path, reload will fail
	// and the previous policy is kept.
	if store.GetPolicy() == nil {
		t.Fatal("policy store should not be nil after directory rename")
	}
}

func TestPolicyStore_Reload_NilStore(t *testing.T) {
	var store *PolicyStore
	err := store.Reload("/nonexistent/path.yaml")
	if err == nil {
		t.Fatal("expected error for nil store")
	}
}

func TestWatchPolicy_NilStore(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tamga-policy.yaml")
	if err := os.WriteFile(path, []byte("version: \"1.0\"\nname: x\nrules: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stop, err := WatchPolicy(path, nil, nil)
	if err == nil {
		if stop != nil {
			stop()
		}
		t.Fatal("expected error for nil policy store")
	}
}

func TestWatchPolicy_OnReloadCallback(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tamga-policy.yaml")

	v1 := []byte(`
version: "1.0"
name: callback-v1
rules: {}
`)
	if err := os.WriteFile(path, v1, 0o600); err != nil {
		t.Fatal(err)
	}

	p0, err := LoadFromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	store := NewPolicyStore(p0)

	reloadCount := 0
	stop, err := WatchPolicy(path, store, func() {
		reloadCount++
	})
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	v2 := []byte(`
version: "1.0"
name: callback-v2
rules: {}
`)
	if err := os.WriteFile(path, v2, 0o600); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for reloadCount < 1 {
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for onReload callback; count=%d", reloadCount)
		}
		time.Sleep(50 * time.Millisecond)
	}

	if store.GetPolicy().Name != "callback-v2" {
		t.Fatalf("expected callback-v2, got %q", store.GetPolicy().Name)
	}
}

func TestPolicyStore_GetPolicy_NilStore(t *testing.T) {
	var s *PolicyStore
	if p := s.GetPolicy(); p != nil {
		t.Fatalf("nil store should return nil policy, got %+v", p)
	}
}

func TestPolicyStore_NewPolicyStore_Nil(t *testing.T) {
	s := NewPolicyStore(nil)
	if s == nil {
		t.Fatal("NewPolicyStore should not return nil")
	}
	if p := s.GetPolicy(); p != nil {
		t.Fatalf("nil initial policy should result in nil stored policy, got %+v", p)
	}
}
