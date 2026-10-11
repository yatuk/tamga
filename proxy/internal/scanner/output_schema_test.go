package scanner

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// schemaFromYAML compiles a schema written the way a policy holds it.
func schemaFromYAML(t testing.TB, text string) (*Schema, error) {
	t.Helper()
	var doc map[string]interface{}
	if err := yaml.Unmarshal([]byte(text), &doc); err != nil {
		t.Fatalf("test schema is not YAML: %v", err)
	}
	return CompileSchema(doc)
}

const ticketSchema = `
type: object
required: [id, status, tags]
additionalProperties: false
properties:
  id: {type: integer, minimum: 1, maximum: 999999}
  status: {enum: [open, closed, 3, null]}
  title: {type: string, minLength: 3, maxLength: 20, pattern: "^[A-Z]"}
  score: {type: [number, "null"], minimum: 0, maximum: 1}
  tags:
    type: array
    minItems: 1
    maxItems: 3
    items: {type: string, pattern: "^[a-z-]+$"}
  owner:
    type: object
    required: [name]
    properties:
      name: {type: string}
    additionalProperties: {type: boolean}
`

func TestSchema_Check(t *testing.T) {
	s, err := schemaFromYAML(t, ticketSchema)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name, doc string
		// keyword is the first failure expected; "" means accepted.
		keyword, path string
		notJSON       bool
	}{
		{name: "accepted", doc: `{"id": 7, "status": "open", "tags": ["a-b"], "title": "Hello", "score": 0.5, "owner": {"name": "x", "active": true}}`},
		{name: "accepted, integer written with a fraction of zero", doc: `{"id": 7.0, "status": 3, "tags": ["a"]}`},
		{name: "accepted, null in enum and a null score", doc: `{"id": 1, "status": null, "tags": ["a"], "score": null}`},
		{name: "accepted, white space around", doc: "\n  {\"id\": 1, \"status\": \"closed\", \"tags\": [\"a\"]}\n"},
		{name: "wrong root type", doc: `[1, 2]`, keyword: "type", path: "/type"},
		{name: "missing required", doc: `{"id": 1, "status": "open"}`, keyword: "required", path: "/required"},
		{name: "additional property", doc: `{"id": 1, "status": "open", "tags": ["a"], "secret": "x"}`, keyword: "additionalProperties", path: "/additionalProperties"},
		{name: "not an integer", doc: `{"id": 1.5, "status": "open", "tags": ["a"]}`, keyword: "type", path: "/properties/id/type"},
		{name: "a string where a number is wanted", doc: `{"id": "1", "status": "open", "tags": ["a"]}`, keyword: "type", path: "/properties/id/type"},
		{name: "below minimum", doc: `{"id": 0, "status": "open", "tags": ["a"]}`, keyword: "minimum", path: "/properties/id/minimum"},
		{name: "above maximum", doc: `{"id": 1000000, "status": "open", "tags": ["a"]}`, keyword: "maximum", path: "/properties/id/maximum"},
		{name: "a number too large for float64", doc: `{"id": 1e999, "status": "open", "tags": ["a"]}`, keyword: "maximum", path: "/properties/id/maximum"},
		{name: "not in enum", doc: `{"id": 1, "status": "pending", "tags": ["a"]}`, keyword: "enum", path: "/properties/status/enum"},
		{name: "enum number as a string", doc: `{"id": 1, "status": "3", "tags": ["a"]}`, keyword: "enum", path: "/properties/status/enum"},
		{name: "too short", doc: `{"id": 1, "status": "open", "tags": ["a"], "title": "Hi"}`, keyword: "minLength", path: "/properties/title/minLength"},
		{name: "too long", doc: `{"id": 1, "status": "open", "tags": ["a"], "title": "A title that goes on and on"}`, keyword: "maxLength", path: "/properties/title/maxLength"},
		{name: "length is counted in characters", doc: `{"id": 1, "status": "open", "tags": ["a"], "title": "Aşşşşşşşşşşşşşşşşşşş"}`},
		{name: "pattern", doc: `{"id": 1, "status": "open", "tags": ["a"], "title": "lower"}`, keyword: "pattern", path: "/properties/title/pattern"},
		{name: "too few items", doc: `{"id": 1, "status": "open", "tags": []}`, keyword: "minItems", path: "/properties/tags/minItems"},
		{name: "too many items", doc: `{"id": 1, "status": "open", "tags": ["a","b","c","d"]}`, keyword: "maxItems", path: "/properties/tags/maxItems"},
		{name: "item of the wrong type", doc: `{"id": 1, "status": "open", "tags": ["a", 2]}`, keyword: "type", path: "/properties/tags/items/type"},
		{name: "item fails pattern", doc: `{"id": 1, "status": "open", "tags": ["a", "B"]}`, keyword: "pattern", path: "/properties/tags/items/pattern"},
		{name: "nested required", doc: `{"id": 1, "status": "open", "tags": ["a"], "owner": {}}`, keyword: "required", path: "/properties/owner/required"},
		{name: "additional property against its schema", doc: `{"id": 1, "status": "open", "tags": ["a"], "owner": {"name": "x", "active": "yes"}}`, keyword: "type", path: "/properties/owner/additionalProperties/type"},
		{name: "prose", doc: `Here is the ticket you asked for.`, notJSON: true},
		{name: "fenced", doc: "```json\n{\"id\": 1, \"status\": \"open\", \"tags\": [\"a\"]}\n```", notJSON: true},
		{name: "cut off", doc: `{"id": 1, "status": "open", "tags": ["a"`, notJSON: true},
		{name: "two documents", doc: `{"id": 1, "status": "open", "tags": ["a"]} {"id": 2}`, notJSON: true},
		{name: "text after the document", doc: `{"id": 1, "status": "open", "tags": ["a"]} Hope this helps!`, notJSON: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := s.Check(context.Background(), []byte(tt.doc))
			if err != nil {
				t.Fatal(err)
			}
			if res.NotJSON != tt.notJSON {
				t.Fatalf("NotJSON = %v, want %v", res.NotJSON, tt.notJSON)
			}
			if tt.notJSON {
				if res.OK() {
					t.Fatal("a text that is not JSON was accepted")
				}
				return
			}
			if tt.keyword == "" {
				if !res.OK() {
					t.Fatalf("refused: %+v", res)
				}
				return
			}
			if res.Keyword != tt.keyword || res.SchemaPath != tt.path || res.Count < 1 {
				t.Fatalf("got %+v, want keyword %q at %q", res, tt.keyword, tt.path)
			}
		})
	}
}

// A keyword outside the subset is refused by name. Reading it and checking
// without it would let through what the operator wrote the schema to stop.
func TestCompileSchema_RefusesWhatItDoesNotCheck(t *testing.T) {
	for _, keyword := range []string{
		"oneOf", "anyOf", "allOf", "not", "$ref", "$defs", "definitions", "const", "format",
		"patternProperties", "propertyNames", "dependentRequired", "if", "then", "else",
		"exclusiveMinimum", "exclusiveMaximum", "multipleOf", "uniqueItems", "contains",
		"prefixItems", "minProperties", "maxProperties", "nullable", "unevaluatedProperties",
	} {
		_, err := CompileSchema(map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"field": map[string]interface{}{"type": "string", keyword: true},
			},
		})
		if err == nil {
			t.Errorf("%s: the schema compiled", keyword)
			continue
		}
		if !strings.Contains(err.Error(), `"`+keyword+`"`) || !strings.Contains(err.Error(), "/properties/field") {
			t.Errorf("%s: the error does not name the keyword and where it is: %v", keyword, err)
		}
	}
}

func TestCompileSchema_Refusals(t *testing.T) {
	deep := map[string]interface{}{"type": "string"}
	for i := 0; i < MaxSchemaDepth+1; i++ {
		deep = map[string]interface{}{"type": "array", "items": deep}
	}
	many := map[string]interface{}{}
	for i := 0; i < MaxSchemaNodes+1; i++ {
		many[fmt.Sprintf("p%d", i)] = map[string]interface{}{}
	}
	var bigEnum []interface{}
	for i := 0; i < MaxSchemaEnum+1; i++ {
		bigEnum = append(bigEnum, i)
	}
	tests := map[string]struct {
		schema interface{}
		want   string
	}{
		"not a mapping":                {[]interface{}{1}, "must be a mapping"},
		"unknown type":                 {map[string]interface{}{"type": "int"}, "unknown type"},
		"type of the wrong form":       {map[string]interface{}{"type": 3}, "type must be"},
		"empty enum":                   {map[string]interface{}{"enum": []interface{}{}}, "at least one value"},
		"enum too large":               {map[string]interface{}{"enum": bigEnum}, "the limit is"},
		"required not names":           {map[string]interface{}{"required": []interface{}{1}}, "list of property names"},
		"negative length":              {map[string]interface{}{"minLength": -1}, "whole number"},
		"fractional count":             {map[string]interface{}{"maxItems": 1.5}, "whole number"},
		"minimum not a number":         {map[string]interface{}{"minimum": "1"}, "must be a number"},
		"tuple items":                  {map[string]interface{}{"items": []interface{}{map[string]interface{}{}}}, "not supported"},
		"additional of the wrong form": {map[string]interface{}{"additionalProperties": "no"}, "true, false or a schema"},
		"pattern does not parse":       {map[string]interface{}{"pattern": "([a-z"}, "invalid regular expression"},
		"pattern with look-ahead":      {map[string]interface{}{"pattern": "^(?=.*x)"}, "invalid regular expression"},
		"pattern too large":            {map[string]interface{}{"pattern": "[a-z]{400}[0-9]{400}"}, "instructions"},
		"too deep":                     {deep, "deep"},
		"too many schemas":             {map[string]interface{}{"properties": many}, "more than"},
		"too large":                    {map[string]interface{}{"description": strings.Repeat("x", MaxSchemaBytes)}, "bytes as JSON"},
	}
	for name, tt := range tests {
		_, err := CompileSchema(tt.schema)
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: err = %v, want one that says %q", name, err, tt.want)
		}
	}
}

func TestCompileSchema_AnnotationsAreAccepted(t *testing.T) {
	s, err := schemaFromYAML(t, `
$schema: "https://json-schema.org/draft/2020-12/schema"
$id: "https://example.invalid/ticket"
title: Ticket
description: A ticket.
$comment: nothing to see
default: {}
examples: [{}]
type: object
`)
	if err != nil {
		t.Fatal(err)
	}
	if res, _ := s.Check(context.Background(), []byte(`{}`)); !res.OK() {
		t.Fatalf("refused: %+v", res)
	}
}

// What a mismatch says comes from the schema: no name and no value of the
// document is in the result or in the finding made of it.
func TestSchemaFinding_HoldsNothingOfTheDocument(t *testing.T) {
	s, err := schemaFromYAML(t, ticketSchema)
	if err != nil {
		t.Fatal(err)
	}
	doc := `{"id": 1, "status": "open", "tags": ["a"], "tckn_10000000146": "ayse@example.com", "title": "gizli-deger"}`
	res, err := s.Check(context.Background(), []byte(doc))
	if err != nil || res.OK() {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
	f := SchemaFinding(res)
	raw, _ := json.Marshal(f)
	for _, leak := range []string{"10000000146", "ayse", "gizli", "tckn_"} {
		if strings.Contains(string(raw), leak) {
			t.Errorf("the finding holds %q of the document: %s", leak, raw)
		}
	}
	if f.Type != "output_schema" || f.Category != "schema_mismatch" || f.Match != "" || f.Placed() {
		t.Errorf("finding = %+v", f)
	}
	if f.Metadata["count"] != "2" || f.Metadata["keyword"] != "additionalProperties" || f.Metadata["schema_path"] != "/additionalProperties" {
		t.Errorf("metadata = %v", f.Metadata)
	}
	if MaskableType(f.Type) {
		t.Error("a schema mismatch is listed as maskable")
	}
	if nj := SchemaFinding(SchemaResult{NotJSON: true, Count: 1}); nj.Category != "not_json" || nj.Metadata != nil {
		t.Errorf("not-JSON finding = %+v", nj)
	}
}

func TestSchema_PatternIsNotRunOnAVeryLongString(t *testing.T) {
	s, err := CompileSchema(map[string]interface{}{"type": "string", "pattern": "^a+$"})
	if err != nil {
		t.Fatal(err)
	}
	ok, _ := json.Marshal(strings.Repeat("a", MaxSchemaPatternValue))
	if res, _ := s.Check(context.Background(), ok); !res.OK() {
		t.Fatalf("a string at the limit was refused: %+v", res)
	}
	long, _ := json.Marshal(strings.Repeat("a", MaxSchemaPatternValue+1))
	if res, _ := s.Check(context.Background(), long); res.OK() || res.Keyword != "pattern" {
		t.Fatalf("a string past the limit: %+v", res)
	}
}

func TestSchema_CheckStopsWithItsContext(t *testing.T) {
	s, err := CompileSchema(map[string]interface{}{"type": "array", "items": map[string]interface{}{"pattern": "^a"}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Check(ctx, []byte(`["a","b"]`)); err == nil {
		t.Fatal("a check under a cancelled context returned no error")
	}
}

func TestSchema_FailuresAreCountedUpToALimit(t *testing.T) {
	s, err := CompileSchema(map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}})
	if err != nil {
		t.Fatal(err)
	}
	doc := "[" + strings.TrimSuffix(strings.Repeat("1,", 5000), ",") + "]"
	res, err := s.Check(context.Background(), []byte(doc))
	if err != nil || res.Count != maxSchemaFailures {
		t.Fatalf("count = %d (err %v), want %d", res.Count, err, maxSchemaFailures)
	}
}

func BenchmarkSchemaCheck(b *testing.B) {
	s, err := schemaFromYAML(b, ticketSchema)
	if err != nil {
		b.Fatal(err)
	}
	small := []byte(`{"id": 7, "status": "open", "tags": ["a-b", "c"], "title": "Hello", "score": 0.5, "owner": {"name": "x", "active": true}}`)
	list, err := CompileSchema(map[string]interface{}{"type": "array", "items": map[string]interface{}{
		"type": "object", "required": []interface{}{"id", "name"},
		"properties": map[string]interface{}{"id": map[string]interface{}{"type": "integer"}, "name": map[string]interface{}{"type": "string", "pattern": "^[a-z ]+$"}},
	}})
	if err != nil {
		b.Fatal(err)
	}
	large := []byte("[" + strings.TrimSuffix(strings.Repeat(`{"id": 12345, "name": "a name of ordinary length"},`, 1300), ",") + "]")
	b.Run("small_answer", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, _ = s.Check(context.Background(), small)
		}
	})
	b.Run("list_64k", func(b *testing.B) {
		b.SetBytes(int64(len(large)))
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, _ = list.Check(context.Background(), large)
		}
	})
}
