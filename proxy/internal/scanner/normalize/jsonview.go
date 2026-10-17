package normalize

import (
	"strconv"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"
)

// A tool that answers in JSON writes its text inside string values, and the
// encoder may write any character of it as an escape: \u0069gnore is
// "ignore", and so is the same string encoded twice, \\u0069gnore, when one
// tool wraps another's output. The text as written then holds none of the
// words, and a pattern run on it finds nothing. JSONUnits reads the
// document and gives its strings back decoded, with, for every byte, the
// span of the text as written it came from: a match in a reading can be
// put back on the escapes it was read from.
//
// The strings are read in units, and a unit is what one party wrote in one
// place: the string values of one object, the strings of one array, or a
// document that is a single string. The values of a unit are joined with a
// line break, so that an instruction split over the name and the
// description of one item reads as one text. Two units are never joined:
// the scanners find an instruction across almost any separator, and the
// title of one search result followed by the title of the next is not a
// text anybody wrote. A key is not part of its object's unit: between two
// values it would cut the very sentence the unit exists to read whole. The
// text as written, keys included, is read anyway; keys written with escapes
// get a unit of their own.
//
// A string value that holds a JSON document is read as that document, to a
// depth of jsonMaxNesting documents one inside another.

// Bounds on what JSONUnits reads.
const (
	// jsonMaxBytes is the longest text read. A longer tool result is still
	// scanned as written; it is not read a second time here.
	jsonMaxBytes = 1 << 20
	// jsonMaxNesting is how many documents written as strings inside one
	// another are read: a tool's output carried by a gateway, carried by
	// another, and one more.
	jsonMaxNesting = 4
	// jsonMaxDepth bounds objects and arrays inside one another.
	jsonMaxDepth = 64
	// jsonMaxUnits bounds the units of one text. The strings of the units
	// past it are added to the last one, so nothing is left unread; a
	// document of thousands of records is read as fewer, longer texts.
	jsonMaxUnits = 512
)

// JSONField is one string of a JSON document, decoded.
type JSONField struct {
	// From and To delimit the bytes of the text as written that the
	// string's content was read from, its quotes left out.
	From, To int
	// View is the decoded string. Its spans are spans of the text as
	// written.
	View View
	// Escaped says the text as written does not show the string as it
	// is: it holds an escape, or it was read out of a document that was
	// itself a string.
	Escaped bool
	// Hidden says an escape in it, or in the document it was read out of,
	// can hide what a pattern for a value looks for: any escape but those
	// of a quote, a backslash, an apostrophe and the <, > and & that
	// encoders write for HTML. A digit or a slash written as an escape, or
	// a line break right before a number, can.
	Hidden bool
	// letters says a letter or a digit of it is an escape in its own
	// document.
	letters bool
}

// JSONUnit is a part of a JSON document read as one text.
type JSONUnit struct {
	// View is the unit's strings joined by line breaks, located on the text
	// as written.
	View   View
	Fields []JSONField
}

// LooksLikeJSON reports whether s, without the white space around it,
// opens and closes like a JSON object, array or string. It is a cheap test
// before JSONUnits, not a parse.
func LooksLikeJSON(s string) bool {
	i, j := 0, len(s)
	for i < j && isJSONSpace(s[i]) {
		i++
	}
	for j > i && isJSONSpace(s[j-1]) {
		j--
	}
	if j-i < 2 {
		return false
	}
	switch s[i] {
	case '{':
		return s[j-1] == '}'
	case '[':
		return s[j-1] == ']'
	case '"':
		return s[j-1] == '"'
	}
	return false
}

// JSONUnits reads s as one JSON document and returns its units. It returns
// nil when s is not a JSON document, is longer than jsonMaxBytes, nests
// deeper than jsonMaxDepth, or holds no string.
func JSONUnits(s string) []JSONUnit {
	if len(s) > jsonMaxBytes || !LooksLikeJSON(s) {
		return nil
	}
	p := &jsonReader{}
	if !p.document(SourceView(s), 0, false) {
		return nil
	}
	units := make([]JSONUnit, 0, len(p.units))
	for _, fields := range p.units {
		if len(fields) == 0 {
			continue
		}
		units = append(units, JSONUnit{View: joinFields(fields), Fields: fields})
	}
	if len(units) == 0 {
		return nil
	}
	return units
}

type jsonReader struct {
	units [][]JSONField
}

// add records the strings of one unit.
func (p *jsonReader) add(fields []JSONField) {
	if len(fields) == 0 {
		return
	}
	if len(p.units) >= jsonMaxUnits {
		last := len(p.units) - 1
		p.units[last] = append(p.units[last], fields...)
		return
	}
	p.units = append(p.units, fields)
}

// document reads in.Text as a whole JSON document. nesting is how many
// documents written as strings it lies in.
func (p *jsonReader) document(in View, nesting int, hidden bool) bool {
	before := len(p.units)
	d := &jsonDoc{in: in, s: in.Text, nesting: nesting, hidden: hidden, r: p}
	d.space()
	var own []JSONField
	if d.pos < len(d.s) && d.s[d.pos] == '"' {
		// A document that is one string is a unit of its own.
		f, ok := d.str()
		if !ok {
			p.units = p.units[:before]
			return false
		}
		own = d.value(own, f)
	} else if !d.value0(0) {
		p.units = p.units[:before]
		return false
	}
	d.space()
	if d.pos != len(d.s) {
		p.units = p.units[:before]
		return false
	}
	p.add(own)
	return true
}

type jsonDoc struct {
	in      View
	s       string
	pos     int
	nesting int
	// hidden says the string this document was read out of is Hidden.
	hidden bool
	r      *jsonReader
}

func isJSONSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' }

func (d *jsonDoc) space() {
	for d.pos < len(d.s) && isJSONSpace(d.s[d.pos]) {
		d.pos++
	}
}

// value adds a string to the unit being collected, or, when the string
// holds a JSON document that can be read, reads that document instead.
func (d *jsonDoc) value(unit []JSONField, f JSONField) []JSONField {
	if f.View.Text == "" {
		return unit
	}
	if d.nesting+1 < jsonMaxNesting && LooksLikeJSON(f.View.Text) && d.r.document(f.View, d.nesting+1, f.Hidden) {
		return unit
	}
	return append(unit, f)
}

// value0 reads a value that is not a member of the unit at hand: the
// document itself, or an object or array inside another.
func (d *jsonDoc) value0(depth int) bool {
	if depth > jsonMaxDepth || d.pos >= len(d.s) {
		return false
	}
	switch c := d.s[d.pos]; {
	case c == '{':
		return d.object(depth + 1)
	case c == '[':
		return d.array(depth + 1)
	case c == '"':
		f, ok := d.str()
		if ok {
			d.r.add(d.value(nil, f))
		}
		return ok
	default:
		return d.scalar()
	}
}

func (d *jsonDoc) object(depth int) bool {
	d.pos++ // {
	var unit, keys []JSONField
	d.space()
	if d.pos < len(d.s) && d.s[d.pos] == '}' {
		d.pos++
		return true
	}
	for {
		d.space()
		if d.pos >= len(d.s) || d.s[d.pos] != '"' {
			return false
		}
		key, ok := d.str()
		if !ok {
			return false
		}
		if key.letters {
			keys = append(keys, key)
		}
		d.space()
		if d.pos >= len(d.s) || d.s[d.pos] != ':' {
			return false
		}
		d.pos++
		d.space()
		if d.pos < len(d.s) && d.s[d.pos] == '"' {
			f, ok := d.str()
			if !ok {
				return false
			}
			unit = d.value(unit, f)
		} else if !d.value0(depth) {
			return false
		}
		d.space()
		if d.pos >= len(d.s) {
			return false
		}
		if d.s[d.pos] == ',' {
			d.pos++
			continue
		}
		if d.s[d.pos] != '}' {
			return false
		}
		d.pos++
		d.r.add(unit)
		d.r.add(keys)
		return true
	}
}

func (d *jsonDoc) array(depth int) bool {
	d.pos++ // [
	var unit []JSONField
	d.space()
	if d.pos < len(d.s) && d.s[d.pos] == ']' {
		d.pos++
		return true
	}
	for {
		d.space()
		if d.pos < len(d.s) && d.s[d.pos] == '"' {
			f, ok := d.str()
			if !ok {
				return false
			}
			unit = d.value(unit, f)
		} else if !d.value0(depth) {
			return false
		}
		d.space()
		if d.pos >= len(d.s) {
			return false
		}
		if d.s[d.pos] == ',' {
			d.pos++
			continue
		}
		if d.s[d.pos] != ']' {
			return false
		}
		d.pos++
		d.r.add(unit)
		return true
	}
}

// scalar skips a number, true, false or null.
func (d *jsonDoc) scalar() bool {
	start := d.pos
	for d.pos < len(d.s) {
		c := d.s[d.pos]
		if c == ',' || c == '}' || c == ']' || isJSONSpace(c) {
			break
		}
		d.pos++
	}
	switch tok := d.s[start:d.pos]; tok {
	case "true", "false", "null":
		return true
	case "":
		return false
	default:
		_, err := strconv.ParseFloat(tok, 64)
		return err == nil && (tok[0] == '-' || (tok[0] >= '0' && tok[0] <= '9'))
	}
}

// str reads the string at d.pos and decodes it, keeping for each byte of
// the decoded text the span of the text as written it came from.
func (d *jsonDoc) str() (JSONField, bool) {
	d.pos++ // opening quote
	from := d.pos
	b := &spanBuilder{}
	letters, escapes, hidden := false, false, false
	for {
		if d.pos >= len(d.s) {
			return JSONField{}, false
		}
		c := d.s[d.pos]
		switch {
		case c == '"':
			to := d.pos
			d.pos++
			f := JSONField{View: b.view(), Escaped: escapes || d.nesting > 0, Hidden: hidden || d.hidden, letters: letters}
			if from < to {
				f.From, f.To, _ = d.in.Span(from, to)
			}
			return f, true
		case c < 0x20:
			return JSONField{}, false
		case c != '\\':
			// A run of bytes written as they are.
			j := d.pos + 1
			for j < len(d.s) && d.s[j] != '"' && d.s[j] != '\\' && d.s[j] >= 0x20 {
				j++
			}
			b.copy(d.in, d.pos, j)
			d.pos = j
		default:
			if d.pos+1 >= len(d.s) {
				return JSONField{}, false
			}
			start := d.pos
			escapes = true
			var r rune
			switch e := d.s[d.pos+1]; e {
			case '"', '\\', '/':
				r = rune(e)
				d.pos += 2
			case 'b':
				r, d.pos = '\b', d.pos+2
			case 'f':
				r, d.pos = '\f', d.pos+2
			case 'n':
				r, d.pos = '\n', d.pos+2
			case 'r':
				r, d.pos = '\r', d.pos+2
			case 't':
				r, d.pos = '\t', d.pos+2
			case 'u':
				u, ok := hex4(d.s, d.pos+2)
				if !ok {
					return JSONField{}, false
				}
				d.pos += 6
				r = rune(u)
				if utf16.IsSurrogate(r) {
					// A pair stands for one character; half of one is the
					// replacement character, as encoding/json reads it.
					if lo, ok := hex4(d.s, d.pos+2); ok && d.pos+1 < len(d.s) && d.s[d.pos] == '\\' && d.s[d.pos+1] == 'u' {
						if pair := utf16.DecodeRune(r, rune(lo)); pair != utf8.RuneError {
							r = pair
							d.pos += 6
						} else {
							r = utf8.RuneError
						}
					} else {
						r = utf8.RuneError
					}
				}
				if unicode.IsLetter(r) || unicode.IsDigit(r) {
					letters = true
				}
			default:
				return JSONField{}, false
			}
			switch r {
			case '"', '\\', '\'', '<', '>', '&':
			default:
				hidden = true
			}
			b.put(d.in, start, d.pos, string(r))
		}
	}
}

func hex4(s string, at int) (uint64, bool) {
	if at+4 > len(s) {
		return 0, false
	}
	v, err := strconv.ParseUint(s[at:at+4], 16, 32)
	return v, err == nil
}

// spanBuilder builds a located view out of pieces of other views.
type spanBuilder struct {
	out        []byte
	start, end []int32
}

// copy appends in.Text[from:to] as it is.
func (b *spanBuilder) copy(in View, from, to int) {
	for i := from; i < to; i++ {
		a, z, _ := in.Span(i, i+1)
		b.out = append(b.out, in.Text[i])
		b.start = append(b.start, int32(a))
		b.end = append(b.end, int32(z))
	}
}

// put appends s, every byte of which stands for in.Text[from:to].
func (b *spanBuilder) put(in View, from, to int, s string) {
	a, z, _ := in.Span(from, to)
	b.out = append(b.out, s...)
	for range len(s) {
		b.start = append(b.start, int32(a))
		b.end = append(b.end, int32(z))
	}
}

// putSpan appends s standing for the source span [a, z).
func (b *spanBuilder) putSpan(a, z int, s string) {
	b.out = append(b.out, s...)
	for range len(s) {
		b.start = append(b.start, int32(a))
		b.end = append(b.end, int32(z))
	}
}

// add appends a located view.
func (b *spanBuilder) add(v View) {
	for i := 0; i < len(v.Text); i++ {
		a, z, _ := v.Span(i, i+1)
		b.out = append(b.out, v.Text[i])
		b.start = append(b.start, int32(a))
		b.end = append(b.end, int32(z))
	}
}

func (b *spanBuilder) view() View {
	return View{Text: string(b.out), located: true, start: b.start, end: b.end}
}

// joinFields joins the strings of a unit with line breaks. A line break
// stands for the bytes between the two strings it separates.
func joinFields(fields []JSONField) View { return JoinFields(fields, "\n") }

// JoinFields joins strings with sep. The bytes of sep stand for the bytes
// of the text as written between the two strings they separate.
func JoinFields(fields []JSONField, sep string) View {
	if len(fields) == 1 {
		return fields[0].View
	}
	b := &spanBuilder{}
	for i, f := range fields {
		if i > 0 {
			b.separate(fields[i-1].To, f.From, sep)
		}
		b.add(f.View)
	}
	return b.view()
}

// JoinUnits joins units with sep, as JoinFields joins strings.
func JoinUnits(units []JSONUnit, sep string) View {
	if len(units) == 1 {
		return units[0].View
	}
	b := &spanBuilder{}
	for i, u := range units {
		if i > 0 {
			prev := units[i-1].Fields
			b.separate(prev[len(prev)-1].To, u.Fields[0].From, sep)
		}
		b.add(u.View)
	}
	return b.view()
}

// separate appends sep standing for the bytes from a to z, or, when the two
// pieces it separates are not in that order (a nested document's units come
// before the unit of the object that holds it), for the first byte of the
// second.
func (b *spanBuilder) separate(a, z int, sep string) {
	if z <= a {
		a, z = z, z+1
	}
	b.putSpan(a, z, sep)
}
