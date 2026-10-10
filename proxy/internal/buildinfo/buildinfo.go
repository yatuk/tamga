// Package buildinfo says which build of Tamga this binary is. It is the one
// place the version comes from: the startup banner, GET /health/detail and
// the OpenTelemetry resource all ask here, and nothing else in the module
// writes a version down.
//
// (scanner.ScannerVersion is something else: it names the detection logic for
// cache keys and changes only when a scan can give a different answer.)
package buildinfo

import (
	"runtime/debug"
	"strings"
)

// Version and Revision are set when the binary is linked:
//
//	go build -ldflags "-X github.com/yatuk/tamga/internal/buildinfo.Version=v0.9.0 \
//	                   -X github.com/yatuk/tamga/internal/buildinfo.Revision=$(git rev-parse HEAD)"
//
// The Dockerfile passes them from its VERSION and REVISION build arguments
// and the Makefile from the variables of the same names. A release build
// sets Version to the tag. Both are empty in a build that sets neither.
var (
	Version  = ""
	Revision = ""
)

// devMarker is the version of a build nobody named. Such a binary does not
// claim a release it is not.
const devMarker = "dev"

// revisionLength is how much of a commit hash is shown.
const revisionLength = 12

// String returns the version to report: Version when the build set one,
// otherwise "dev" followed by the commit the binary was built from when that
// is known, as in "dev+3f9c2ab41d07", with ".dirty" appended when the working
// tree had changes that were not committed.
func String() string {
	return describe(strings.TrimSpace(Version), strings.TrimSpace(Revision), readVCS)
}

// describe is String with its inputs passed in.
func describe(version, revision string, vcs func() (revision string, modified bool)) string {
	if version != "" {
		return version
	}
	modified := false
	if revision == "" && vcs != nil {
		// What the Go tool recorded when it built inside a checkout. A build
		// from an exported tree, as in the Dockerfile, has none.
		revision, modified = vcs()
	}
	if revision == "" {
		return devMarker
	}
	if len(revision) > revisionLength {
		revision = revision[:revisionLength]
	}
	out := devMarker + "+" + revision
	if modified {
		out += ".dirty"
	}
	return out
}

// readVCS returns the commit and the state of the working tree the Go tool
// stamped into the binary, or "" when it stamped none.
func readVCS() (revision string, modified bool) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "", false
	}
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			revision = s.Value
		case "vcs.modified":
			modified = s.Value == "true"
		}
	}
	return revision, modified
}
