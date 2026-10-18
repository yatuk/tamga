package proxy

import (
	"context"
	"fmt"
	"net/http"

	"github.com/yatuk/tamga/internal/attachment"
	"github.com/yatuk/tamga/internal/extract"
	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/scanner"
)

// attachmentsHeader reports, on a request the policy's attachments section
// looked at, how many files it carried, how many were read and how many
// attachment_unreadable findings were made. Counts only.
const attachmentsHeader = "X-Tamga-Attachments"

func attachmentSettings(spec policy.AttachmentSpec) attachment.Settings {
	return attachment.Settings{
		Extract:  spec.Extract,
		OCR:      spec.OCR,
		MaxBytes: spec.MaxBytes,
		MaxPages: spec.MaxPages,
		Timeout:  spec.Timeout,
	}
}

// readAttachments applies the attachments section to a request read by
// segment: the texts read out of its files are added after its own
// segments, to be scanned with them, and what could not be read is
// returned as findings, to be judged with the scanners' own. Without the
// section it returns segs as they are and no findings: the files go on
// unread, as they always have.
func readAttachments(ctx context.Context, w http.ResponseWriter, ex attachment.Extractor, pol *policy.Policy,
	requestID string, segs []extract.Segment, files []extract.Attachment) ([]extract.Segment, []scanner.Finding) {
	spec := pol.AttachmentSpec()
	if !spec.Enabled {
		return segs, nil
	}
	out := attachment.Process(ctx, ex, attachmentSettings(spec), requestID, segs, files)
	if len(files) > 0 || len(out.Findings) > 0 {
		w.Header().Set(attachmentsHeader, fmt.Sprintf("files=%d; read=%d; findings=%d", len(files), out.Read, len(out.Findings)))
	}
	if len(out.Segments) > 0 {
		segs = append(segs[:len(segs):len(segs)], out.Segments...)
	}
	return segs, out.Findings
}
