package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// A single-file bind mount cannot be renamed over. busyRename stands in for it.
func busyRename(t *testing.T) {
	t.Helper()
	old := renameFile
	renameFile = func(_, newpath string) error {
		return &os.LinkError{Op: "rename", New: newpath, Err: syscall.EBUSY}
	}
	t.Cleanup(func() { renameFile = old })
}

func TestWritePolicyFile_RewritesInPlaceWhenRenameIsRefused(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tamga-policy.yaml")
	if err := os.WriteFile(path, []byte("name: old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	busyRename(t)

	if err := writePolicyFile(path, []byte("name: new\n")); err != nil {
		t.Fatalf("writePolicyFile: %v", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "name: new\n" {
		t.Fatalf("file holds %q", got)
	}
	// The failed rename must not leave its temp file behind.
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("directory holds %d entries, want only the policy file", len(entries))
	}
}

func TestWritePolicyFile_ReadOnlyFileIsReportedAsReadOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tamga-policy.yaml")
	if err := os.WriteFile(path, []byte("name: old\n"), 0o400); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })
	if f, err := os.OpenFile(path, os.O_WRONLY, 0); err == nil {
		_ = f.Close()
		t.Skip("this user can write a read-only file (running as root)")
	}
	busyRename(t)

	err := writePolicyFile(path, []byte("name: new\n"))
	if !errors.Is(err, errPolicyReadOnly) {
		t.Fatalf("got %v, want errPolicyReadOnly", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "name: old\n" {
		t.Fatalf("a refused write changed the file to %q", got)
	}
	if policyWritable(path) {
		t.Fatal("policyWritable says a read-only file is writable")
	}
}

func TestPolicyWritable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tamga-policy.yaml")
	if policyWritable(path) {
		t.Fatal("a missing file is not writable")
	}
	if err := os.WriteFile(path, []byte("name: p\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !policyWritable(path) {
		t.Fatal("a plain file should be writable")
	}
	// Asking must not change the file.
	got, _ := os.ReadFile(path)
	if string(got) != "name: p\n" {
		t.Fatalf("policyWritable changed the file to %q", got)
	}
	if policyWritable("") {
		t.Fatal("no path is not writable")
	}
}

func TestWritePolicyWriteError(t *testing.T) {
	rec := httptest.NewRecorder()
	writePolicyWriteError(rec, errors.Join(errPolicyReadOnly, syscall.EROFS))
	if rec.Code != http.StatusConflict {
		t.Fatalf("read-only: status %d, want 409", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, `"code":"policy_read_only"`) || strings.Contains(body, "read-only file system") {
		t.Fatalf("read-only: body %s", body)
	}

	rec = httptest.NewRecorder()
	writePolicyWriteError(rec, errors.New("disk full"))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("other error: status %d, want 500", rec.Code)
	}
}
