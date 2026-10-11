package normalize

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// ────────────────────────────────────────────────────────────────────────────
// Token Smuggling Detection — 2.4 Multi-Language Hardening
// ────────────────────────────────────────────────────────────────────────────
//
// Attackers split injection keywords with spaces to evade substring-based
// DFA matching:
//
//	"i g n o r e"                 → "ignore"  (character-by-character)
//	"i g n o r e p r e v i o u s" → "ignoreprevious" (then matches DFA)
//
// This file provides conservative whitespace collapsing that only targets
// the unambiguous case: runs of ≥3 single-letter tokens separated by single
// spaces.  This catches character-by-character splitting while avoiding
// false positives on legitimate short word pairs ("I am", "a dog").
//
// Known limitations (left for future dictionary-based approach):
//   - "I gnore" (one single-letter + one multi-letter) — NOT collapsed
//   - "ig nore pre vious" (multi-letter fragments) — NOT collapsed
// ────────────────────────────────────────────────────────────────────────────

// collapseSplitWords finds runs of ≥3 consecutive single-letter tokens
// separated by single spaces, then removes the spaces to rejoin the original
// word.  Multi-letter tokens, punctuation, and multi-space gaps act as
// run boundaries and are left unchanged.
//
// Example: "i g n o r e" → "ignore"
// Example: "p l e a s e i g n o r e" → "please ignore"
// Example: "I am here" → "I am here" (2 single-letter tokens — below threshold)
func collapseSplitWords(s string) string {
	tokens := tokenize(s)
	if len(tokens) < 5 {
		// Shortest split-word pattern: letter + space + letter + space + letter = 5 tokens
		return s
	}

	var out strings.Builder
	out.Grow(len(s))
	i := 0
	for i < len(tokens) {
		runLen := splitRunLen(tokens, i)
		if runLen >= 5 {
			// Collapse the run: write all letter tokens, skip spaces.
			for j := i; j < i+runLen; j++ {
				t := tokens[j]
				if strings.TrimSpace(t) != "" {
					out.WriteString(t)
				}
			}
			i += runLen
		} else {
			out.WriteString(tokens[i])
			i++
		}
	}
	return out.String()
}

// splitRunLen returns the length (in tokens) of a suspicious split-word run
// starting at tokens[i].  A split-word run is a sequence of:
//
//	letter-token, space-token, letter-token, space-token, ...
//
// where every letter-token is a SINGLE letter and every space-token is
// a SINGLE space character.  The run must contain ≥3 letter tokens
// (i.e. ≥5 tokens total: L S L S L) to be considered suspicious.
//
// The run stops at the last consumed single-letter token — a trailing space
// that precedes a multi-letter token (or end of input) is NOT included.
// This prevents the space between the collapsed word and the following word
// from being swallowed.
func splitRunLen(tokens []string, i int) int {
	if i >= len(tokens) {
		return 0
	}
	if !isSingleLetter(tokens[i]) {
		return 0
	}

	j := i + 1
	letterCount := 1
	lastValid := i // always points to the last consumed single-letter token
	for j < len(tokens) {
		// Expect a single space.
		if tokens[j] != " " {
			break
		}
		j++
		if j >= len(tokens) {
			break
		}
		// Expect a single letter.
		if !isSingleLetter(tokens[j]) {
			break
		}
		letterCount++
		lastValid = j
		j++
	}

	if letterCount < 3 {
		return 0
	}
	return lastValid - i + 1
}

// ────────────────────────────────────────────────────────────────────────────
// Phrases spelled one letter at a time
// ────────────────────────────────────────────────────────────────────────────
//
// collapseSplitWords above joins a word whose letters are spaced out. It
// does not read a whole phrase written that way:
//
//	"i g n o r e p r e v i o u s i n s t r u c t i o n s"  one run, no word ends
//	"i.g.n.o.r.e a.l.l p.r.e.v.i.o.u.s"                   dots, not spaces
//	"i-g-n-o-r-e   a-l-l"                                 dashes, wide gaps
//
// spelledRuns reads these. A run is a row of single letters with one
// separator between two letters of a word (a space, a dot, a dash or an
// underscore) and something wider, or another separator, between two words.
// Invisible characters need no mention here: they are dropped before this
// pass runs, so letters split by them are already words.
//
// Only letters make a run. A digit ends it, so a row of digits in a table
// is never joined into a number here, and so does any other character.
// ────────────────────────────────────────────────────────────────────────────

const (
	// spelledMinLetters is how many single letters a run needs. Shorter
	// rows are ordinary text: "a b c", the options "a b c d", initials,
	// "U S A", a spelled-out acronym. Eight letters is one long word or two
	// short ones, the least a phrase can be.
	spelledMinLetters = 8
	// spelledMaxGap is the widest stretch of separators that still belongs
	// to a run, as the gap between two words: "   ", " - ", ". ".
	spelledMaxGap = 4
)

// isSpellSeparator reports whether c can stand between two letters of a
// spelled-out word, or (several of them) between two such words.
func isSpellSeparator(c byte) bool {
	return c == ' ' || c == '.' || c == '-' || c == '_' || c == '\t'
}

// letterAt decodes the rune at s[i:] and says whether it is a letter or a
// digit. Underscore is neither here: it is one of the separators.
func letterAt(s string, i int) (r rune, size int, letter, digit bool) {
	c := s[i]
	if c < utf8.RuneSelf {
		switch {
		case (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z'):
			return rune(c), 1, true, false
		case c >= '0' && c <= '9':
			return rune(c), 1, false, true
		}
		return rune(c), 1, false, false
	}
	r, size = utf8.DecodeRuneInString(s[i:])
	return r, size, unicode.IsLetter(r), unicode.IsDigit(r)
}

// singleLetterAt reports whether a letter stands alone at s[i:]: what
// follows it is not a letter or a digit. The caller knows what precedes it.
func singleLetterAt(s string, i int) (r rune, size int, ok bool) {
	if i >= len(s) {
		return 0, 0, false
	}
	r, size, letter, _ := letterAt(s, i)
	if !letter {
		return 0, 0, false
	}
	if next := i + size; next < len(s) {
		if _, _, l, d := letterAt(s, next); l || d {
			return 0, 0, false
		}
	}
	return r, size, true
}

// spelledRuns returns what the runs of single letters in s spell: the
// letters of a word joined, the words of a run separated by one space. A run
// is returned when it has at least spelledMinLetters letters and at least
// two of them are joined; a row of one-letter words spells nothing.
//
// Where one word ends and the next begins is only known when the text shows
// it. "i g n o r e p r e v i o u s" comes back as "ignoreprevious"; finding
// the words in that is left to the caller, who knows which words matter.
func spelledRuns(s string) []string {
	var out []string
	// One buffer for every run: ordinary text is full of single letters
	// ("I", "a") that start a run of one.
	var buf []byte
	inWord := false // the previous character is a letter or a digit
	for i := 0; i < len(s); {
		r, size, letter, digit := letterAt(s, i)
		if inWord || !letter {
			inWord = letter || digit
			i += size
			continue
		}
		if _, _, ok := singleLetterAt(s, i); !ok {
			inWord = true
			i += size
			continue
		}

		// s[i] is a single letter: read the run that starts here.
		buf = utf8.AppendRune(buf[:0], r)
		letters, joins := 1, 0
		var letterSep byte // what separates the letters of a word in this run
		j := i + size
		for {
			k := j
			for k < len(s) && k-j <= spelledMaxGap && isSpellSeparator(s[k]) {
				k++
			}
			gap := k - j
			if gap == 0 || gap > spelledMaxGap {
				break
			}
			nr, nsize, ok := singleLetterAt(s, k)
			if !ok {
				break
			}
			if gap == 1 && letterSep == 0 {
				letterSep = s[j]
			}
			if gap == 1 && s[j] == letterSep {
				joins++
			} else {
				buf = append(buf, ' ')
			}
			buf = utf8.AppendRune(buf, nr)
			letters++
			j = k + nsize
		}
		if letters >= spelledMinLetters && joins > 0 {
			out = append(out, string(buf))
		}
		// The run ended on a single letter, which nothing alphanumeric
		// follows.
		inWord = false
		i = j
	}
	return out
}

// isLeetRunChar reports whether c is a digit or symbol that stands for a
// letter in leetspeak and can take a letter's place in a spelled-out run.
func isLeetRunChar(c byte) bool {
	switch c {
	case '0', '1', '3', '4', '5', '7', '8', '9', '@', '$':
		return true
	}
	return false
}

// mayHoldLeetRun is the cheap test in front of spelledLeetRuns: a leet
// character with a separator on both sides.
func mayHoldLeetRun(s string) bool {
	for i := 0; i < len(s); i++ {
		if isLeetRunChar(s[i]) && (i == 0 || isSpellSeparator(s[i-1])) && (i+1 == len(s) || isSpellSeparator(s[i+1])) {
			return true
		}
	}
	return false
}

// leetMemberAt reports whether a member of a leet run stands alone at s[i:]:
// a single letter, or a single character that stands in for one.
func leetMemberAt(s string, i int) (r rune, size int, ok, standIn bool) {
	if r, size, ok = singleLetterAt(s, i); ok {
		return r, size, true, false
	}
	if i >= len(s) || !isLeetRunChar(s[i]) {
		return 0, 0, false, false
	}
	if next := i + 1; next < len(s) {
		if _, _, l, d := letterAt(s, next); l || d || isLeetRunChar(s[next]) {
			return 0, 0, false, false
		}
	}
	return rune(s[i]), 1, true, true
}

// spelledLeetRuns returns the spelled-out runs of s in which a digit or a
// symbol stands where a letter would ("1 g n 0 r 3"), with those characters
// as they are: which letter each stands for is for the caller, who knows the
// words that matter. A run needs at least as many letters as stand-ins, so
// a row of numbers in a table is not one. Runs made of letters
// only are spelledRuns' and are not repeated here.
//
// It is spelledRuns with one more kind of member, kept apart from it so
// that the pass every text goes through stays as it was.
func spelledLeetRuns(s string) []string {
	if !mayHoldLeetRun(s) {
		return nil
	}
	var out []string
	var buf []byte
	inWord := false // the previous character is a letter or a digit
	for i := 0; i < len(s); {
		r, size, letter, digit := letterAt(s, i)
		if inWord || !(letter || isLeetRunChar(s[i])) {
			inWord = letter || digit
			i += size
			continue
		}
		_, _, ok, standIn := leetMemberAt(s, i)
		if !ok {
			inWord = true
			i += size
			continue
		}

		buf = utf8.AppendRune(buf[:0], r)
		members, joins, standIns := 1, 0, 0
		if standIn {
			standIns++
		}
		var letterSep byte
		j := i + size
		for {
			k := j
			for k < len(s) && k-j <= spelledMaxGap && isSpellSeparator(s[k]) {
				k++
			}
			gap := k - j
			if gap == 0 || gap > spelledMaxGap {
				break
			}
			nr, nsize, ok, standIn := leetMemberAt(s, k)
			if !ok {
				break
			}
			if standIn {
				standIns++
			}
			if gap == 1 && letterSep == 0 {
				letterSep = s[j]
			}
			if gap == 1 && s[j] == letterSep {
				joins++
			} else {
				buf = append(buf, ' ')
			}
			buf = utf8.AppendRune(buf, nr)
			members++
			j = k + nsize
		}
		if members >= spelledMinLetters && joins > 0 && standIns > 0 && standIns*2 <= members {
			out = append(out, string(buf))
		}
		inWord = false
		i = j
	}
	return out
}

// isSingleLetter reports whether token is exactly one ASCII letter.
func isSingleLetter(token string) bool {
	return len(token) == 1 &&
		((token[0] >= 'a' && token[0] <= 'z') ||
			(token[0] >= 'A' && token[0] <= 'Z'))
}
