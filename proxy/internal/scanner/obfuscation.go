package scanner

import (
	"math"
	"strconv"
	"strings"

	"github.com/yatuk/tamga/internal/scanner/normalize"
)

// Two findings that say a text hides something, without saying it is an
// attack. Both are of type "injection", next to ascii_art and
// encoded_override, so the rules written for that type decide about them.
//
//   - hidden_text: characters that do not show and carry text of their own
//     (tag characters, runs of variation selectors), a long run of
//     zero-width characters, or a bidirectional override. When the hidden
//     characters spell text it is severity high: nobody types a sentence a
//     reader cannot see by accident, and the model reads it. What they
//     spell is also given to the other scanners (normalize.FindHidden).
//     Everything else is severity low.
//
//   - unreadable_block: a long run that looks encoded and that none of the
//     decoders turns into text. Severity low, always.
//
// A finding of these categories at severity low is a signal (Signal): it
// says the text is unusual, not that it is an attack. It raises the
// request's risk score, and it makes the inline classifier judge the text
// it is in by scan.classifier.signal_threshold. No rule acts on it unless
// the rule names the category, which is how an operator asks to be told:
//
//	injection_detection:
//	  action: WARN
//	  types: [unreadable_block, hidden_text]
//
// A rule without a types list does not, at any sensitivity, and neither
// does scan.default_action (policy.evalRuleAction).
const (
	categoryHiddenText      = "hidden_text"
	categoryUnreadableBlock = "unreadable_block"

	// hiddenMinChars is how much hidden text makes a finding. A hidden
	// instruction needs a few words; one stray character is noise.
	hiddenMinChars = 8

	// unreadableMinLen is the shortest run looked at. It is above the
	// length of the hashes, identifiers and keys that fill developer
	// traffic (a SHA-512 in base64 is 88 characters, a fine-grained GitHub
	// token 93), and long enough for the statistics below to mean
	// something.
	unreadableMinLen = 100
	// unreadableMinEntropy is in bits per character, over the characters of
	// the run. Of 20,000 random base64 strings of 100 characters the lowest
	// measured 5.09 (base58: 5.06); long identifiers, slugs and paths made
	// of words measured 4.2 to 4.6.
	unreadableMinEntropy = 4.85
	// unreadableMinFlips is the least share of neighbouring characters that
	// differ in class (upper case, lower case, digit, symbol). The same
	// random strings never went below 0.41; words joined by capitals,
	// dashes or underscores stay under 0.37.
	unreadableMinFlips = 0.40
	// base32MinEntropy is the entropy bound for a run in the base32
	// alphabet, which has 32 characters and one case. Random base32 of 100
	// characters did not go below 4.47; upper-case words and protein
	// sequences are near 4.
	base32MinEntropy = 4.3
	// unreadableLookBack bounds the search for the start of the word a run
	// is part of.
	unreadableLookBack = 512
)

// signalCategories are the injection categories that are a signal at
// severity low: the two above, and the two shapes of request that are an
// attack or an ordinary request depending on what the scanner cannot know
// (injection_context.go).
var signalCategories = map[string]bool{
	categoryHiddenText:           true,
	categoryUnreadableBlock:      true,
	categoryAboveTextRequest:     true,
	categoryTranslationAbandoned: true,
	categoryQuotedInjection:      true,
}

// Signal reports whether f marks its text as odd without being a verdict on
// it: one of the signal categories, at severity low.
func Signal(f Finding) bool {
	return f.Type == "injection" && f.Severity == "low" && signalCategories[f.Category]
}

// obfuscationFindings returns the hidden-text and unreadable-block findings
// of a text.
func obfuscationFindings(text string) []Finding {
	out := hiddenTextFindings(text)
	if f, ok := unreadableBlockFinding(text); ok {
		out = append(out, f)
	}
	return out
}

func hiddenFinding(kind, severity string, confidence float64, span normalize.HiddenSpan) Finding {
	return Finding{
		Type: "injection", Category: categoryHiddenText, Severity: severity,
		// The match names what was found and holds none of it.
		Match:      strconv.Itoa(span.Count) + " " + strings.ReplaceAll(kind, "_", " "),
		StartPos:   span.Start,
		EndPos:     span.End,
		Confidence: confidence,
		Metadata:   map[string]string{"kind": kind, "count": strconv.Itoa(span.Count)},
	}
}

func hiddenTextFindings(text string) []Finding {
	h := normalize.FindHidden(text)
	var out []Finding
	if len(h.TagText) >= hiddenMinChars {
		out = append(out, hiddenFinding("tag_characters", "high", 0.90, h.Tags))
	}
	switch {
	case len(h.SelectorText) >= hiddenMinChars:
		out = append(out, hiddenFinding("variation_selectors", "high", 0.90, h.Selectors))
	case h.Selectors.Count >= hiddenMinChars:
		out = append(out, hiddenFinding("variation_selectors", "low", 0.40, h.Selectors))
	}
	if h.FormatRun.Count >= hiddenMinChars {
		out = append(out, hiddenFinding("format_characters", "low", 0.40, h.FormatRun))
	}
	if h.Overrides.Count > 0 {
		out = append(out, hiddenFinding("bidi_overrides", "low", 0.40, h.Overrides))
	}
	return out
}

// isBlockChar reports whether c can be part of an encoded run: the base64
// alphabets, which contain those of base32, base58 and hex.
func isBlockChar(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') ||
		c == '+' || c == '/' || c == '=' || c == '_' || c == '-'
}

// binaryMagic are the first characters of base64 that encodes a file of a
// common binary format. Such a block is a picture or an archive pasted as
// text, not an encoding of words. The list is kept by hand: there is no
// table of these in the standard library.
var binaryMagic = []string{
	"iVBORw0KGgo", // PNG
	"/9j/",        // JPEG
	"R0lGOD",      // GIF
	"UklGR",       // RIFF: WebP, WAV, AVI
	"JVBERi0",     // PDF
	"UEsDB",       // ZIP: also docx, xlsx, jar
	"H4sI",        // gzip
	"QlpoO",       // bzip2
	"/Td6WFoA",    // xz
	"KLUv/Q",      // zstd
	"N3q8ryc",     // 7z
	"T2dnUw",      // Ogg
	"SUQz",        // MP3 with an ID3 tag
	"d09GR",       // WOFF
	"TVqQ",        // Windows executable
	"f0VMR",       // ELF
	"MII",         // ASN.1 DER: certificates and keys
	"AAAA",        // a length in four bytes: SSH public keys, MP4
}

// subresourcePrefixes start an integrity value ("sha512-…"), where the dash
// is part of the run.
var subresourcePrefixes = []string{"sha1-", "sha256-", "sha384-", "sha512-"}

// unreadableBlockFinding looks for runs that look encoded and decode to
// nothing readable. It reports the first, with the number found.
func unreadableBlockFinding(text string) (Finding, bool) {
	if len(text) < unreadableMinLen {
		return Finding{}, false
	}
	blocks, chars := 0, 0
	first := [2]int{}
	for i := 0; i < len(text); {
		if !isBlockChar(text[i]) {
			i++
			continue
		}
		j := i + 1
		for j < len(text) && isBlockChar(text[j]) {
			j++
		}
		if j-i >= unreadableMinLen && unreadableRun(text, i, j) {
			if blocks == 0 {
				first = [2]int{i, j}
			}
			blocks++
			chars += j - i
		}
		i = j
	}
	if blocks == 0 {
		return Finding{}, false
	}
	return Finding{
		Type: "injection", Category: categoryUnreadableBlock, Severity: "low",
		Match:      "unreadable block of " + strconv.Itoa(first[1]-first[0]) + " characters",
		StartPos:   first[0],
		EndPos:     first[1],
		Confidence: 0.40,
		Metadata:   map[string]string{"blocks": strconv.Itoa(blocks), "chars": strconv.Itoa(chars)},
	}, true
}

// unreadableRun decides about one run, text[start:end].
func unreadableRun(text string, start, end int) bool {
	tok := text[start:end]

	// Where it stands. A run inside a URL is a path, a signature or a
	// state parameter; one behind "data:image/…;base64," is the image; one
	// that continues a JWT is its signature, which the secret scanner
	// reports.
	wordStart := start
	for wordStart > 0 && start-wordStart < unreadableLookBack {
		c := text[wordStart-1]
		if c == ' ' || c == '\n' || c == '\t' || c == '\r' {
			break
		}
		wordStart--
	}
	before := text[wordStart:start]
	if strings.Contains(before, "://") {
		return false
	}
	if i := strings.LastIndex(before, "data:"); i >= 0 && strings.Contains(before[i:], ";base64,") &&
		!strings.HasPrefix(before[i:], "data:text/") {
		return false
	}
	jwtStart := start
	for jwtStart > wordStart && (isBlockChar(text[jwtStart-1]) || text[jwtStart-1] == '.') {
		jwtStart--
	}
	if jwtStart < start && strings.HasPrefix(text[jwtStart:], "eyJ") {
		return false
	}

	// What it is known to be.
	for _, p := range subresourcePrefixes {
		if strings.HasPrefix(tok, p) {
			return false
		}
	}
	for _, m := range binaryMagic {
		if strings.HasPrefix(tok, m) {
			return false
		}
	}

	// What its characters say.
	var counts [256]int
	hexOnly, base32Only := true, true
	flips := 0
	prev := byte(0)
	for k := 0; k < len(tok); k++ {
		c := tok[k]
		counts[c]++
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			hexOnly = false
		}
		if !((c >= 'A' && c <= 'Z') || (c >= '2' && c <= '7') || c == '=') {
			base32Only = false
		}
		class := charClass(c)
		if k > 0 && class != prev {
			flips++
		}
		prev = class
	}
	if hexOnly {
		// A hash, an address or a dump of bytes. Hex that spells text is
		// decoded by the normalizer and scanned as text.
		return false
	}
	entropy := 0.0
	n := float64(len(tok))
	for _, c := range counts {
		if c > 0 {
			p := float64(c) / n
			entropy -= p * math.Log2(p)
		}
	}
	if base32Only {
		if entropy < base32MinEntropy {
			return false
		}
	} else if entropy < unreadableMinEntropy || float64(flips)/(n-1) < unreadableMinFlips {
		return false
	}

	// Whether anything here can read it. If so the scanners have read it
	// as well, and there is nothing unreadable about it.
	return !normalize.DecodesToText(tok)
}

func charClass(c byte) byte {
	switch {
	case c >= 'a' && c <= 'z':
		return 1
	case c >= 'A' && c <= 'Z':
		return 2
	case c >= '0' && c <= '9':
		return 3
	}
	return 4
}
