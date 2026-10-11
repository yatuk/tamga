package scanner_test

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"unicode/utf8"
)

// Every text and data file of the scanners is UTF-8, and none holds U+FFFD,
// the character a decoder leaves where it could not read one. What a scanner
// says about a finding (a deny-list reason, a bank's name) is compiled or
// embedded from these files and shown as it is, so a file saved in another
// encoding, or one that was decoded wrongly once and saved again, reaches
// the console as a broken name.
//
// A test that needs the character itself writes it as "\uFFFD".
func TestScannerFilesAreUTF8(t *testing.T) {
	replacement := []byte("\uFFFD")
	checked := 0
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		// A file with a NUL byte is not text (a model, an archive).
		if bytes.IndexByte(data, 0) >= 0 {
			return nil
		}
		checked++
		if !utf8.Valid(data) {
			line := 1 + bytes.Count(data[:invalidAt(data)], []byte("\n"))
			t.Errorf("%s is not valid UTF-8 (line %d): save it as UTF-8", path, line)
			return nil
		}
		if at := bytes.Index(data, replacement); at >= 0 {
			line := 1 + bytes.Count(data[:at], []byte("\n"))
			t.Errorf("%s holds U+FFFD at line %d: a character was lost when the file was decoded; restore it", path, line)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// The package's own sources at the least: a walk that read nothing
	// checked nothing.
	if checked < 50 {
		t.Fatalf("only %d files were read from the scanner directory", checked)
	}
}

// invalidAt is the offset of the first byte that is not valid UTF-8.
func invalidAt(data []byte) int {
	for i := 0; i < len(data); {
		r, size := utf8.DecodeRune(data[i:])
		if r == utf8.RuneError && size == 1 {
			return i
		}
		i += size
	}
	return len(data)
}
