package normalize

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Characters that take no room on the screen come in two kinds, and the
// scanners need a different answer for each.
//
// Most of them carry nothing: a zero-width space between two digits, a soft
// hyphen inside a word, a direction mark. They are dropped from the view the
// scanners match on, so a pattern split by them is found. Many have honest
// uses (the joiner inside an emoji or a Persian word, a soft hyphen out of a
// PDF, a byte order mark at the start of a file), and dropping them from
// the view costs those texts nothing: the request is forwarded as written.
//
// A few can carry text of their own. Tag characters are a copy of ASCII
// that no font draws, and a run of variation selectors can stand for
// arbitrary bytes. What they spell is read out by FindHidden and given to
// the scanners as one more view.
//
// The categories come from Go's unicode tables: Cc, Cf, Co, Cs and the
// unassigned code points are whatever unicode.IsGraphic rejects, and the
// invisible characters that have a graphic category (variation selectors,
// the grapheme joiner, the Hangul fillers) are the Variation_Selector and
// Other_Default_Ignorable_Code_Point properties. "Unassigned" therefore
// means unassigned in the Unicode version of the Go release the proxy was
// built with.

const (
	tagFirst  = 0xE0020 // TAG SPACE
	tagLast   = 0xE007E // TAG TILDE
	tagCancel = 0xE007F
	tagBase   = 0xE0000
	// blackFlag starts the only tag sequences in ordinary use: the flags of
	// England, Scotland and Wales.
	blackFlag = 0x1F3F4
)

// invisible says what the scanners' view does with r: keep it, drop it, or
// put the ASCII byte repl in its place.
func invisible(r rune) (drop bool, repl byte) {
	switch {
	case r == '\t' || r == '\n' || r == '\r':
		return false, 0
	case r < 0x20:
		if r == '\v' || r == '\f' {
			return true, ' '
		}
		return true, 0
	case r < 0x7F:
		return false, 0
	case r < 0xA0: // DEL and the C1 controls
		if r == 0x85 { // NEXT LINE
			return true, '\n'
		}
		return true, 0
	case r == 0xA0: // NO-BREAK SPACE
		return true, ' '
	case r == 0xAD: // SOFT HYPHEN
		return true, 0
	case r < 0x300:
		return false, 0
	case r == 0x2028 || r == 0x2029: // LINE and PARAGRAPH SEPARATOR
		return true, '\n'
	}
	if !unicode.IsGraphic(r) {
		// Cf, Co, Cs and what is not assigned.
		return true, 0
	}
	if r == utf8.RuneError {
		return false, 0
	}
	if unicode.Is(unicode.Variation_Selector, r) || unicode.Is(unicode.Other_Default_Ignorable_Code_Point, r) {
		return true, 0
	}
	return false, 0
}

// mayHoldInvisible is the cheap test that lets plain ASCII text through
// without a pass over its runes.
func mayHoldInvisible(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 0x7F || (c < 0x20 && c != '\t' && c != '\n' && c != '\r') {
			return true
		}
	}
	return false
}

// stripInvisible removes from s the characters that do not show, so that a
// pattern split by them matches (4<U+200B>111, ig<U+00AD>nore). A no-break
// space and the vertical controls become a space, the Unicode line
// separators a line feed.
func stripInvisible(s string) string {
	if !mayHoldInvisible(s) {
		return s
	}
	var b strings.Builder
	last := 0
	changed := false
	for i := 0; i < len(s); {
		r, size := rune(s[i]), 1
		if r >= utf8.RuneSelf {
			r, size = utf8.DecodeRuneInString(s[i:])
		}
		if drop, repl := invisible(r); drop {
			if !changed {
				b.Grow(len(s))
				changed = true
			}
			b.WriteString(s[last:i])
			if repl != 0 {
				b.WriteByte(repl)
			}
			last = i + size
		}
		i += size
	}
	if !changed {
		return s
	}
	b.WriteString(s[last:])
	return b.String()
}

// HiddenSpan is where one kind of invisible text lies in a string: from the
// first of its characters to the end of the last, and how many there are.
type HiddenSpan struct {
	Start, End int
	Count      int
}

func (h *HiddenSpan) add(start, end int) {
	if h.Count == 0 {
		h.Start = start
	}
	h.End = end
	h.Count++
}

// Hidden is what a text carries in characters that do not show.
type Hidden struct {
	// TagText is the ASCII written in tag characters, in the order it
	// appears. The tags of a flag emoji are left out.
	TagText string
	Tags    HiddenSpan
	// SelectorText is what the runs of two or more variation selectors
	// spell when each is read as a byte, if that is text. One selector
	// after a character is the ordinary use and is not counted.
	SelectorText string
	Selectors    HiddenSpan
	// FormatRun is the longest unbroken run of other invisible format
	// characters: zero-width spaces and joiners, direction marks and the
	// like. One or two between letters is normal; a long run is a channel.
	FormatRun HiddenSpan
	// Overrides are the bidirectional override characters (U+202D, U+202E),
	// which make a reader see text in another order than it is written in.
	Overrides HiddenSpan
}

// mayHoldHidden reports whether s has any of the lead bytes the characters
// FindHidden looks for start with: U+2000..U+207F (E2 80, E2 81),
// U+FE00..U+FE0F (EF B8), U+FEFF (EF BB) and plane 14 (F3 A0).
func mayHoldHidden(s string) bool {
	for i := 0; i+1 < len(s); i++ {
		switch s[i] {
		case 0xE2:
			if s[i+1] == 0x80 || s[i+1] == 0x81 {
				return true
			}
		case 0xEF:
			if s[i+1] == 0xB8 || s[i+1] == 0xBB {
				return true
			}
		case 0xF3:
			if s[i+1] == 0xA0 {
				return true
			}
		}
	}
	return false
}

// selectorByte reads a variation selector as the byte it stands for in the
// usual smuggling scheme: U+FE00..U+FE0F are 0 to 15, U+E0100..U+E01EF the
// rest.
func selectorByte(r rune) (byte, bool) {
	switch {
	case r >= 0xFE00 && r <= 0xFE0F:
		return byte(r - 0xFE00), true
	case r >= 0xE0100 && r <= 0xE01EF:
		return byte(r - 0xE0100 + 16), true
	}
	return 0, false
}

// FindHidden reads out what s carries in invisible characters.
func FindHidden(s string) Hidden {
	var h Hidden
	if !mayHoldHidden(s) {
		return h
	}
	var tags, selectors []byte

	// A flag emoji is a black flag, up to six tag letters and a cancel
	// tag. Tags that start like one and do not end like one are text.
	var flagTags []byte
	var flagSpan HiddenSpan
	afterFlag := false
	addTag := func(c byte, start, end int) {
		tags = append(tags, c)
		h.Tags.add(start, end)
	}
	notAFlag := func() {
		for k, c := range flagTags {
			// The tags of one run are four bytes each.
			addTag(c, flagSpan.Start+4*k, flagSpan.Start+4*k+4)
		}
		flagTags, flagSpan = flagTags[:0], HiddenSpan{}
	}

	var selRun []byte
	var selSpan HiddenSpan
	endSelectors := func() {
		if len(selRun) >= 2 {
			selectors = append(selectors, selRun...)
			if h.Selectors.Count == 0 {
				h.Selectors.Start = selSpan.Start
			}
			h.Selectors.End = selSpan.End
			h.Selectors.Count += selSpan.Count
		}
		selRun, selSpan = selRun[:0], HiddenSpan{}
	}

	var run HiddenSpan
	endRun := func() {
		if run.Count > h.FormatRun.Count {
			h.FormatRun = run
		}
		run = HiddenSpan{}
	}

	for i := 0; i < len(s); {
		r, size := rune(s[i]), 1
		if r >= utf8.RuneSelf {
			r, size = utf8.DecodeRuneInString(s[i:])
		}
		end := i + size

		if r >= tagFirst && r <= tagCancel {
			endSelectors()
			endRun()
			switch {
			case r == tagCancel:
				if !afterFlag || len(flagTags) == 0 {
					notAFlag()
				}
				flagTags, flagSpan = flagTags[:0], HiddenSpan{}
				afterFlag = false
			case afterFlag && len(flagTags) < 6:
				flagTags = append(flagTags, byte(r-tagBase))
				flagSpan.add(i, end)
			default:
				notAFlag()
				afterFlag = false
				addTag(byte(r-tagBase), i, end)
			}
			i = end
			continue
		}
		notAFlag()
		afterFlag = r == blackFlag

		if c, ok := selectorByte(r); ok {
			endRun()
			selRun = append(selRun, c)
			selSpan.add(i, end)
			i = end
			continue
		}
		endSelectors()

		switch {
		case r == 0x202D || r == 0x202E:
			h.Overrides.add(i, end)
			run.add(i, end)
		case r >= 0x300 && unicode.Is(unicode.Cf, r):
			run.add(i, end)
		default:
			endRun()
		}
		i = end
	}
	notAFlag()
	endSelectors()
	endRun()

	h.TagText = string(tags)
	if looksLikeText(selectors) {
		h.SelectorText = string(selectors)
	}
	return h
}

// hiddenViews returns the hidden text of s that is long enough to be a
// word, for the scanners to read as further views.
func hiddenViews(s string) []string {
	h := FindHidden(s)
	var out []string
	if len(h.TagText) >= 4 {
		out = append(out, h.TagText)
	}
	if len(h.SelectorText) >= 4 {
		out = append(out, h.SelectorText)
	}
	return out
}
