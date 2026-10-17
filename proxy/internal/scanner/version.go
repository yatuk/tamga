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
//
// 1.7.0: four Turkish identifiers are read where the text names them
// (mersis_no, imei, passport_tr, license_plate_tr), and an IMEI or a MERSİS
// number named as one is no longer a credit_card finding. An answer a build
// of 1.6.0 stored as clean can hold one of the four, which a policy that
// lists it in output_rules masks or blocks.
//
// 1.8.0: Turkish instructions are read by stem and ending
// (injection_turkish.go), instructions handed over in quoted parts are
// read, and a request to send all of some personal data to an address is a
// finding (data_exfiltration). An answer a build of 1.7.0 stored as clean
// can hold a Turkish instruction this build blocks under output_rules.
//
// 1.9.0: a link whose host reads as a protected name and is not it
// (gıthub.com with a dotless ı) is a link_homoglyph finding. An answer a
// build of 1.8.0 stored as clean can hold such a link, which a policy that
// lists malicious_link in output_rules masks or blocks.
//
// 1.11.0: an injection a text only quotes while it discusses it is the
// signal quoted_injection and not a verdict (injection_framing.go), and the
// word "jailbreak" alone is severity low. Both make fewer verdicts. The
// second reading, with the quoted parts blanked out, is a text the scanners
// had not read before, so the version moves with it.
const ScannerVersion = "1.11.0"

// ScannerVersions maps each scanner name to its current semantic version.
var ScannerVersions = map[string]string{
	"pii":                "1.5.0", // mersis_no, imei, passport_tr, license_plate_tr (pii_tr_ids.go)
	"injection":          "1.4.0", // what a text only quotes while it discusses it is quoted_injection
	"secret":             "1.2.0", // read in the folded reading with the case kept, with positions
	"content_moderation": "1.0.0",
	"jailbreak":          "1.6.0", // what a text only quotes while it discusses it is quoted_injection
	"custom":             "1.1.0", // patterns are also run on the normalised readings
	"malicious_link":     "1.1.0", // look-alikes of protected names written in Latin letters only
	"output_schema":      "1.0.0", // answers only: output_rules.json_schema
}

// BINDatasetVersion identifies the embedded binlist.csv snapshot.
// Format: YYYY-QN (year + quarter). Update on each BIN dataset refresh.
const BINDatasetVersion = "2026-Q2"
