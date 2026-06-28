package scanner

// ScannerVersion is the semantic version of the scanner package.
// Increment on any Finding struct change, validator enhancement, or new scanner.
const ScannerVersion = "1.1.0"

// ScannerVersions maps each scanner name to its current semantic version.
var ScannerVersions = map[string]string{
	"pii":                "1.1.0", // TCKN denylist + PCI-DSS maskPAN added
	"injection":          "1.0.0",
	"secret":             "1.0.0",
	"content_moderation": "1.0.0",
	"jailbreak":          "1.0.0",
	"custom":               "1.0.0",
	"operator_state":       "1.0.0",
}

// BINDatasetVersion identifies the embedded binlist.csv snapshot.
// Format: YYYY-QN (year + quarter). Update on each BIN dataset refresh.
const BINDatasetVersion = "2026-Q2"
