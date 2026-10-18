package attachment

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"testing"
	"time"

	"github.com/yatuk/tamga/internal/extract"
	"github.com/yatuk/tamga/internal/scanner"
	pb "github.com/yatuk/tamga/proto/analyzer/v1"
)

type fakeExtractor struct {
	calls []*pb.ExtractRequest
	resp  *pb.ExtractResponse
	// deadline is the time left on the context of the last call.
	deadline time.Duration
}

func (f *fakeExtractor) Extract(ctx context.Context, req *pb.ExtractRequest) (*pb.ExtractResponse, error) {
	f.calls = append(f.calls, req)
	if d, ok := ctx.Deadline(); ok {
		f.deadline = time.Until(d)
	}
	return f.resp, nil
}

var on = Settings{Extract: true, MaxBytes: 1 << 20, MaxPages: 10, Timeout: 2 * time.Second}

func file(path string, data []byte) extract.Attachment {
	return extract.Attachment{Role: extract.RoleUser, Kind: extract.KindText, Path: path, MIME: "application/pdf", Filename: "a.pdf", Data: base64.StdEncoding.EncodeToString(data), Start: 10, End: 20}
}

func categories(fs []scanner.Finding) []string {
	var out []string
	for _, f := range fs {
		if f.Type != scanner.AttachmentFindingType || f.Match != "" || f.Placed() {
			out = append(out, "bad finding")
			continue
		}
		out = append(out, f.Category)
	}
	return out
}

func TestProcess_ReadsWhatTheAnalyzerReads(t *testing.T) {
	ex := &fakeExtractor{resp: &pb.ExtractResponse{Status: "ok", Parts: []*pb.ExtractedPart{{Kind: "page", Index: 1, Text: "one"}, {Kind: "page", Index: 2, Text: "  "}, {Kind: "page", Index: 3, Text: "three"}}}}
	out := Process(context.Background(), ex, on, "req-1", nil, []extract.Attachment{file("p", []byte(pdfPlain))})
	if len(out.Findings) != 0 || out.Read != 1 || out.Called != 1 {
		t.Fatalf("%+v", out)
	}
	if len(out.Segments) != 2 || out.Segments[0].Text != "one" || out.Segments[1].Text != "three" {
		t.Fatalf("segments %+v", out.Segments)
	}
	s := out.Segments[0]
	if s.Kind != extract.KindDocument || s.Role != extract.RoleUser || s.Path != "p" || s.Encoding != extract.EncodingExtracted || s.Start != 10 || s.End != 20 {
		t.Fatalf("segment %+v", s)
	}
	req := ex.calls[0]
	if req.GetRequestId() != "req-1" || req.GetFilename() != "a.pdf" || req.GetLimits().GetMaxPages() != 10 || req.GetLimits().GetMaxChars() != MaxChars || req.GetOcr() {
		t.Fatalf("request %+v", req)
	}
	if ex.deadline <= on.Timeout || ex.deadline > on.Timeout+time.Second {
		t.Fatalf("deadline %v for a timeout of %v", ex.deadline, on.Timeout)
	}
}

func TestProcess_WhatIsNotSent(t *testing.T) {
	ex := &fakeExtractor{resp: &pb.ExtractResponse{Status: "ok"}}
	png := []byte("\x89PNG\r\n\x1a\nxxxx")
	cases := []struct {
		name string
		set  Settings
		data []byte
		want string
	}{
		{"encrypted zip", on, zipEncrypted(), scanner.AttachmentEncrypted},
		{"encrypted pdf, extract off", Settings{}, []byte(pdfEncryptedTrailer), scanner.AttachmentEncrypted},
		{"a pdf, extract off", Settings{}, []byte(pdfPlain), scanner.AttachmentUnsupportedType},
		{"an archive", on, []byte{0x1F, 0x8B, 8, 0}, scanner.AttachmentUnsupportedType},
		{"a legacy office file", on, cfb("WordDocument"), scanner.AttachmentUnsupportedType},
		{"an image, no OCR", on, png, scanner.AttachmentOCRUnavailable},
		{"too large", Settings{Extract: true, MaxBytes: 16, Timeout: time.Second}, []byte(pdfPlain), scanner.AttachmentTooLarge},
	}
	for _, tc := range cases {
		out := Process(context.Background(), ex, tc.set, "", nil, []extract.Attachment{file("p", tc.data)})
		if got := categories(out.Findings); len(got) != 1 || got[0] != tc.want {
			t.Errorf("%s: %v, want [%s]", tc.name, got, tc.want)
		}
		if len(out.Segments) != 0 {
			t.Errorf("%s: segments %+v", tc.name, out.Segments)
		}
	}
	if len(ex.calls) != 0 {
		t.Fatalf("%d calls for files that are not sent", len(ex.calls))
	}

	// An encrypted PDF is sent when extraction is on: its password may be
	// empty. A nil extractor is a failure to read.
	out := Process(context.Background(), ex, on, "", nil, []extract.Attachment{file("p", []byte(pdfEncryptedTrailer))})
	if len(ex.calls) != 1 || len(out.Findings) != 0 {
		t.Fatalf("encrypted pdf with extract on: %d calls, %v", len(ex.calls), categories(out.Findings))
	}
	out = Process(context.Background(), nil, on, "", nil, []extract.Attachment{file("p", []byte(pdfPlain))})
	if got := categories(out.Findings); len(got) != 1 || got[0] != scanner.AttachmentExtractFailed {
		t.Fatalf("no extractor: %v", got)
	}
}

func TestProcess_TextAndBounds(t *testing.T) {
	// Text sent as a binary type is read here, encrypted text is marked.
	text := file("t", []byte("plain words"))
	text.MIME = "application/octet-stream"
	pgp := file("g", []byte("-----BEGIN PGP MESSAGE-----\nhQEMA\n-----END PGP MESSAGE-----\n"))
	bad := extract.Attachment{Role: extract.RoleUser, Path: "b", Data: "%%% not base64"}
	segs := []extract.Segment{{Role: extract.RoleUser, Path: "messages[0].content", Text: "key:\n-----BEGIN ENCRYPTED PRIVATE KEY-----\nMIIE"}}
	out := Process(context.Background(), nil, Settings{}, "", segs, []extract.Attachment{text, pgp, bad})
	if got := categories(out.Findings); len(got) != 3 || got[0] != scanner.AttachmentEncrypted || got[1] != scanner.AttachmentEncrypted || got[2] != scanner.AttachmentExtractFailed {
		t.Fatalf("findings %v", got)
	}
	if out.Findings[0].Path != "messages[0].content" || out.Findings[0].Seg != 0 {
		t.Fatalf("the encrypted text's finding is not on its segment: %+v", out.Findings[0])
	}
	if len(out.Segments) != 2 || out.Segments[0].Text != "plain words" {
		t.Fatalf("segments %+v", out.Segments)
	}

	// More files than are read: the rest are too_large, unread.
	var many []extract.Attachment
	for i := 0; i < MaxFiles+2; i++ {
		many = append(many, file("p", []byte(pdfPlain)))
	}
	ex := &fakeExtractor{resp: &pb.ExtractResponse{Status: "ok"}}
	out = Process(context.Background(), ex, on, "", nil, many)
	if len(ex.calls) != MaxFiles {
		t.Fatalf("%d calls, want %d", len(ex.calls), MaxFiles)
	}
	if got := categories(out.Findings); len(got) != 2 || got[0] != scanner.AttachmentTooLarge {
		t.Fatalf("findings %v", got)
	}

	// A file read in part, and statuses the proxy does not know.
	ex = &fakeExtractor{resp: &pb.ExtractResponse{Status: "ok", Truncated: true, Parts: []*pb.ExtractedPart{{Text: "some"}}}}
	out = Process(context.Background(), ex, on, "", nil, []extract.Attachment{file("p", []byte(pdfPlain))})
	if got := categories(out.Findings); len(got) != 1 || got[0] != scanner.AttachmentTooLarge || len(out.Segments) != 1 {
		t.Fatalf("truncated: %v %+v", got, out.Segments)
	}
	ex = &fakeExtractor{resp: &pb.ExtractResponse{Status: "a status from a later analyzer"}}
	out = Process(context.Background(), ex, on, "", nil, []extract.Attachment{file("p", []byte(pdfPlain))})
	if got := categories(out.Findings); len(got) != 1 || got[0] != scanner.AttachmentExtractFailed {
		t.Fatalf("unknown status: %v", got)
	}
}

func zipEncrypted() []byte {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	f, _ := w.CreateHeader(&zip.FileHeader{Name: "word/document.xml", Method: zip.Deflate, Flags: 0x1})
	_, _ = f.Write([]byte("<x/>"))
	_ = w.Close()
	return buf.Bytes()
}
