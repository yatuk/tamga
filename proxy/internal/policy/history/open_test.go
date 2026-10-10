package history

import (
	"os"
	"path/filepath"
	"testing"
)

// When the Postgres history store cannot be created the proxy said it was
// "falling back to file", created no file store, and then logged "policy
// history backed by postgres". The fallback is real now.
func TestOpen_FallsBackToFiles(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "history")

	// A database is configured and its pool is not usable.
	st, backend := Open(true, nil, dir)
	if backend != BackendFile {
		t.Fatalf("backend = %q, want %q", backend, BackendFile)
	}
	if _, ok := st.(*FileStore); !ok {
		t.Fatalf("store is %T, want the file store", st)
	}
	rev, err := st.AppendRevision(Revision{Author: "test", YAML: "name: p\n"})
	if err != nil {
		t.Fatalf("the fallback store does not take a revision: %v", err)
	}
	revs, err := st.ListRevisions()
	if err != nil || len(revs) != 1 || revs[0].ID != rev.ID {
		t.Fatalf("the revision was not kept: %v %v", revs, err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("the history directory was not created: %v", err)
	}
}

func TestOpen_WithoutADatabase(t *testing.T) {
	st, backend := Open(false, nil, t.TempDir())
	if backend != BackendFile || st == nil {
		t.Fatalf("backend = %q, store = %v", backend, st)
	}
}

// A directory that cannot be used leaves no history, and a nil store: a nil
// interface, which the API checks for, not a store that holds nothing.
func TestOpen_Disabled(t *testing.T) {
	// A file where the directory should be.
	blocked := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocked, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, hasDB := range []bool{true, false} {
		st, backend := Open(hasDB, nil, filepath.Join(blocked, "history"))
		if backend != BackendDisabled {
			t.Fatalf("hasDB=%v: backend = %q, want %q", hasDB, backend, BackendDisabled)
		}
		if st != nil {
			t.Fatalf("hasDB=%v: store = %T, want nil", hasDB, st)
		}
	}
}
