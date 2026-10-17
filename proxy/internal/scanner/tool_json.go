package scanner

import (
	"context"
	"strconv"

	"github.com/yatuk/tamga/internal/extract"
	"github.com/yatuk/tamga/internal/scanner/normalize"
)

// A tool result is very often JSON: MCP tools answer with a text block that
// holds the tool's JSON output, and function results are JSON strings. The
// scanners read such a text as written, which is the JSON with its escapes,
// and that misses what the escapes hide (\u0069gnore), what a document
// encoded twice hides behind a second layer of them, and an instruction
// whose halves stand in two fields of one object with the quotes, colon and
// key of the second between them.
//
// So a tool result that is JSON is read a second time, decoded, in the
// units normalize.JSONUnits gives: the string values of one object, the
// strings of one array, a document that is one string. An instruction is
// reported when one unit holds it, so that it is found where one party
// wrote it and never across two records. The strings in which an escape
// can hide a value (normalize.JSONField.Hidden: a digit written as an
// escape, a line break right before a number) are also given to the other
// scanners; what they find is put back on the escapes it was read from, so
// REDACT masks the right bytes.
//
// A scanner call costs about the same for a short text as for a page, and a
// search result can hold hundreds of records. So the units are first read
// together, as one text with a blank line between them, and only when that
// reading finds an instruction the text as written did not is each unit
// read alone, to tell one unit's instruction from words that only meet
// across two. The strings for the other scanners are read together the
// same way: an identifier is matched within a line or two and does not
// reach over a blank line into the next string.
//
// Only tool results are read this way (extract.RoleTool): text from outside
// the conversation, where the encoding is the tool's and the words are
// someone else's. A user who pastes JSON is the author of the instruction
// in it either way.

// metaViaJSON is the detected_via of a finding made in a decoded reading of
// a tool result that is JSON.
const metaViaJSON = "json"

// jsonUnitSeparator stands between units, and between strings, read
// together.
const jsonUnitSeparator = "\n\n"

// toolJSON is what scanToolJSON scans in one segment.
type toolJSON struct {
	seg   int
	units []normalize.JSONUnit
}

// toolJSONSegments returns the tool-result segments that are JSON documents,
// with their units.
func toolJSONSegments(segs []extract.Segment) []toolJSON {
	var out []toolJSON
	for i, s := range segs {
		if s.Role != extract.RoleTool || !normalize.LooksLikeJSON(s.Text) {
			continue
		}
		if units := normalize.JSONUnits(s.Text); len(units) > 0 {
			out = append(out, toolJSON{seg: i, units: units})
		}
	}
	return out
}

// scanToolJSON scans the decoded units of the tool results that are JSON
// and returns, per segment, the findings to add to what the text as written
// gave (perSeg). idErr is the first error identify returned; scanErr is
// what the scanners of instructions report, as a degradation.
//
// An instruction found in a unit is reported when its category was not
// found in the segment already, as scanAcrossSegments does it. identify
// scans one text with every scanner and settles its findings as ScanRaw
// does; it is the local registry or the remote scanner.
func (r *Registry) scanToolJSON(ctx context.Context, segs []extract.Segment, perSeg [][]Finding,
	identify func(context.Context, []byte) ([]Finding, error)) (extra [][]Finding, idErr, scanErr error) {
	docs := toolJSONSegments(segs)
	if len(docs) == 0 {
		return nil, nil, nil
	}
	r.mu.RLock()
	var instruction []Scanner
	for _, s := range r.scanners {
		if crossSegmentScanners[s.Name()] {
			instruction = append(instruction, s)
		}
	}
	r.mu.RUnlock()

	extra = make([][]Finding, len(segs))
	var deg degradation
	scan := func(text string) []Finding {
		var all []Finding
		for _, s := range instruction {
			// Answered from the scan cache when the text was read before:
			// an agent sends the same tool results with every turn.
			found, err := cachedScan(ctx, s, []byte(text))
			if err != nil {
				deg.note(s.Name(), DegradedError, err)
			}
			all = append(all, found...)
		}
		return all
	}
	for _, doc := range docs {
		if err := ctx.Err(); err != nil {
			return extra, idErr, err
		}
		seg := segs[doc.seg]
		known := map[string]bool{}
		for _, f := range perSeg[doc.seg] {
			known[f.Type+"/"+f.Category] = true
		}

		var units []normalize.JSONUnit
		var fields []normalize.JSONField
		for _, u := range doc.units {
			if unitNeedsReading(u) {
				units = append(units, u)
			}
			for _, f := range u.Fields {
				if f.Hidden {
					fields = append(fields, f)
				}
			}
		}

		if len(instruction) > 0 && len(units) > 0 && hasNew(scan(normalize.JoinUnits(units, jsonUnitSeparator).Text), known) {
			for u, unit := range units {
				for _, f := range scan(unit.View.Text) {
					key := f.Type + "/" + f.Category
					if known[key] {
						continue
					}
					known[key] = true
					f.Metadata = copyMeta(f.Metadata)
					f.Metadata[metaDetectedVia] = metaViaJSON
					f.Metadata[metaPosition] = positionSource
					f.Metadata["json_unit"] = strconv.Itoa(u)
					// The finding is about the unit; its span is the
					// unit's, from its first string to its last.
					f.StartPos, f.EndPos = unitSpan(unit)
					placeFinding(&f, doc.seg, seg)
					extra[doc.seg] = append(extra[doc.seg], f)
				}
			}
		}

		if identify == nil || len(fields) == 0 {
			continue
		}
		joined := normalize.JoinFields(fields, jsonUnitSeparator)
		found, err := identify(ctx, []byte(joined.Text))
		if err != nil && idErr == nil {
			idErr = err
		}
		for _, f := range found {
			if !redactable(f) {
				continue
			}
			f.Metadata = copyMeta(f.Metadata)
			f.Metadata[metaDetectedVia] = metaViaJSON
			delete(f.Metadata, metaPosition)
			if from, to, ok := joined.Span(f.StartPos, f.EndPos); ok && f.Placed() {
				f.StartPos, f.EndPos = from, to
				f.Metadata[metaPosition] = positionSource
			} else {
				f.StartPos, f.EndPos = 0, 0
			}
			placeFinding(&f, doc.seg, seg)
			extra[doc.seg] = append(extra[doc.seg], f)
		}
	}
	return extra, idErr, deg.result()
}

// hasNew reports whether a finding is of a type and category not in known.
func hasNew(found []Finding, known map[string]bool) bool {
	for _, f := range found {
		if !known[f.Type+"/"+f.Category] {
			return true
		}
	}
	return false
}

// unitSpan is the span of the text as written from a unit's first string to
// its last.
func unitSpan(u normalize.JSONUnit) (int, int) {
	from, to := u.Fields[0].From, u.Fields[0].To
	for _, f := range u.Fields[1:] {
		from, to = min(from, f.From), max(to, f.To)
	}
	return from, to
}

// unitNeedsReading reports whether a unit can hold what the text as
// written does not show: more than one string, or a string the text as
// written shows only through escapes or a second layer of encoding. A
// single string written without an escape was read where it stands.
func unitNeedsReading(u normalize.JSONUnit) bool {
	if len(u.Fields) > 1 {
		return true
	}
	return u.Fields[0].Escaped
}
