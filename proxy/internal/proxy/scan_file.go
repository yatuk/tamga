package proxy

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/yatuk/tamga/internal/extract"
	"github.com/yatuk/tamga/internal/policy"
)

// A file sent to the scan API: {"file": {"data", "name", "mime"}, "role"}.
// It is put in a chat request as an OpenAI file part and read by the same
// path as an attachment on the proxy path (policy attachments section,
// internal/attachment): the scan API takes files when the policy has the
// section, and its answer says which kinds it reads (scanResponse.Files).

// scanFileRequest is the "file" member of a scan request.
type scanFileRequest struct {
	// Data is the file in standard base64.
	Data *string `json:"data"`
	Name string  `json:"name"`
	MIME string  `json:"mime"`
}

// scanFiles is what a scan answer says about files, when the policy has an
// attachments section: the kinds a file is read as, and the largest file
// read. A file of another kind is still taken, and judged as one that could
// not be read.
type scanFiles struct {
	Kinds    []string `json:"kinds"`
	MaxBytes int      `json:"max_bytes"`
}

// The scan API's messages about files. Fixed, as the others are.
const (
	scanErrFilesOff   = "files are not read: the policy has no attachments section"
	scanErrTextOrFile = "send text or file, not both"
	scanErrFileData   = "file.data must be standard base64"
	scanErrFileName   = "file.name must be at most 255 characters, without control characters"
	scanErrFileMIME   = "file.mime must be a media type such as application/pdf"
)

// scanFileSlack is what a scan request holds besides the file's base64.
const scanFileSlack = 64 << 10

var scanFileMIME = regexp.MustCompile(`^[a-z0-9][a-z0-9!#$&^_.+-]{0,63}/[a-z0-9][a-z0-9!#$&^_.+-]{0,63}$`)

// scanFilesOf is what an answer says about files under the policy, nil
// without the attachments section.
func scanFilesOf(pol *policy.Policy) *scanFiles {
	spec := pol.AttachmentSpec()
	if !spec.Enabled {
		return nil
	}
	kinds := []string{"pdf", "docx", "xlsx", "pptx"}
	if spec.OCR {
		kinds = append(kinds, "image")
	}
	return &scanFiles{Kinds: kinds, MaxBytes: spec.MaxBytes}
}

// scanBodyLimit is the largest scan request read, and the message for one
// that is larger. With the attachments section it is large enough for a
// file of attachments.extract.max_bytes; a text is still held to
// scanMaxBodyBytes once the request is read.
func scanBodyLimit(pol *policy.Policy) (int64, string) {
	spec := pol.AttachmentSpec()
	if !spec.Enabled {
		return scanMaxBodyBytes, scanErrTooLarge
	}
	limit := int64(base64.StdEncoding.EncodedLen(spec.MaxBytes)) + scanFileSlack
	if limit <= scanMaxBodyBytes {
		return scanMaxBodyBytes, scanErrTooLarge
	}
	return limit, fmt.Sprintf("request body exceeds %d bytes", limit)
}

// checkScanFile refuses a file member that is not what the API takes. The
// size of the file is not checked here: one over max_bytes is judged, as on
// the proxy path, as too_large.
func checkScanFile(f *scanFileRequest) string {
	if f.Data == nil || *f.Data == "" {
		return scanErrFileData
	}
	if _, err := base64.StdEncoding.DecodeString(*f.Data); err != nil {
		return scanErrFileData
	}
	if utf8.RuneCountInString(f.Name) > 255 || strings.IndexFunc(f.Name, func(r rune) bool { return r < 0x20 || r == 0x7f }) >= 0 {
		return scanErrFileName
	}
	if f.MIME != "" && !scanFileMIME.MatchString(f.MIME) {
		return scanErrFileMIME
	}
	return ""
}

// scanFileStatus is the status for a refused file member.
func scanFileStatus(msg string) int {
	if msg == scanErrFilesOff {
		return http.StatusUnprocessableEntity
	}
	return http.StatusBadRequest
}

// filePath is where the file is in the request evaluateFile builds.
const filePath = "messages[0].content[0].file.file_data"

// evaluateFile is the scan path for a file: the request a chat client
// would send with the file attached, in the role given.
func (h *scanHandler) evaluateFile(ctx context.Context, call *scanCall, requestID string, who caller, pol *policy.Policy, role string, f *scanFileRequest) (ev scanEval, outcome string) {
	mime := f.MIME
	if mime == "" {
		mime = "application/octet-stream"
	}
	file := map[string]string{"file_data": "data:" + mime + ";base64," + *f.Data}
	if f.Name != "" {
		file["filename"] = f.Name
	}
	doc := map[string]any{"messages": []any{map[string]any{
		"role":    role,
		"content": []any{map[string]any{"type": "file", "file": file}},
	}}}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(doc); err != nil {
		return ev, "segment_mismatch"
	}
	body := bytes.TrimSuffix(buf.Bytes(), []byte("\n"))
	res, ok := extract.Extract(scanProvider, body)
	if !ok {
		return ev, "segment_mismatch"
	}
	in := scanInput{body: body, segs: res.Segments, files: res.Attachments, locked: redactAttachment}
	if strings.HasPrefix(mime, "text/") {
		// A text file is decoded by the extractor and is a segment of the
		// request, as on the proxy path.
		if len(in.files) != 0 {
			return ev, "segment_mismatch"
		}
	} else if len(in.files) != 1 || in.files[0].Path != filePath {
		return ev, "segment_mismatch"
	}
	return h.evaluateInput(ctx, call, requestID, who, pol, in)
}
