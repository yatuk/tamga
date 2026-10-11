package classifier

import (
	"unicode"
	"unicode/utf8"
)

// A change of writing system inside a conversation is one of the signals
// that make the classifier judge a text more strictly. An attack is
// sometimes written in another language than the conversation so that
// filters tuned on English do not read it.
//
// What is detected is the script, not the language: Latin, Cyrillic, Arabic
// and so on, from Go's unicode tables, with no model. Turkish and English
// are both Latin and never count. A change between two languages of one
// script (English to German, Russian to Ukrainian) is not seen at all;
// telling those apart needs a language model, which this is not.
//
// Only a change towards a script other than Latin counts. Someone writing
// in Russian, Arabic or Chinese pastes English logs and code all day; the
// reverse, Latin text followed by a block in another script, is the rare
// one.

// Script is a writing system, as far as this file tells them apart.
type Script uint8

const (
	ScriptNone Script = iota // too few letters to say
	ScriptLatin
	ScriptCyrillic
	ScriptGreek
	ScriptArabic
	ScriptHebrew
	// ScriptCJK is Han, Hiragana, Katakana and Hangul together: Japanese
	// mixes three of them in one sentence.
	ScriptCJK
	ScriptIndic
	ScriptThai
	ScriptOther
	scriptCount
)

var scriptNames = [scriptCount]string{"", "latin", "cyrillic", "greek", "arabic", "hebrew", "cjk", "indic", "thai", "other"}

func (s Script) String() string { return scriptNames[s] }

const (
	// scriptMinLetters is the least number of letters a text, or a half of
	// one, needs before its script is named.
	scriptMinLetters = 12
	// scriptSample bounds how much of one text is read.
	scriptSample = 4096
)

var scriptTables = []struct {
	script Script
	table  *unicode.RangeTable
}{
	{ScriptLatin, unicode.Latin},
	{ScriptCyrillic, unicode.Cyrillic},
	{ScriptArabic, unicode.Arabic},
	{ScriptCJK, unicode.Han},
	{ScriptCJK, unicode.Hiragana},
	{ScriptCJK, unicode.Katakana},
	{ScriptCJK, unicode.Hangul},
	{ScriptGreek, unicode.Greek},
	{ScriptHebrew, unicode.Hebrew},
	{ScriptIndic, unicode.Devanagari},
	{ScriptIndic, unicode.Bengali},
	{ScriptIndic, unicode.Tamil},
	{ScriptIndic, unicode.Telugu},
	{ScriptIndic, unicode.Gujarati},
	{ScriptIndic, unicode.Gurmukhi},
	{ScriptIndic, unicode.Kannada},
	{ScriptIndic, unicode.Malayalam},
	{ScriptThai, unicode.Thai},
}

func scriptOf(r rune) Script {
	if r < utf8.RuneSelf {
		return ScriptLatin
	}
	for _, t := range scriptTables {
		if unicode.Is(t.table, r) {
			return t.script
		}
	}
	return ScriptOther
}

// ScriptCounts is how many letters of each script a text has.
type ScriptCounts [scriptCount]int

// Add counts the letters of text, of its first scriptSample bytes.
func (c *ScriptCounts) Add(text string) {
	if len(text) > scriptSample {
		text = text[:scriptSample]
	}
	for _, r := range text {
		if r < utf8.RuneSelf {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				c[ScriptLatin]++
			}
			continue
		}
		if unicode.IsLetter(r) {
			c[scriptOf(r)]++
		}
	}
}

// Dominant is the script most of the letters are in: at least three in
// five, of at least scriptMinLetters. Otherwise ScriptNone.
func (c *ScriptCounts) Dominant() Script {
	total, best := 0, ScriptLatin
	for s := ScriptLatin; s < scriptCount; s++ {
		total += c[s]
		if c[s] > c[best] {
			best = s
		}
	}
	if total < scriptMinLetters || c[best]*5 < total*3 {
		return ScriptNone
	}
	return best
}

// halves returns the dominant script of the first and of the second half of
// text's letters.
func halves(text string) (Script, Script) {
	if len(text) > scriptSample {
		text = text[:scriptSample]
	}
	letters := 0
	for _, r := range text {
		if unicode.IsLetter(r) {
			letters++
		}
	}
	if letters < 2*scriptMinLetters {
		return ScriptNone, ScriptNone
	}
	var first, second ScriptCounts
	seen := 0
	for _, r := range text {
		if !unicode.IsLetter(r) {
			continue
		}
		if seen < letters/2 {
			first[scriptOf(r)]++
		} else {
			second[scriptOf(r)]++
		}
		seen++
	}
	return first.Dominant(), second.Dominant()
}

// ScriptSwitch reports whether text changes writing system: against the
// script the conversation has been in (before, ScriptNone when there is no
// earlier text or it has no one script), or between its own two halves. It
// returns the script changed to.
func ScriptSwitch(before Script, text string) (Script, bool) {
	var c ScriptCounts
	c.Add(text)
	whole := c.Dominant()
	if before != ScriptNone && whole != ScriptNone && whole != ScriptLatin && whole != before {
		return whole, true
	}
	a, b := halves(text)
	if a == ScriptNone || b == ScriptNone || a == b {
		return ScriptNone, false
	}
	// The half that is not Latin is the one changed to, unless the
	// conversation was in that script already.
	to := b
	if to == ScriptLatin {
		to = a
	}
	if to == before {
		return ScriptNone, false
	}
	return to, true
}
