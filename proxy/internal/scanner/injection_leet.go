package scanner

import (
	"strings"
	"sync"

	"github.com/yatuk/tamga/internal/scanner/normalize"
)

// An instruction written in leetspeak ("1gn0r3 4ll pr3v10us 1nstruct10ns")
// is read by no phrase and no rule as it stands. This file gives the two
// scanners that look for instructions one more view of the text, in which
// the tokens that are leetspeak for a word the rules know are written as
// that word.
//
// The normaliser's own de-leeting (normalize: applyDeleet) rewrites a token
// when at least half of its characters are digits that stand for letters.
// That reads "1gn0r3" and leaves "pr3v10us" and "1nstruct10ns", which is
// most of a phrase. Lowering its threshold would rewrite ordinary tokens
// with a digit in them. What is done here instead is narrower and reads
// more: a token is rewritten only when the result is a word of the rules'
// own vocabulary (leetVocab: the words of the phrase list and of the rules
// next to it). "pr3v10us" becomes "previous"; "w1nd0ws", "pr0d-db-01",
// "h4x0r", "c0ffee", a version string, a product code, an IBAN and base64
// become nothing, because nothing they could stand for is such a word.
//
// So the view cannot say more than the same text written plainly would:
// every word in it that was not in the text is a word the writer put there
// in digits. A request that is benign in plain letters is benign in leet.
//
// Only the instruction scanners get the view, as with the spelled-out one
// (injection_spelled.go). It turns digits into letters, so it holds no
// number that was not there, and no PII or secret scanner reads it.

// leetExtraWords are the words of the paraphrase and role-play expressions
// (injection_paraphrase.go, injection_role.go) that the spelled-out view's
// list does not hold. They are kept apart from it: that list also decides
// where a run of letters without word gaps is cut, and a longer list there
// cuts more.
var leetExtraWords = strings.Fields(`
	everything earlier before told said longer apply applies bound constrained
	restricted protocol protocols imagine suppose consider like limit
	limitations lacking moral response reply different uncensored jailbroken
	unlimited what were
`)

var (
	leetVocabOnce sync.Once
	leetVocab     map[string]bool
	leetVocabMax  int
)

func loadLeetVocab() {
	spelledVocabOnce.Do(loadSpelledVocab)
	leetVocab = make(map[string]bool, len(spelledVocab)+len(leetExtraWords))
	leetVocabMax = spelledVocabMax
	for w := range spelledVocab {
		leetVocab[w] = true
	}
	for _, w := range leetExtraWords {
		leetVocab[w] = true
		if len(w) > leetVocabMax {
			leetVocabMax = len(w)
		}
	}
}

// leetLetter is the letter a leet character stands for, and zero for a
// character that stands for none. The ones with two readings are in
// leetEither.
var leetLetter = [256]byte{
	'0': 'o', '3': 'e', '4': 'a', '5': 's', '6': 'g', '7': 't', '8': 'b', '9': 'g',
	'@': 'a', '$': 's', '+': 't',
}

// leetEither are the characters that stand for "i" in one word and "l" in
// another: "1gnore", "a11".
func leetEither(c byte) bool { return c == '1' || c == '!' || c == '|' }

func isLeetChar(c byte) bool { return leetLetter[c] != 0 || leetEither(c) }

func isLowerLetter(c byte) bool { return c >= 'a' && c <= 'z' }

// isLeetTokenChar reports whether c can be part of a token that may be
// leetspeak: a letter, a leet character, or any other digit, which keeps a
// token whole so that "a2b" is not read as the two tokens around the 2.
func isLeetTokenChar(c byte) bool {
	return isLowerLetter(c) || isLeetChar(c) || (c >= '0' && c <= '9')
}

// leetMaxEither bounds the readings tried for one token: each character
// with two readings doubles them.
const leetMaxEither = 4

// foldLeetToken reads tok, a run of lower-case letters and leet characters,
// as the word of the rules' vocabulary it is leetspeak for. It returns false
// when tok holds no letter or no leet character, or stands for no such word.
func foldLeetToken(tok string) (string, bool) {
	leetVocabOnce.Do(loadLeetVocab)
	if len(tok) < 2 || len(tok) > leetVocabMax {
		return "", false
	}
	letters, leet := 0, 0
	var either []int
	buf := make([]byte, len(tok))
	for i := 0; i < len(tok); i++ {
		c := tok[i]
		switch {
		case isLowerLetter(c):
			letters++
			buf[i] = c
		case leetEither(c):
			leet++
			either = append(either, i)
			buf[i] = 'i'
		case leetLetter[c] != 0:
			leet++
			buf[i] = leetLetter[c]
		default:
			return "", false
		}
	}
	if letters == 0 || leet == 0 {
		return "", false
	}
	if len(either) > leetMaxEither {
		// Too many to try each: all "i", then all "l".
		if leetVocab[string(buf)] {
			return string(buf), true
		}
		for _, i := range either {
			buf[i] = 'l'
		}
		if leetVocab[string(buf)] {
			return string(buf), true
		}
		return "", false
	}
	for mask := 0; mask < 1<<len(either); mask++ {
		for bit, i := range either {
			if mask&(1<<bit) != 0 {
				buf[i] = 'l'
			} else {
				buf[i] = 'i'
			}
		}
		if leetVocab[string(buf)] {
			return string(buf), true
		}
	}
	return "", false
}

// leetMinAnchor is the shortest rewritten token that makes a text one with
// leetspeak in it. A two-character token ("4s", "n0") is rewritten only
// next to a longer one: on its own it is a model number or a typo.
const leetMinAnchor = 3

// leetView returns text with its leetspeak tokens written as the words they
// stand for, and false when there is none. text is the normalised view from
// before de-leeting (normalize.Result.Plain): lower case, folded letters,
// digits still digits.
func leetView(text string) (string, bool) {
	var b strings.Builder
	anchored := false
	last := 0
	for i := 0; i < len(text); {
		if !isLeetTokenChar(text[i]) {
			i++
			continue
		}
		start := i
		letters, leet := false, false
		for i < len(text) && isLeetTokenChar(text[i]) {
			if isLowerLetter(text[i]) {
				letters = true
			} else {
				leet = true
			}
			i++
		}
		// A word character of another alphabet on either side means this
		// was not a token of its own.
		if !letters || !leet || (i < len(text) && text[i] >= 0x80) || (start > 0 && text[start-1] >= 0x80) {
			continue
		}
		// An exclamation mark or a plus at the end is punctuation.
		end := i
		for end > start && (text[end-1] == '!' || text[end-1] == '+') {
			end--
		}
		word, ok := foldLeetToken(text[start:end])
		if !ok {
			continue
		}
		if end-start >= leetMinAnchor {
			anchored = true
		}
		b.WriteString(text[last:start])
		b.WriteString(word)
		last = end
	}
	if !anchored {
		return "", false
	}
	b.WriteString(text[last:])
	return b.String(), true
}

// foldLeetAll reads every leet character of s as a letter, the ones with two
// readings as either.
func foldLeetAll(s string, either byte) (string, bool) {
	buf := []byte(s)
	for i, c := range buf {
		switch {
		case isLowerLetter(c):
		case leetEither(c):
			buf[i] = either
		case leetLetter[c] != 0:
			buf[i] = leetLetter[c]
		default:
			return "", false
		}
	}
	return string(buf), true
}

// spelledLeetView reads one spelled-out run with leet characters in it
// ("1gn0r3 4ll pr3v10us", or "1gn0r3pr3v10us" where the text showed no word
// gaps). A piece that is leetspeak for a known word becomes the word; one
// that is several known words run together is cut into them, as respace
// cuts a spelled run of letters. It returns false when nothing was read.
func spelledLeetView(run string) (string, bool) {
	fields := strings.Fields(run)
	read := false
	for i, f := range fields {
		if word, ok := foldLeetToken(f); ok {
			fields[i], read = word, true
			continue
		}
		for _, either := range []byte{'i', 'l'} {
			all, ok := foldLeetAll(f, either)
			if !ok {
				break
			}
			if cut := respace(all); cut != all {
				fields[i], read = cut, true
				break
			}
		}
		if fields[i] == f {
			fields[i] = respace(f)
		}
	}
	if !read {
		return "", false
	}
	return strings.Join(fields, " "), true
}

// leetViews returns the leetspeak readings of a normalised text: the text
// itself with its leet tokens read, and each spelled-out run that holds
// leet characters.
func leetViews(norm normalize.Result) []string {
	var out []string
	if v, ok := leetView(norm.Plain); ok {
		out = append(out, v)
	}
	for _, run := range norm.SpelledLeet {
		if v, ok := spelledLeetView(run); ok {
			out = append(out, v)
		}
	}
	return out
}
