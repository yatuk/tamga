package api

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"syscall"
)

// errPolicyReadOnly means the policy file cannot be written where it is: a
// read-only mount, or a file the proxy's user may not change. It is a
// deployment choice, not a fault, and the API says so instead of answering 500.
var errPolicyReadOnly = errors.New("the policy file is read-only")

const policyReadOnlyMessage = "The policy file is read-only where the proxy runs, so it cannot be changed from the console. " +
	"Edit the file there and reload it, or mount it writable."

// renameFile is os.Rename; tests replace it to stand in for a bind mount.
var renameFile = os.Rename

func isReadOnlyErr(err error) bool {
	return errors.Is(err, os.ErrPermission) || errors.Is(err, syscall.EROFS)
}

// policyWritable reports whether the policy file can be opened for writing.
// It changes nothing: the file is opened without truncation and closed.
func policyWritable(path string) bool {
	if path == "" {
		return false
	}
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}

// writePolicyFile replaces the policy file with data.
//
// The safe way is a temp file and a rename, so a crash never leaves half a
// policy. A file that is itself a mount point (a single-file bind mount, the
// usual container setup) cannot be renamed over: the kernel answers EBUSY.
// There the file is rewritten in place, and put back as it was if that write
// fails part way.
func writePolicyFile(path string, data []byte) error {
	renameErr := writeByRename(path, data)
	if renameErr == nil {
		return nil
	}

	old, readErr := os.ReadFile(path)
	if readErr != nil {
		// Nothing to fall back on: the file is not there to rewrite.
		return renameErr
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0)
	if err != nil {
		if isReadOnlyErr(err) {
			return fmt.Errorf("%w: %v", errPolicyReadOnly, err)
		}
		return err
	}
	_, werr := f.Write(data)
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		_ = os.WriteFile(path, old, 0)
		return werr
	}
	return nil
}

func writeByRename(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tamga-policy-*.yaml")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return err
	}
	if err := renameFile(name, path); err != nil {
		_ = os.Remove(name)
		return err
	}
	return nil
}

// writePolicyWriteError answers a failed policy write: 409 with a reason a
// person can act on when the file is read-only, 500 otherwise.
func writePolicyWriteError(w http.ResponseWriter, err error) {
	if errors.Is(err, errPolicyReadOnly) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": policyReadOnlyMessage, "code": "policy_read_only"})
		return
	}
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
}
