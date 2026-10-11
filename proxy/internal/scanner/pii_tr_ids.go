package scanner

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Turkish identifiers beyond the ones pii.go has always known (tc_kimlik,
// vkn, iban, phone_tr). Which ones, and why these and not others, is in
// docs/research/turkish-dlp-landscape.md.
//
// None of these formats is rare enough to be taken on its shape alone: a
// sixteen digit run is an order number, "34 ABC 123" is a part number, a
// letter and eight digits is a ticket. Each pattern is therefore read only
// in a text that names the thing ("MERSİS", "IMEI", "pasaport", "plaka"),
// and a match counts only when that word stands near it. The word is looked
// for before any regular expression runs, so a text without it pays a few
// byte comparisons and nothing more.
//
// Categories:
//
//   - mersis_no: the sixteen digit trade registry number. It is built from a
//     tax number (a legal entity: "0", the ten digit VKN, five more digits)
//     or from a national id (a sole proprietor: the eleven digit TCKN, five
//     more digits), so it carries that number's check digits. The second
//     form holds a person's TCKN in full and is as severe as one.
//   - imei: fifteen digits, the last a Luhn check digit (3GPP TS 23.003).
//   - passport_tr: a letter and eight digits that follow the word passport
//     on the same line. No check digit, and no official source for the
//     series was found, so the word has to come first and close by.
//   - license_plate_tr: a province code 01-81, one to three letters and two
//     to five digits, in the six combinations the registration uses.
const (
	categoryMersis      = "mersis_no"
	categoryIMEI        = "imei"
	categoryPassportTR  = "passport_tr"
	categoryPlateTR     = "license_plate_tr"
	trWordWindowBefore  = 60
	trWordWindowAfter   = 40
	trWordBeforeOnLine  = 40
	mersisNone          = 0
	mersisLegalEntity   = 1 // "0" + VKN + five digits
	mersisNaturalPerson = 2 // TCKN + five digits
)

// The words that name each identifier, lower case, as strings.ToLower gives
// them ("MERSİS" and "MERSIS" both become "mersis"). They are matched at the
// start of a word, so "plate" does not count inside "template", and any
// ending may follow ("plakalı", "pasaportum", "IMEI'si").
var (
	mersisWords   = []string{"mersis"}
	imeiWords     = []string{"imei"}
	passportWords = []string{"pasaport", "passport"}
	plateWords    = []string{"plaka", "plate"}
)

// The same words as plain ASCII stems for the cheap first look at a text.
// "mers" is as much of "MERSİS" as is ASCII in both spellings.
var (
	mersisGate   = []string{"mers"}
	imeiGate     = []string{"imei"}
	passportGate = []string{"pasaport", "passport"}
	plateGate    = []string{"plaka", "plate"}
)

// trIDPatterns returns the patterns of this file. compilePIIPatterns adds
// them after its own.
func trIDPatterns() []piiPattern {
	sixteen := regexp.MustCompile(`\b\d(?:[ -]?\d){15}\b`)
	checked := ConfidenceFactor{Format: WFormat, Algorithm: WAlgorithm, Context: WContext}
	named := ConfidenceFactor{Format: WFormat, Context: WContext}

	mersis := func(kind int, severity string) piiPattern {
		return piiPattern{
			Name:     categoryMersis,
			Regex:    sixteen,
			Severity: severity,
			validate: func(m string) bool { return mersisKind(digitsOnly(m)) == kind },
			skip: func(text string, loc []int) bool {
				return digitRunContinues(text, loc) || !wordNear(text, loc[0], loc[1], mersisWords)
			},
			gate:        mersisGate,
			fingerprint: idFingerprint,
			factor:      checked,
			forwardOnly: true,
			confidence:  0.90,
		}
	}

	return []piiPattern{
		mersis(mersisLegalEntity, "high"),
		mersis(mersisNaturalPerson, "critical"),
		{
			Name: categoryIMEI,
			// As printed (AA-BBBBBB-CCCCCC-D) or as one run.
			Regex:    regexp.MustCompile(`\b\d{2}[ -]?\d{6}[ -]?\d{6}[ -]?\d\b`),
			Severity: "high",
			validate: func(m string) bool { return validIMEI(digitsOnly(m)) },
			skip: func(text string, loc []int) bool {
				return digitRunContinues(text, loc) || !wordNear(text, loc[0], loc[1], imeiWords)
			},
			gate:        imeiGate,
			fingerprint: idFingerprint,
			factor:      checked,
			forwardOnly: true,
			confidence:  0.90,
		},
		{
			Name:     categoryPassportTR,
			Regex:    regexp.MustCompile(`\b[A-Z]\d{8}\b`),
			Severity: "high",
			validate: func(m string) bool { return !allSameDigit(m[1:]) },
			skip: func(text string, loc []int) bool {
				return !wordBeforeOnLine(text, loc[0], passportWords)
			},
			gate:        passportGate,
			fingerprint: idFingerprint,
			factor:      named,
			forwardOnly: true,
			confidence:  0.80,
		},
		{
			Name: categoryPlateTR,
			// Upper case only: in lower case "34 ve 123" has the same shape.
			// Q, W and X are not used on plates.
			Regex:    regexp.MustCompile(`\b(?:0[1-9]|[1-7][0-9]|8[01])[ -]?[A-PR-VYZ]{1,3}[ -]?\d{2,5}\b`),
			Severity: "medium",
			validate: validPlateTR,
			skip: func(text string, loc []int) bool {
				return !wordNear(text, loc[0], loc[1], plateWords)
			},
			gate:        plateGate,
			fingerprint: idFingerprint,
			factor:      named,
			forwardOnly: true,
			confidence:  0.75,
		},
	}
}

// mersisKind says which of the two forms a sixteen digit string is, or
// mersisNone when it is neither: the number it would be built from fails its
// own check digits.
func mersisKind(d string) int {
	if len(d) != 16 || allSameDigit(d) {
		return mersisNone
	}
	if d[0] == '0' {
		if validVKN(d[1:11]) {
			return mersisLegalEntity
		}
		return mersisNone
	}
	if validTCKN(d[:11]) {
		return mersisNaturalPerson
	}
	return mersisNone
}

// validIMEI checks the fifteen digit form: fourteen digits and their Luhn
// check digit. The sixteen digit IMEISV has no check digit and is not read.
func validIMEI(d string) bool {
	return len(d) == 15 && !allSameDigit(d) && validLuhn(d)
}

// validPlateTR holds a plate to the combinations in use: the fewer letters,
// the more digits (99 X 9999, 99 X 99999, 99 XX 999, 99 XX 9999, 99 XXX 99,
// 99 XXX 999).
func validPlateTR(m string) bool {
	letters, digits := 0, 0
	for i := 2; i < len(m); i++ {
		switch c := m[i]; {
		case c >= 'A' && c <= 'Z':
			letters++
		case c >= '0' && c <= '9':
			digits++
		}
	}
	switch letters {
	case 1:
		return digits == 4 || digits == 5
	case 2:
		return digits == 3 || digits == 4
	case 3:
		return digits == 2 || digits == 3
	}
	return false
}

// labelledNotACard reports whether the card-shaped match at loc is one of
// the identifiers above and says so itself. A fifteen digit IMEI passes the
// Luhn check by construction, and a sixteen digit MERSİS number passes it
// one time in ten, so both used to be reported as card numbers. Only a
// number that no card network issues in that shape is given up: fifteen
// digits that do not start like the fifteen digit card ranges, next to the
// word IMEI, or sixteen digits built from a valid tax number or national id,
// next to the word MERSİS.
func labelledNotACard(text string, loc []int) bool {
	d := digitsOnly(text[loc[0]:loc[1]])
	switch len(d) {
	case 15:
		return !fifteenDigitCardPrefix(d) && validIMEI(d) && wordNear(text, loc[0], loc[1], imeiWords)
	case 16:
		return mersisKind(d) != mersisNone && wordNear(text, loc[0], loc[1], mersisWords)
	}
	return false
}

// fifteenDigitCardPrefix reports whether d starts like a card number that
// has fifteen digits: American Express (34, 37), UATP (1), and the old
// fifteen digit ranges of JCB (2131, 1800) and enRoute (2014, 2149).
func fifteenDigitCardPrefix(d string) bool {
	return d[0] == '1' || strings.HasPrefix(d, "34") || strings.HasPrefix(d, "37") ||
		strings.HasPrefix(d, "2131") || strings.HasPrefix(d, "2014") || strings.HasPrefix(d, "2149")
}

// idFingerprint tells two identifiers apart whose masked matches are the
// same (the mask keeps the first two and the last two characters), as
// cardFingerprint does for cards. Separators are ignored.
func idFingerprint(m string) uint64 {
	var b strings.Builder
	for i := 0; i < len(m); i++ {
		if isASCIIAlnum(m[i]) {
			b.WriteByte(m[i])
		}
	}
	return secretFingerprint(b.String())
}

// digitRunContinues reports whether the digits matched at loc are a part of
// a longer run: "1234 5678 9012 3456 7890" holds no sixteen digit number.
func digitRunContinues(text string, loc []int) bool {
	isDigit := func(i int) bool { return i >= 0 && i < len(text) && text[i] >= '0' && text[i] <= '9' }
	isSep := func(i int) bool { return i >= 0 && i < len(text) && (text[i] == ' ' || text[i] == '-') }
	if isDigit(loc[1]) || (isSep(loc[1]) && isDigit(loc[1]+1)) {
		return true
	}
	return isDigit(loc[0]-1) || (isSep(loc[0]-1) && isDigit(loc[0]-2))
}

// wordNear reports whether one of words starts a word close to text[start:end]:
// within trWordWindowBefore bytes before it or trWordWindowAfter after it.
// The words are lower case; the text is folded with strings.ToLower.
func wordNear(text string, start, end int, words []string) bool {
	from := start - trWordWindowBefore
	if from < 0 {
		from = 0
	}
	to := end + trWordWindowAfter
	if to > len(text) {
		to = len(text)
	}
	return wordIn(text, from, to, words)
}

// wordBeforeOnLine is the stricter form for a format with no check digit:
// the word has to come first, at most trWordBeforeOnLine bytes before the
// match and on its line ("Pasaport No: U12345678"). A build number further
// down a file that has a pasaport_no column is not a passport number.
func wordBeforeOnLine(text string, start int, words []string) bool {
	from := start - trWordBeforeOnLine
	if from < 0 {
		from = 0
	}
	if nl := strings.LastIndexByte(text[from:start], '\n'); nl >= 0 {
		from += nl + 1
	}
	return wordIn(text, from, start, words)
}

// wordIn reports whether one of words starts a word in text[from:to].
func wordIn(text string, from, to int, words []string) bool {
	window := strings.ToLower(text[from:to])
	for _, w := range words {
		for i := 0; i < len(window); {
			j := strings.Index(window[i:], w)
			if j < 0 {
				break
			}
			j += i
			before := window[:j]
			if j == 0 {
				// The window may have been cut inside a word.
				before = text[:from]
			}
			if r, size := utf8.DecodeLastRuneInString(before); size == 0 || !(unicode.IsLetter(r) || unicode.IsDigit(r)) {
				return true
			}
			i = j + 1
		}
	}
	return false
}

// anyStemFold reports whether text holds one of the ASCII lower case stems,
// in any case. It is the first look that decides whether a pattern runs.
func anyStemFold(text string, stems []string) bool {
	for _, stem := range stems {
		n := len(stem)
		for i := 0; i+n <= len(text); i++ {
			if text[i]|0x20 != stem[0] {
				continue
			}
			j := 1
			for j < n && text[i+j]|0x20 == stem[j] {
				j++
			}
			if j == n {
				return true
			}
		}
	}
	return false
}

// PIICategories returns every category a finding of the PII scanner can
// have. A policy template is tested against it: a category in a rule that
// no pattern reports would read as protection and be none.
func PIICategories() []string {
	piiOnce.Do(compilePIIPatterns)
	seen := map[string]bool{}
	var out []string
	for _, name := range append([]string{"ip_public", "ip_private"}, patternNames()...) {
		if !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	return out
}

func patternNames() []string {
	out := make([]string, 0, len(piiPatterns))
	for _, p := range piiPatterns {
		out = append(out, p.Name)
	}
	return out
}
