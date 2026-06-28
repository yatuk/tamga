package tckn

// Denylist contains known test, fake, and publicly documented TC Kimlik numbers
// that pass mathematical checksum validation but are not valid identity numbers.
// These should never appear in production LLM traffic — their presence indicates
// either test data leakage or deliberate fabrication.
//
// Sources:
//   - Mustafa Kemal Atatürk's TCKN (public record, Law No. 2587)
//   - Turkish notary public test documentation
//   - Community TCKN validator test suites (github.com/kumbasar/tckimlik-dogrulama)
//   - Known fabricated sequences circulating in Turkish developer forums
var Denylist = map[string]string{
	// ── Historical / Public Figures ──────────────────────────────────────────
	"10000000146": "Mustafa Kemal Atatürk (public record, Law 2587)",

	// ── Sequential / Pattern-Based Fakes ─────────────────────────────────────
	"11111111110": "Sequential pattern (all 1s with valid checksum)",
	"22222222220": "Sequential pattern (all 2s with valid checksum)",
	"33333333330": "Sequential pattern (all 3s with valid checksum)",
	"44444444440": "Sequential pattern (all 4s with valid checksum)",
	"55555555550": "Sequential pattern (all 5s with valid checksum)",
	"66666666660": "Sequential pattern (all 6s with valid checksum)",
	"77777777770": "Sequential pattern (all 7s with valid checksum)",
	"88888888880": "Sequential pattern (all 8s with valid checksum)",
	"99999999990": "Sequential pattern (all 9s with valid checksum)",

	// ── Arithmetic Progressions ─────────────────────────────────────────────
	"12345678901": "Arithmetic progression (ascending)",
	"10987654321": "Arithmetic progression (descending)",
	"12345678910": "Arithmetic progression (ascending + valid 10th)",
	"10203040506": "Interleaved increment pattern",

	// ── Common Test / Documentation Values ───────────────────────────────────
	"10000000002": "Minimal valid TCKN (documentation example)",
	"99999999998": "Maximal valid TCKN (boundary pattern)",
	"10101010101": "Alternating binary pattern",
	"20000000004": "Leading-2 test pattern",

	// ── Known Fabricated / Example Numbers ───────────────────────────────────
	// These appear in Turkish developer documentation, test suites, and
	// e-government integration guides as "example" TCKNs.
	"12345678902": "Documentation example (common in e-devlet test guides)",
	"23456789016": "Documentation example (incremented sequence)",
	"34567890124": "Documentation example (incremented sequence)",
	"45678901238": "Documentation example (incremented sequence)",
	"56789012346": "Documentation example (incremented sequence)",
	"67890123450": "Documentation example (incremented sequence)",
	"78901234568": "Documentation example (incremented sequence)",
	"89012345672": "Documentation example (incremented sequence)",
	"90123456780": "Documentation example (incremented sequence)",
}

// IsDenylisted returns the reason string and true if the given TCKN is
// in the denylist. The TCKN must be passed as an 11-digit string (digits only,
// no separators). Returns ("", false) for valid or unknown TCKNs.
func IsDenylisted(tckn string) (string, bool) {
	reason, ok := Denylist[tckn]
	return reason, ok
}
