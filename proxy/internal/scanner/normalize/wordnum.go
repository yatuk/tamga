package normalize

import (
	"strings"
	"unicode"
)

// wordNumEN maps English number words (digits, teens, tens, magnitudes) to
// their integer value. Used by expandWordsToNumbers to collapse phonetically
// spelled numerical evasions back to digits before DFA matching.
var wordNumEN = map[string]int{
	"zero":      0,
	"one":       1,
	"two":       2,
	"three":     3,
	"four":      4,
	"five":      5,
	"six":       6,
	"seven":     7,
	"eight":     8,
	"nine":      9,
	"ten":       10,
	"eleven":    11,
	"twelve":    12,
	"thirteen":  13,
	"fourteen":  14,
	"fifteen":   15,
	"sixteen":   16,
	"seventeen": 17,
	"eighteen":  18,
	"nineteen":  19,
	"twenty":    20,
	"thirty":    30,
	"forty":     40,
	"fifty":     50,
	"sixty":     60,
	"seventy":   70,
	"eighty":    80,
	"ninety":    90,
	"hundred":   100,
	"thousand":  1000,
	"million":   1000000,
	"billion":   1000000000,
}

// wordNumTR maps Turkish number words.
var wordNumTR = map[string]int{
	"sifir": 0, "sıfır": 0,
	"bir": 1,
	"iki": 2,
	"uc":  3, "üç": 3,
	"dort": 4, "dört": 4,
	"bes": 5, "beş": 5,
	"alti": 6, "altı": 6,
	"yedi":  7,
	"sekiz": 8,
	"dokuz": 9,
	"on":    10,
	"yirmi": 20,
	"otuz":  30,
	"kirk":  40, "kırk": 40,
	"elli":   50,
	"altmis": 60, "altmış": 60,
	"yetmis": 70, "yetmiş": 70,
	"seksen": 80,
	"doksan": 90,
	"yuz":    100, "yüz": 100,
	"bin":    1000,
	"milyon": 1000000,
	"milyar": 1000000000,
}

// expandWordsToNumbers scans s, converting runs of spelled-out numbers into
// their numeric digit form. Conservative: only touches sequences of at least
// two consecutive number words, so regular prose ("I have one dog") is
// unaffected.
func expandWordsToNumbers(s string) string {
	if s == "" {
		return s
	}
	// Tokenize on whitespace while preserving the original separators.
	tokens := tokenize(s)
	if len(tokens) < 2 {
		return s
	}

	var out strings.Builder
	out.Grow(len(s))
	i := 0
	for i < len(tokens) {
		run := collectNumberRun(tokens, i)
		if run > 1 {
			if val, ok := parseNumberRun(tokens[i : i+run]); ok {
				// Write the numeric representation.
				out.WriteString(val)
				i += run
				continue
			}
		}
		out.WriteString(tokens[i])
		i++
	}
	return out.String()
}

// tokenize splits s while keeping whitespace tokens so we can rebuild the
// string without losing formatting.
func tokenize(s string) []string {
	var out []string
	var cur strings.Builder
	inWord := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if !inWord {
				if cur.Len() > 0 {
					out = append(out, cur.String())
					cur.Reset()
				}
				inWord = true
			}
			cur.WriteRune(r)
		} else {
			if inWord {
				if cur.Len() > 0 {
					out = append(out, cur.String())
					cur.Reset()
				}
				inWord = false
			}
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

func isNumberWord(t string) bool {
	k := strings.ToLower(t)
	if _, ok := wordNumEN[k]; ok {
		return true
	}
	if _, ok := wordNumTR[k]; ok {
		return true
	}
	return false
}

// collectNumberRun returns how many consecutive number-word tokens start at i,
// skipping pure-whitespace separators between them.
func collectNumberRun(tokens []string, i int) int {
	count := 0
	j := i
	for j < len(tokens) {
		if isNumberWord(tokens[j]) {
			count++
			j++
			continue
		}
		// allow single whitespace separator between words
		if strings.TrimSpace(tokens[j]) == "" && j+1 < len(tokens) && isNumberWord(tokens[j+1]) {
			j++
			continue
		}
		break
	}
	return j - i
}

// parseNumberRun evaluates a run like ["four","one","one","one"] to "4111",
// or ["iki","yuz","kirk","bes"] to "245".
func parseNumberRun(run []string) (string, bool) {
	// First pass: collect numeric values, dropping whitespace-only tokens.
	var values []int
	for _, t := range run {
		if strings.TrimSpace(t) == "" {
			continue
		}
		k := strings.ToLower(t)
		if v, ok := wordNumEN[k]; ok {
			values = append(values, v)
			continue
		}
		if v, ok := wordNumTR[k]; ok {
			values = append(values, v)
			continue
		}
		return "", false
	}
	if len(values) == 0 {
		return "", false
	}
	// If the run is entirely single digits (0-9), treat as digit concatenation
	// — this is the most common exfiltration pattern ("four one one one").
	allDigits := true
	for _, v := range values {
		if v > 9 {
			allDigits = false
			break
		}
	}
	if allDigits {
		var b strings.Builder
		for _, v := range values {
			b.WriteByte('0' + byte(v))
		}
		return b.String(), true
	}

	// If ANY value is a magnitude word (100, 1000, etc.), use the
	// magnitude-based accumulator ("two hundred forty five" → 245).
	hasMagnitude := false
	for _, v := range values {
		if v == 100 || v == 1000 || v == 1000000 || v == 1000000000 {
			hasMagnitude = true
			break
		}
	}

	// No magnitude words: treat as phone-number or digit-group dictation.
	// Compound numbers form digit pairs ("elli beş" = 50+5 = 55, not "505").
	// Walk the values left-to-right, combining tens (10-90) with following
	// ones (1-9) before concatenating.
	if !hasMagnitude {
		var b strings.Builder
		for i := 0; i < len(values); i++ {
			v := values[i]
			if v >= 10 && v <= 90 && i+1 < len(values) && values[i+1] >= 1 && values[i+1] <= 9 {
				// Combine tens + ones: "elli beş" → 55
				b.WriteString(itoa(v + values[i+1]))
				i++ // skip the ones digit
			} else {
				b.WriteString(itoa(v))
			}
		}
		return b.String(), true
	}

	// Generic magnitude-based accumulator (good enough for TR/EN compound
	// numbers like "two hundred forty five" / "iki yuz kirk bes").
	total := 0
	current := 0
	for _, v := range values {
		switch v {
		case 100:
			if current == 0 {
				current = 1
			}
			current *= 100
		case 1000, 1000000, 1000000000:
			if current == 0 {
				current = 1
			}
			total += current * v
			current = 0
		default:
			current += v
		}
	}
	total += current
	return itoa(total), true
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
