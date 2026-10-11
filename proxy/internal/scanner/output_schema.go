package scanner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// An answer that is meant to be a JSON document can be checked against a
// schema the operator writes in the policy (output_rules.json_schema). The
// check is of a subset of JSON Schema, and the subset is closed: a schema
// that uses a keyword outside it does not compile, and the error names the
// keyword. A keyword that was read and then ignored would be a constraint
// the operator believes is enforced.
//
// The subset:
//
//	type                  one name or a list of names: object, array, string,
//	                      number, integer, boolean, null
//	enum                  a list of values
//	properties            name -> schema
//	required              a list of names
//	additionalProperties  true, false, or a schema
//	items                 one schema, for every element
//	minItems, maxItems
//	minimum, maximum      inclusive
//	minLength, maxLength  in characters (Unicode code points)
//	pattern               a regular expression, under the limits every
//	                      pattern in the policy is under (pattern_limits.go)
//
// title, description, $schema, $id, $comment, default and examples are
// accepted and have no effect, as in JSON Schema itself.
//
// Where this differs from the specification, on purpose:
//
//   - pattern is Go's regular expression syntax (RE2), not ECMA 262: no
//     look-around, no back-references. Such a pattern does not compile.
//   - A string longer than MaxSchemaPatternValue is not matched against a
//     pattern; it counts as a mismatch.
//   - Numbers are compared as float64, so a bound beyond 2^53 is compared
//     with that precision.
const (
	// MaxSchemaBytes bounds a schema, measured as JSON.
	MaxSchemaBytes = 64 << 10
	// MaxSchemaDepth bounds how deep schemas nest.
	MaxSchemaDepth = 16
	// MaxSchemaNodes bounds how many schemas one schema holds in all.
	MaxSchemaNodes = 1000
	// MaxSchemaEnum bounds the values of one enum.
	MaxSchemaEnum = 256
	// MaxSchemaPatternValue is the longest string a pattern is run on. One
	// search costs up to (program size x input length); see pattern_limits.go.
	MaxSchemaPatternValue = 16 << 10

	// SchemaFindingType is the type of the finding a mismatch is.
	SchemaFindingType = "output_schema"
	// SchemaMismatch is its category when the answer is JSON that the schema
	// does not accept, SchemaNotJSON when the answer is not JSON at all.
	SchemaMismatch = "schema_mismatch"
	SchemaNotJSON  = "not_json"
)

// schemaKeywords are the keywords of the subset, in the order an error
// message lists them.
var schemaKeywords = []string{
	"type", "enum", "properties", "required", "additionalProperties", "items",
	"minItems", "maxItems", "minimum", "maximum", "minLength", "maxLength", "pattern",
}

// schemaAnnotations are accepted and do nothing.
var schemaAnnotations = map[string]bool{
	"title": true, "description": true, "$schema": true, "$id": true, "$comment": true,
	"default": true, "examples": true,
}

var schemaTypes = map[string]bool{
	"object": true, "array": true, "string": true, "number": true, "integer": true, "boolean": true, "null": true,
}

// Schema is a compiled schema.
type Schema struct {
	root *schemaNode
}

type schemaNode struct {
	types map[string]bool // nil: any type

	enum    []interface{}
	hasEnum bool

	properties map[string]*schemaNode
	required   []string
	// additional is the schema for properties not named; noAdditional is
	// additionalProperties: false.
	additional   *schemaNode
	noAdditional bool

	items              *schemaNode
	minItems, maxItems *int

	minimum, maximum     *float64
	minLength, maxLength *int
	pattern              *regexp.Regexp
}

// CompileSchema compiles a schema as a policy holds it: the value a YAML or
// JSON decoder made of it. The error names the keyword and where it is.
func CompileSchema(doc interface{}) (*Schema, error) {
	raw, err := json.Marshal(normalizeYAML(doc))
	if err != nil {
		return nil, errors.New("the schema cannot be written as JSON: it holds a value JSON has no form for")
	}
	if len(raw) > MaxSchemaBytes {
		return nil, fmt.Errorf("the schema is %d bytes as JSON, the limit is %d", len(raw), MaxSchemaBytes)
	}
	// From here on the schema is what JSON says it is: every number a
	// float64, every object a map with string keys.
	var tree interface{}
	if err := json.Unmarshal(raw, &tree); err != nil {
		return nil, errors.New("the schema is not valid JSON")
	}
	c := &schemaCompiler{}
	root, err := c.node(tree, "", 1)
	if err != nil {
		return nil, err
	}
	return &Schema{root: root}, nil
}

// normalizeYAML turns the maps a YAML decoder can produce into maps JSON
// can write.
func normalizeYAML(v interface{}) interface{} {
	switch t := v.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(t))
		for k, val := range t {
			out[k] = normalizeYAML(val)
		}
		return out
	case map[interface{}]interface{}:
		out := make(map[string]interface{}, len(t))
		for k, val := range t {
			out[fmt.Sprint(k)] = normalizeYAML(val)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(t))
		for i, val := range t {
			out[i] = normalizeYAML(val)
		}
		return out
	}
	return v
}

type schemaCompiler struct {
	nodes int
}

func schemaAt(path string) string {
	if path == "" {
		return "the top of the schema"
	}
	return path
}

func (c *schemaCompiler) node(v interface{}, path string, depth int) (*schemaNode, error) {
	obj, ok := v.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("%s: a schema must be a mapping", schemaAt(path))
	}
	if depth > MaxSchemaDepth {
		return nil, fmt.Errorf("%s: schemas nest more than %d deep", schemaAt(path), MaxSchemaDepth)
	}
	if c.nodes++; c.nodes > MaxSchemaNodes {
		return nil, fmt.Errorf("the schema holds more than %d schemas", MaxSchemaNodes)
	}
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	n := &schemaNode{}
	for _, key := range keys {
		val := obj[key]
		where := path + "/" + key
		switch key {
		case "type":
			names, ok := stringList(val)
			if s, isString := val.(string); isString {
				names, ok = []string{s}, true
			}
			if !ok || len(names) == 0 {
				return nil, fmt.Errorf("%s: type must be a type name or a list of them", where)
			}
			n.types = map[string]bool{}
			for _, name := range names {
				if !schemaTypes[name] {
					return nil, fmt.Errorf("%s: unknown type %q (object, array, string, number, integer, boolean or null)", where, PatternPrefix(name))
				}
				n.types[name] = true
			}
		case "enum":
			list, ok := val.([]interface{})
			if !ok || len(list) == 0 {
				return nil, fmt.Errorf("%s: enum must be a list with at least one value", where)
			}
			if len(list) > MaxSchemaEnum {
				return nil, fmt.Errorf("%s: enum has %d values, the limit is %d", where, len(list), MaxSchemaEnum)
			}
			n.enum, n.hasEnum = list, true
		case "properties":
			props, ok := val.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("%s: properties must be a mapping of names to schemas", where)
			}
			n.properties = make(map[string]*schemaNode, len(props))
			names := make([]string, 0, len(props))
			for name := range props {
				names = append(names, name)
			}
			sort.Strings(names)
			for _, name := range names {
				child, err := c.node(props[name], where+"/"+name, depth+1)
				if err != nil {
					return nil, err
				}
				n.properties[name] = child
			}
		case "required":
			names, ok := stringList(val)
			if !ok {
				return nil, fmt.Errorf("%s: required must be a list of property names", where)
			}
			n.required = names
		case "additionalProperties":
			switch t := val.(type) {
			case bool:
				n.noAdditional = !t
			case map[string]interface{}:
				child, err := c.node(t, where, depth+1)
				if err != nil {
					return nil, err
				}
				n.additional = child
			default:
				return nil, fmt.Errorf("%s: additionalProperties must be true, false or a schema", where)
			}
		case "items":
			if _, isList := val.([]interface{}); isList {
				return nil, fmt.Errorf("%s: items as a list (one schema per position) is not supported; give one schema for every element", where)
			}
			child, err := c.node(val, where, depth+1)
			if err != nil {
				return nil, err
			}
			n.items = child
		case "minItems", "maxItems", "minLength", "maxLength":
			count, ok := wholeNumber(val)
			if !ok {
				return nil, fmt.Errorf("%s: %s must be a whole number, zero or more", where, key)
			}
			switch key {
			case "minItems":
				n.minItems = &count
			case "maxItems":
				n.maxItems = &count
			case "minLength":
				n.minLength = &count
			case "maxLength":
				n.maxLength = &count
			}
		case "minimum", "maximum":
			num, ok := val.(float64)
			if !ok || math.IsNaN(num) || math.IsInf(num, 0) {
				return nil, fmt.Errorf("%s: %s must be a number", where, key)
			}
			if key == "minimum" {
				n.minimum = &num
			} else {
				n.maximum = &num
			}
		case "pattern":
			pat, ok := val.(string)
			if !ok {
				return nil, fmt.Errorf("%s: pattern must be a string", where)
			}
			re, err := CompileMatcherPattern(pat)
			if err != nil {
				return nil, fmt.Errorf("%s: %v", where, err)
			}
			n.pattern = re
		default:
			if schemaAnnotations[key] {
				continue
			}
			return nil, fmt.Errorf("%s: the keyword %q is not supported, and a schema that uses it is not loaded rather than checked without it (supported: %s)",
				schemaAt(path), PatternPrefix(key), strings.Join(schemaKeywords, ", "))
		}
	}
	return n, nil
}

func stringList(v interface{}) ([]string, bool) {
	list, ok := v.([]interface{})
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(list))
	for _, e := range list {
		s, ok := e.(string)
		if !ok {
			return nil, false
		}
		out = append(out, s)
	}
	return out, true
}

func wholeNumber(v interface{}) (int, bool) {
	f, ok := v.(float64)
	if !ok || f < 0 || f != math.Trunc(f) || f > 1<<31 {
		return 0, false
	}
	return int(f), true
}

// SchemaResult is the outcome of checking one document.
type SchemaResult struct {
	// NotJSON is set when the text is not one JSON document.
	NotJSON bool
	// Keyword is the keyword of the first constraint that failed, and
	// SchemaPath where that constraint is in the schema
	// ("/properties/age/minimum"). Both come from the schema: neither holds
	// anything of the document.
	Keyword    string
	SchemaPath string
	// Count is how many constraints failed, up to maxSchemaFailures.
	Count int
}

// OK reports whether the document was accepted.
func (r SchemaResult) OK() bool { return !r.NotJSON && r.Count == 0 }

// maxSchemaFailures is where counting the failures of one document stops.
const maxSchemaFailures = 100

// Check judges a document. The error is the context's: the check was cut
// short and says nothing about the document.
func (s *Schema) Check(ctx context.Context, doc []byte) (SchemaResult, error) {
	dec := json.NewDecoder(bytes.NewReader(doc))
	dec.UseNumber()
	var value interface{}
	if err := dec.Decode(&value); err != nil {
		return SchemaResult{NotJSON: true, Count: 1}, nil
	}
	// One document and nothing after it.
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return SchemaResult{NotJSON: true, Count: 1}, nil
	}
	c := &schemaCheck{ctx: ctx}
	c.value(s.root, value, "")
	if c.err != nil {
		return SchemaResult{}, c.err
	}
	return c.res, nil
}

type schemaCheck struct {
	ctx context.Context
	res SchemaResult
	err error
}

func (c *schemaCheck) fail(path, keyword string) {
	if c.res.Count == 0 {
		c.res.Keyword, c.res.SchemaPath = keyword, path+"/"+keyword
	}
	c.res.Count++
}

func (c *schemaCheck) done() bool {
	return c.err != nil || c.res.Count >= maxSchemaFailures
}

func jsonType(v interface{}) string {
	switch t := v.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case string:
		return "string"
	case json.Number:
		if isInteger(t) {
			return "integer"
		}
		return "number"
	case []interface{}:
		return "array"
	case map[string]interface{}:
		return "object"
	}
	return ""
}

// isInteger reports whether a JSON number has no fractional part: 3, 3.0
// and 3e2 are integers, as the specification has it.
func isInteger(n json.Number) bool {
	s := n.String()
	if !strings.ContainsAny(s, ".eE") {
		return true
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		// Too large for a float64: a number of that size has no fraction.
		return math.IsInf(f, 0)
	}
	return f == math.Trunc(f)
}

// schemaNumber is a JSON number as a float64. One out of float64's range
// comes back as the infinity of its sign, which compares as it should.
func schemaNumber(n json.Number) float64 {
	f, _ := strconv.ParseFloat(n.String(), 64)
	return f
}

func (c *schemaCheck) value(n *schemaNode, v interface{}, path string) {
	if c.done() {
		return
	}
	if n.types != nil {
		t := jsonType(v)
		if !n.types[t] && !(t == "integer" && n.types["number"]) {
			c.fail(path, "type")
			return // the other keywords assume the type
		}
	}
	if n.hasEnum {
		found := false
		for _, e := range n.enum {
			if jsonEqual(e, v) {
				found = true
				break
			}
		}
		if !found {
			c.fail(path, "enum")
		}
	}
	switch t := v.(type) {
	case string:
		if n.minLength != nil || n.maxLength != nil {
			length := utf8.RuneCountInString(t)
			if n.minLength != nil && length < *n.minLength {
				c.fail(path, "minLength")
			}
			if n.maxLength != nil && length > *n.maxLength {
				c.fail(path, "maxLength")
			}
		}
		if n.pattern != nil {
			if err := c.ctx.Err(); err != nil {
				c.err = err
				return
			}
			if len(t) > MaxSchemaPatternValue || !n.pattern.MatchString(t) {
				c.fail(path, "pattern")
			}
		}
	case json.Number:
		if n.minimum != nil && schemaNumber(t) < *n.minimum {
			c.fail(path, "minimum")
		}
		if n.maximum != nil && schemaNumber(t) > *n.maximum {
			c.fail(path, "maximum")
		}
	case []interface{}:
		if n.minItems != nil && len(t) < *n.minItems {
			c.fail(path, "minItems")
		}
		if n.maxItems != nil && len(t) > *n.maxItems {
			c.fail(path, "maxItems")
		}
		if n.items != nil {
			for _, e := range t {
				c.value(n.items, e, path+"/items")
				if c.done() {
					return
				}
			}
		}
	case map[string]interface{}:
		for _, name := range n.required {
			if _, ok := t[name]; !ok {
				c.fail(path, "required")
			}
		}
		if n.properties == nil && n.additional == nil && !n.noAdditional {
			return
		}
		// In the order of the names, so that the first failure reported is
		// the same on every run.
		names := make([]string, 0, len(t))
		for name := range t {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			if child, ok := n.properties[name]; ok {
				c.value(child, t[name], path+"/properties/"+name)
			} else if n.noAdditional {
				c.fail(path, "additionalProperties")
			} else if n.additional != nil {
				c.value(n.additional, t[name], path+"/additionalProperties")
			}
			if c.done() {
				return
			}
		}
	}
}

// jsonEqual compares a value of the schema (numbers are float64) with a
// value of the document (numbers are json.Number).
func jsonEqual(schema, doc interface{}) bool {
	switch s := schema.(type) {
	case nil:
		return doc == nil
	case bool:
		d, ok := doc.(bool)
		return ok && d == s
	case string:
		d, ok := doc.(string)
		return ok && d == s
	case float64:
		d, ok := doc.(json.Number)
		return ok && schemaNumber(d) == s
	case []interface{}:
		d, ok := doc.([]interface{})
		if !ok || len(d) != len(s) {
			return false
		}
		for i := range s {
			if !jsonEqual(s[i], d[i]) {
				return false
			}
		}
		return true
	case map[string]interface{}:
		d, ok := doc.(map[string]interface{})
		if !ok || len(d) != len(s) {
			return false
		}
		for k, sv := range s {
			dv, ok := d[k]
			if !ok || !jsonEqual(sv, dv) {
				return false
			}
		}
		return true
	}
	return false
}

// SchemaFinding is the finding a document that was not accepted becomes. It
// has no match and no position: it is a verdict on the answer as a whole,
// and it says which constraint failed, not what the answer held.
func SchemaFinding(r SchemaResult) Finding {
	f := Finding{
		Type:           SchemaFindingType,
		Category:       SchemaMismatch,
		Severity:       "medium",
		Confidence:     1.0,
		ScannerVersion: ScannerVersions[SchemaFindingType],
	}
	if r.NotJSON {
		f.Category = SchemaNotJSON
		return f
	}
	f.Metadata = map[string]string{
		"keyword":     r.Keyword,
		"schema_path": r.SchemaPath,
		"count":       strconv.Itoa(r.Count),
	}
	return f
}
