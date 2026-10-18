package extract

import (
	"bytes"
	"errors"
	"testing"
)

// Every encoded file that is not decoded as text is listed, with its path,
// role, declared type and name, and is still recorded as skipped.
func TestExtract_ListsAttachments(t *testing.T) {
	tests := []struct {
		name, provider, body string
		want                 Attachment
	}{
		{"openai file", "openai",
			`{"messages":[{"role":"user","content":[{"type":"file","file":{"filename":"r.pdf","file_data":"data:application/pdf;base64,JVBERg=="}}]}]}`,
			Attachment{Role: RoleUser, Kind: KindText, Path: "messages[0].content[0].file.file_data", MIME: "application/pdf", Filename: "r.pdf", Data: "JVBERg=="}},
		{"openai image url", "openai",
			`{"messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"data:image/png;base64,iVBO"}}]}]}`,
			Attachment{Role: RoleUser, Kind: KindText, Path: "messages[0].content[0].image_url.url", MIME: "image/png", Data: "iVBO"}},
		{"responses input_file", "openai",
			`{"input":[{"role":"user","content":[{"type":"input_file","filename":"r.pdf","file_data":"data:application/pdf;base64,JVBERg=="}]}]}`,
			Attachment{Role: RoleUser, Kind: KindText, Path: "input[0].content[0].file_data", MIME: "application/pdf", Filename: "r.pdf", Data: "JVBERg=="}},
		{"anthropic document", "anthropic",
			`{"messages":[{"role":"user","content":[{"type":"document","source":{"type":"base64","media_type":"application/pdf","data":"JVBERg=="}}]}]}`,
			Attachment{Role: RoleTool, Kind: KindDocument, Path: "messages[0].content[0].source.data", MIME: "application/pdf", Data: "JVBERg=="}},
		{"anthropic image", "anthropic",
			`{"messages":[{"role":"user","content":[{"type":"image","source":{"type":"base64","media_type":"image/jpeg","data":"/9j/"}}]}]}`,
			Attachment{Role: RoleUser, Kind: KindText, Path: "messages[0].content[0].source.data", MIME: "image/jpeg", Data: "/9j/"}},
		{"gemini inline data", "gemini",
			`{"contents":[{"role":"user","parts":[{"inline_data":{"mime_type":"application/pdf","data":"JVBERg=="}}]}]}`,
			Attachment{Role: RoleUser, Kind: KindText, Path: "contents[0].parts[0].inline_data.data", MIME: "application/pdf", Data: "JVBERg=="}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, ok := Extract(tt.provider, []byte(tt.body))
			if !ok || len(res.Attachments) != 1 {
				t.Fatalf("attachments %+v", res)
			}
			got := res.Attachments[0]
			if string(tt.body[got.Start:got.End]) == "" || tt.body[got.Start] != '"' {
				t.Fatalf("token [%d,%d) is not the string", got.Start, got.End)
			}
			got.Start, got.End = 0, 0
			if got != tt.want {
				t.Fatalf("got  %+v\nwant %+v", got, tt.want)
			}
			skipped := false
			for _, s := range res.Skipped {
				skipped = skipped || (s.Path == tt.want.Path && s.Reason == "binary")
			}
			if !skipped {
				t.Fatal("not recorded as skipped")
			}
		})
	}
	// A text attachment is a segment, not an attachment; a link is neither.
	res, _ := Extract("anthropic", []byte(`{"messages":[{"role":"user","content":[{"type":"document","source":{"type":"base64","media_type":"text/plain","data":"aGVsbG8="}},{"type":"image","source":{"type":"url","url":"https://example.invalid/a.png"}}]}]}`))
	if len(res.Attachments) != 0 {
		t.Fatalf("listed %+v", res.Attachments)
	}
}

// The filename of a Responses API file stays a segment, at the path and
// role it had before the attachment was listed.
func TestExtract_InputFileNameIsStillScanned(t *testing.T) {
	res, _ := Extract("openai", []byte(`{"input":[{"role":"user","content":[{"type":"input_file","filename":"10000000146.pdf","file_data":"data:application/pdf;base64,JVBERg=="}]}]}`))
	found := false
	for _, s := range res.Segments {
		found = found || (s.Path == "input[0].content[0].filename" && s.Text == "10000000146.pdf" && s.Role == RoleUser)
	}
	if !found {
		t.Fatalf("segments %+v", res.Segments)
	}
}

// A segment read out of a file cannot be rewritten; one beside it can, and
// the rewrite is confirmed with the file's segments left out.
func TestRewrite_ExtractedSegments(t *testing.T) {
	body := []byte(`{"messages":[{"role":"user","content":[{"type":"text","text":"mail a@b.example"},{"type":"file","file":{"file_data":"data:application/pdf;base64,JVBERg=="}}]}]}`)
	res, ok := Extract("openai", body)
	if !ok || len(res.Attachments) != 1 {
		t.Fatalf("%+v", res)
	}
	att := res.Attachments[0]
	segs := append(res.Segments, Segment{Role: att.Role, Kind: KindDocument, Path: att.Path, Text: "page text a@b.example", Start: att.Start, End: att.End, Encoding: EncodingExtracted})
	text := 0
	for i, s := range segs {
		if s.Path == "messages[0].content[0].text" {
			text = i
		}
	}
	file := len(segs) - 1

	if _, err := RewriteChecked("openai", body, segs, []Edit{{Seg: file, From: 10, To: 21, Replacement: "[email_REDACTED]"}}); !errors.Is(err, ErrExtracted) {
		t.Fatalf("rewrite of a file's text: %v", err)
	}
	out, err := RewriteChecked("openai", body, segs, []Edit{{Seg: text, From: 5, To: 16, Replacement: "[email_REDACTED]"}})
	if err != nil {
		t.Fatalf("rewrite beside a file: %v", err)
	}
	if want := `"text":"mail [email_REDACTED]"`; !bytes.Contains(out, []byte(want)) || !bytes.Contains(out, []byte("JVBERg==")) {
		t.Fatalf("rewritten %s", out)
	}
}
