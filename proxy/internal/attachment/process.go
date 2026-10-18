package attachment

import (
	"context"
	"encoding/base64"
	"strings"
	"time"

	"github.com/yatuk/tamga/internal/extract"
	"github.com/yatuk/tamga/internal/scanner"
	pb "github.com/yatuk/tamga/proto/analyzer/v1"
)

// Extractor is the analyzer's Extract call (analyzer.GRPCClient).
type Extractor interface {
	Extract(ctx context.Context, req *pb.ExtractRequest) (*pb.ExtractResponse, error)
}

// Settings are the policy's attachments section as Process reads it
// (policy.AttachmentSpec).
type Settings struct {
	// Extract sends the files the analyzer reads to it; without it every
	// file that is not text is unsupported_type.
	Extract bool
	// OCR asks the analyzer to read images, and PDFs without a text layer.
	OCR      bool
	MaxBytes int
	MaxPages int
	// Timeout is how long one file may take.
	Timeout time.Duration
}

// Bounds of one request's files.
const (
	// MaxFiles is the most files of one request that are read. Each is a
	// call that can take Settings.Timeout; the files past it get a
	// too_large finding.
	MaxFiles = 8
	// MaxChars is the most characters of text taken from one file.
	MaxChars = 1_000_000
	// timeoutSlack is what the call's deadline adds to the analyzer's own,
	// for the transfer.
	timeoutSlack = 500 * time.Millisecond
)

// Outcome is what Process found.
type Outcome struct {
	// Segments are the texts read out of the files, one per page, sheet or
	// slide: kind document, the role of the message the file is in, the
	// file's path, and Encoding EncodingExtracted. They go after the
	// request's own segments.
	Segments []extract.Segment
	// Findings are the attachment_unreadable findings: one per file that
	// could not be read, or was read in part, and one per text segment
	// that holds an encrypted block. They have no position (Seg -1 for a
	// file) and carry nothing of the file.
	Findings []scanner.Finding
	// Read counts the files whose text was read, and Called the calls
	// made to the analyzer.
	Read, Called int
}

// Process looks at the files of a request: segs are the request's own
// segments (their texts are checked for encrypted blocks) and files the
// attachments the extractor listed. requestID goes to the analyzer with
// each call. It never fails: what cannot be done is a finding.
func Process(ctx context.Context, ex Extractor, set Settings, requestID string, segs []extract.Segment, files []extract.Attachment) Outcome {
	var out Outcome
	for i, s := range segs {
		if EncryptedText(s.Text) {
			out.Findings = append(out.Findings, finding(scanner.AttachmentEncrypted, string(s.Role), s.Path, i, KindText))
		}
	}
	for n, att := range files {
		out.file(ctx, ex, set, requestID, n, att)
	}
	return out
}

func finding(category, role, path string, seg int, kind string) scanner.Finding {
	return scanner.Finding{
		Type:       scanner.AttachmentFindingType,
		Category:   category,
		Severity:   "medium",
		Confidence: 1,
		Role:       role,
		Path:       path,
		Seg:        seg,
		Metadata:   map[string]string{"file_type": kind},
	}
}

func (o *Outcome) unread(att extract.Attachment, category, kind string) {
	o.Findings = append(o.Findings, finding(category, string(att.Role), att.Path, -1, kind))
}

// decode reads the base64 of an attachment, padded or not.
func decode(data string) ([]byte, bool) {
	data = strings.TrimSpace(data)
	if b, err := base64.StdEncoding.DecodeString(data); err == nil {
		return b, true
	}
	if b, err := base64.RawStdEncoding.DecodeString(strings.TrimRight(data, "=")); err == nil {
		return b, true
	}
	if b, err := base64.URLEncoding.DecodeString(data); err == nil {
		return b, true
	}
	if b, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(data, "=")); err == nil {
		return b, true
	}
	return nil, false
}

func (o *Outcome) file(ctx context.Context, ex Extractor, set Settings, requestID string, n int, att extract.Attachment) {
	if n >= MaxFiles {
		o.unread(att, scanner.AttachmentTooLarge, KindUnknown)
		return
	}
	data, ok := decode(att.Data)
	if !ok || len(data) == 0 {
		o.unread(att, scanner.AttachmentExtractFailed, KindUnknown)
		return
	}
	info := Detect(data)

	switch {
	case info.Kind == KindText:
		// Text sent as something else (application/octet-stream, a
		// type the extractor does not read as text): it is read here.
		if info.Encrypted {
			o.unread(att, scanner.AttachmentEncrypted, info.Kind)
		}
		o.add(att, string(data))
		o.Read++
		return
	case info.Encrypted && (info.Kind != KindPDF || !set.Extract):
		// A PDF whose encryption has an empty user password is read by
		// anyone, the analyzer included; it says encrypted otherwise.
		o.unread(att, scanner.AttachmentEncrypted, info.Kind)
		return
	case !info.Readable() || !set.Extract:
		o.unread(att, scanner.AttachmentUnsupportedType, info.Kind)
		return
	case info.Kind == KindImage && !set.OCR:
		o.unread(att, scanner.AttachmentOCRUnavailable, info.Kind)
		return
	case len(data) > set.MaxBytes:
		o.unread(att, scanner.AttachmentTooLarge, info.Kind)
		return
	case ex == nil:
		o.unread(att, scanner.AttachmentExtractFailed, info.Kind)
		return
	}

	callCtx, cancel := context.WithTimeout(ctx, set.Timeout+timeoutSlack)
	defer cancel()
	o.Called++
	resp, err := ex.Extract(callCtx, &pb.ExtractRequest{
		RequestId: requestID,
		Data:      data,
		MimeType:  att.MIME,
		Filename:  att.Filename,
		Limits: &pb.ExtractLimits{
			MaxBytes:  int64(set.MaxBytes),
			MaxPages:  uint32(set.MaxPages),
			MaxChars:  MaxChars,
			TimeoutMs: uint32(set.Timeout / time.Millisecond),
		},
		Ocr: set.OCR,
	})
	if err != nil || resp == nil {
		o.unread(att, scanner.AttachmentExtractFailed, info.Kind)
		return
	}
	if resp.GetStatus() != "ok" {
		o.unread(att, statusCategory(resp.GetStatus()), info.Kind)
		return
	}
	for _, part := range resp.GetParts() {
		o.add(att, part.GetText())
	}
	o.Read++
	if resp.GetTruncated() {
		// The rest of the file was not read: what is in it is unknown.
		o.unread(att, scanner.AttachmentTooLarge, info.Kind)
	}
}

// add keeps one text read out of a file as a segment.
func (o *Outcome) add(att extract.Attachment, text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	if len(text) > MaxChars {
		text = text[:MaxChars]
	}
	o.Segments = append(o.Segments, extract.Segment{
		Role:     att.Role,
		Kind:     extract.KindDocument,
		Path:     att.Path,
		Text:     strings.ToValidUTF8(text, "�"),
		Start:    att.Start,
		End:      att.End,
		Encoding: extract.EncodingExtracted,
	})
}

// statusCategory maps an Extract status to a finding category. A status
// this proxy does not know is a failure to read.
func statusCategory(status string) string {
	for _, c := range scanner.AttachmentCategories {
		if status == c {
			return c
		}
	}
	return scanner.AttachmentExtractFailed
}
