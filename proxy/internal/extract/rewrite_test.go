package extract

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mustExtract(t *testing.T, provider string, body []byte) *Result {
	t.Helper()
	res, ok := Extract(provider, body)
	if !ok {
		t.Fatalf("not recognised: %s", body)
	}
	return res
}

func indexOfPath(t *testing.T, res *Result, path string) int {
	t.Helper()
	for i, s := range res.Segments {
		if s.Path == path {
			return i
		}
	}
	t.Fatalf("no segment at %s", path)
	return -1
}

// redact replaces the first occurrence of what in the segment at path.
func redact(t *testing.T, res *Result, path, what, with string) Edit {
	t.Helper()
	i := indexOfPath(t, res, path)
	at := strings.Index(res.Segments[i].Text, what)
	if at < 0 {
		t.Fatalf("%q not in segment %s (%q)", what, path, res.Segments[i].Text)
	}
	return Edit{Seg: i, From: at, To: at + len(what), Replacement: with}
}

func TestRewrite_PlainSegment(t *testing.T) {
	// The value has an escaped newline, an escaped quote and a \u escape, so
	// offsets in the text and in the body differ.
	body := []byte(strings.ReplaceAll(`{"model":"gpt-5",  "messages":[{"role":"user","content":"Ad: AyU+015fe \"Y\"\nTC: 10000000146 son"}],"temperature":0.20}`, "U+", `\u`))
	res := mustExtract(t, "openai", body)

	out, err := Rewrite(body, res.Segments, []Edit{redact(t, res, "messages[0].content", "10000000146", "[tc_kimlik_REDACTED]")})
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(out) {
		t.Fatalf("output is not JSON: %s", out)
	}
	got := mustExtract(t, "openai", out).Segments[0].Text
	if want := "Ad: Ayşe \"Y\"\nTC: [tc_kimlik_REDACTED] son"; got != want {
		t.Fatalf("text after rewrite = %q, want %q", got, want)
	}
	// Everything outside the one string token is byte-identical: the odd
	// spacing and "0.20" survive.
	if !bytes.HasPrefix(out, []byte(`{"model":"gpt-5",  "messages":[{"role":"user","content":`)) ||
		!bytes.HasSuffix(out, []byte(`}],"temperature":0.20}`)) {
		t.Fatalf("bytes outside the token changed: %s", out)
	}
}

func TestRewrite_ReplacementNeedingEscapes(t *testing.T) {
	body := []byte(`{"messages":[{"role":"user","content":"mail a@b.co now"}]}`)
	res := mustExtract(t, "openai", body)
	out, err := Rewrite(body, res.Segments, []Edit{redact(t, res, "messages[0].content", "a@b.co", `he said "hi" \ <b>`+"\n")})
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(out) {
		t.Fatalf("output is not JSON: %s", out)
	}
	if got, want := mustExtract(t, "openai", out).Segments[0].Text, "mail he said \"hi\" \\ <b>\n now"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if bytes.Contains(out, []byte("u003c")) {
		t.Fatalf("\"<\" was HTML-escaped: %s", out)
	}
}

func TestRewrite_NestedArguments(t *testing.T) {
	body := []byte(`{"messages":[{"role":"assistant","tool_calls":[{"id":"c1","type":"function","function":{"name":"send","arguments":"{\"to\":\"ayse@example.com\",\"note\":\"TC 10000000146\",\"n\":3}"}}]}]}`)
	res := mustExtract(t, "openai", body)
	base := "messages[0].tool_calls[0].function.arguments#"
	out, err := Rewrite(body, res.Segments, []Edit{
		redact(t, res, base+"to", "ayse@example.com", "[email_REDACTED]"),
		redact(t, res, base+"note", "10000000146", "[tc_kimlik_REDACTED]"),
	})
	if err != nil {
		t.Fatal(err)
	}
	after := mustExtract(t, "openai", out)
	if got := after.Segments[indexOfPath(t, after, base+"to")].Text; got != "[email_REDACTED]" {
		t.Fatalf("to = %q", got)
	}
	if got := after.Segments[indexOfPath(t, after, base+"note")].Text; got != "TC [tc_kimlik_REDACTED]" {
		t.Fatalf("note = %q", got)
	}
	// The arguments are still a JSON document, and its untouched member survives.
	var doc struct {
		Messages []struct {
			ToolCalls []struct {
				Function struct{ Arguments string }
			} `json:"tool_calls"`
		}
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatal(err)
	}
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(doc.Messages[0].ToolCalls[0].Function.Arguments), &args); err != nil {
		t.Fatalf("arguments are no longer JSON: %v", err)
	}
	if args["n"] != float64(3) {
		t.Fatalf("untouched member changed: %v", args)
	}
}

func TestRewrite_Base64Attachment(t *testing.T) {
	body := []byte(`{"input":[{"role":"user","content":[{"type":"input_file","filename":"a.txt","file_data":"data:text/plain;base64,VEMgMTAwMDAwMDAxNDYgc29u"}]}]}`)
	res := mustExtract(t, "openai", body)
	path := "input[0].content[0].file_data"
	out, err := Rewrite(body, res.Segments, []Edit{redact(t, res, path, "10000000146", "[x]")})
	if err != nil {
		t.Fatal(err)
	}
	after := mustExtract(t, "openai", out)
	s := after.Segments[indexOfPath(t, after, path)]
	if s.Text != "TC [x] son" || s.Encoding != "base64" {
		t.Fatalf("got %q (%s)", s.Text, s.Encoding)
	}
	if !bytes.Contains(out, []byte(`"data:text/plain;base64,`)) {
		t.Fatalf("data URI prefix lost: %s", out)
	}
}

func TestRewrite_OverlappingEditsMerge(t *testing.T) {
	body := []byte(`{"messages":[{"role":"user","content":"abcdefghij"}]}`)
	res := mustExtract(t, "openai", body)
	out, err := Rewrite(body, res.Segments, []Edit{
		{Seg: 0, From: 2, To: 6, Replacement: "[A]"},
		{Seg: 0, From: 4, To: 8, Replacement: "[B]"},
		{Seg: 0, From: 9, To: 10, Replacement: "[C]"},
	})
	if err != nil {
		t.Fatal(err)
	}
	// [2,6) and [4,8) overlap: their union [2,8) is replaced once, and no
	// character of either range is left behind.
	if got := mustExtract(t, "openai", out).Segments[0].Text; got != "ab[A]i[C]" {
		t.Fatalf("got %q, want %q", got, "ab[A]i[C]")
	}
}

func TestRewrite_RejectsEditsThatDoNotFit(t *testing.T) {
	body := []byte(`{"messages":[{"role":"user","content":"short"}]}`)
	res := mustExtract(t, "openai", body)
	for _, e := range []Edit{
		{Seg: 0, From: 2, To: 99, Replacement: "x"},
		{Seg: 0, From: 3, To: 3, Replacement: "x"},
		{Seg: 0, From: -1, To: 2, Replacement: "x"},
		{Seg: 5, From: 0, To: 1, Replacement: "x"},
	} {
		if out, err := Rewrite(body, res.Segments, []Edit{e}); err == nil {
			t.Errorf("edit %+v accepted, produced %s", e, out)
		}
	}
	if out, err := Rewrite(body, res.Segments, nil); err != nil || !bytes.Equal(out, body) {
		t.Fatalf("no edits must return the body unchanged, got %s (%v)", out, err)
	}
}

// For every segment of every fixture: replace its middle, and check that the
// body is still JSON, that exactly this segment changed, and that every byte
// outside the token that holds it is untouched.
func TestRewrite_EverySegmentOfEveryFixture(t *testing.T) {
	for name, provider := range fixtures {
		body, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		res := mustExtract(t, provider, body)
		for i, s := range res.Segments {
			from, to := len(s.Text)/3, len(s.Text)/3+1
			for from > 0 && !isRuneStart(s.Text[from]) {
				from--
			}
			for to < len(s.Text) && !isRuneStart(s.Text[to]) {
				to++
			}
			want := s.Text[:from] + `["\X]` + s.Text[to:]
			out, err := Rewrite(body, res.Segments, []Edit{{Seg: i, From: from, To: to, Replacement: `["\X]`}})
			if err != nil {
				t.Fatalf("%s %s: %v", name, s.Path, err)
			}
			if !json.Valid(out) {
				t.Fatalf("%s %s: output is not JSON", name, s.Path)
			}
			if !bytes.Equal(out[:s.Start], body[:s.Start]) || !bytes.Equal(out[len(out)-(len(body)-s.End):], body[s.End:]) {
				t.Fatalf("%s %s: bytes outside the token changed", name, s.Path)
			}
			after, ok := Extract(provider, out)
			if !ok || len(after.Segments) != len(res.Segments) {
				t.Fatalf("%s %s: rewritten body extracts differently", name, s.Path)
			}
			for j := range after.Segments {
				expect := res.Segments[j].Text
				if j == i {
					expect = want
				}
				if after.Segments[j].Text != expect || after.Segments[j].Path != res.Segments[j].Path {
					t.Fatalf("%s: after editing %s, segment %s reads %q, want %q", name, s.Path, after.Segments[j].Path, after.Segments[j].Text, expect)
				}
			}
		}
	}
}

func isRuneStart(b byte) bool { return b&0xC0 != 0x80 }
