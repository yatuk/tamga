package scanner

// ScannerVersion is the semantic version of the scanner package.
// Increment on any Finding struct change, validator enhancement, or new scanner.
//
// It is part of the response cache's key (proxy.cacheScanState): an answer
// cached as clean by one version is not served by another. 1.3.0 is the
// second change of what the scanners find inside the unreleased v0.9.0;
// builds of 1.2.0 are running, and an answer they cached was not read with
// the letter-by-letter view.
const ScannerVersion = "1.3.0"

// ScannerVersions maps each scanner name to its current semantic version.
var ScannerVersions = map[string]string{
	"pii":                "1.2.0", // no e-mail finding in the userinfo of a URL
	"injection":          "1.1.0", // reads phrases spelled one letter at a time
	"secret":             "1.1.0", // vendor formats of 2026-10-11
	"content_moderation": "1.0.0",
	"jailbreak":          "1.2.0", // translation_bypass by shape; spelled-out phrases
	"custom":             "1.0.0",
}

// BINDatasetVersion identifies the embedded binlist.csv snapshot.
// Format: YYYY-QN (year + quarter). Update on each BIN dataset refresh.
const BINDatasetVersion = "2026-Q2"
