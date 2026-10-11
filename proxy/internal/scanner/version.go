package scanner

// ScannerVersion is the semantic version of the scanner package.
// Increment on any Finding struct change, validator enhancement, or new scanner.
//
// It is part of the response cache's key (proxy.cacheScanState): an answer
// cached as clean by one version is not served by another. 1.3.0 was the
// second change of what the scanners find inside the unreleased v0.9.0, and
// 1.4.0 is the third; builds of 1.3.0 are running. A request they forwarded
// and cached can be one this build blocks: a tool result that tells the
// model to drop a translation, or two different cards with the same first
// six and last four digits of which one is written so that only a
// normalised view shows it (it used to be dropped as an echo of the other,
// and the request masked and sent with it). That a card's match now shows
// its last four digits only would not by itself have needed a new version:
// the match changes no verdict.
//
// 1.5.0: answers are read by a link scanner (malicious_link). The cache
// holds only answers nothing was found in, and serves them unscanned; an
// answer a build of 1.4.0 stored as clean can hold a link this build masks
// or blocks.
//
// 1.6.0: a value in a normalised reading is found where it stands, so the
// same value written twice is two findings; secrets and the policy's own
// patterns are read in those readings; leetspeak instructions are read. An
// answer a build of 1.5.0 stored as clean can hold a key in fullwidth
// letters or an instruction in leetspeak.
const ScannerVersion = "1.6.0"

// ScannerVersions maps each scanner name to its current semantic version.
var ScannerVersions = map[string]string{
	"pii":                "1.4.0", // a value in a normalised reading has the position of what it was read from
	"injection":          "1.2.0", // reads leetspeak for the words the rules know
	"secret":             "1.2.0", // read in the folded reading with the case kept, with positions
	"content_moderation": "1.0.0",
	"jailbreak":          "1.4.0", // leetspeak; a phrase in plain sight is not an encoded override
	"custom":             "1.1.0", // patterns are also run on the normalised readings
	"malicious_link":     "1.0.0", // answers only: links judged by form and by the policy's host lists
	"output_schema":      "1.0.0", // answers only: output_rules.json_schema
}

// BINDatasetVersion identifies the embedded binlist.csv snapshot.
// Format: YYYY-QN (year + quarter). Update on each BIN dataset refresh.
const BINDatasetVersion = "2026-Q2"
