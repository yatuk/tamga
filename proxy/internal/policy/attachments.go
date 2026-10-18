package policy

import (
	"strings"
	"time"

	"github.com/yatuk/tamga/internal/scanner"
)

// AttachmentsConfig is the attachments section: what is done with the files
// a request carries that the scanners cannot read as they come (PDFs,
// office documents, images, archives), and whether their text is read out
// of them.
//
//	attachments:
//	  unreadable: WARN          # PASS, WARN or BLOCK
//	  encrypted: BLOCK          # PASS, WARN or BLOCK
//	  extract:
//	    enabled: true
//	    max_bytes: 10485760
//	    max_pages: 200
//	    timeout_ms: 10000
//	  ocr:
//	    enabled: false
//
// Without the section nothing changes: a file that is not text goes to the
// provider unread, as it always has. With it, every file in a request is
// looked at (internal/attachment):
//
//   - An encrypted one (a ZIP with encrypted entries, a password-protected
//     office file, an encrypted 7z or RAR, a PDF that needs a password, a
//     PGP message or an encrypted private key in a text) gets a finding of
//     type attachment_unreadable, category encrypted, and the action of
//     encrypted applies to it.
//   - With extract.enabled, a PDF, DOCX, XLSX or PPTX is sent to the
//     analyzer's Extract call, and with ocr.enabled an image or a PDF
//     without a text layer too. The text that comes back is scanned like
//     any other text, in the role of the message the file is in, and the
//     rules apply to what is found in it.
//   - Any other file, or one the analyzer could not read, gets a finding of
//     type attachment_unreadable with the reason as the category
//     (unsupported_type, too_large, extract_failed, ocr_unavailable), and
//     the action of unreadable applies to it.
//
// The two actions apply as a custom entity's own action applies to its
// matches (entry_action.go): a rule keyed attachment_unreadable applies as
// well and the stronger wins, scan.default_action does not, and an
// exception does not waive them. Empty means PASS: the finding is recorded
// and nothing is stopped.
//
// A value found in the text of a file cannot be masked there: the text is
// not written back into a PDF or a spreadsheet. A verdict of REDACT or
// STRIP that would have to change it blocks the request instead, as a
// REDACT that cannot be carried out always does (X-Tamga-Redact-Fallback:
// block:attachment).
type AttachmentsConfig struct {
	Unreadable string             `yaml:"unreadable" json:"unreadable,omitempty"`
	Encrypted  string             `yaml:"encrypted" json:"encrypted,omitempty"`
	Extract    *AttachmentExtract `yaml:"extract" json:"extract,omitempty"`
	OCR        *AttachmentOCR     `yaml:"ocr" json:"ocr,omitempty"`
}

// AttachmentExtract is attachments.extract.
type AttachmentExtract struct {
	Enabled bool `yaml:"enabled" json:"enabled"`
	// MaxBytes is the largest file sent to the analyzer, decoded. A larger
	// one is too_large. Zero means DefaultAttachmentMaxBytes.
	MaxBytes int `yaml:"max_bytes" json:"max_bytes,omitempty"`
	// MaxPages is the most pages, sheets or slides read of one file; a
	// file with more is read that far and also gets a too_large finding.
	// Zero means DefaultAttachmentMaxPages.
	MaxPages int `yaml:"max_pages" json:"max_pages,omitempty"`
	// TimeoutMs is how long one file may take to read. Zero means
	// DefaultAttachmentTimeoutMs.
	TimeoutMs int `yaml:"timeout_ms" json:"timeout_ms,omitempty"`
}

// AttachmentOCR is attachments.ocr.
type AttachmentOCR struct {
	Enabled bool `yaml:"enabled" json:"enabled"`
}

// Defaults and bounds of the attachments section. MaxAttachmentBytes is the
// largest file the analyzer takes in one call; the analyzer holds its own
// bounds to the same numbers.
const (
	DefaultAttachmentMaxBytes  = 10 << 20
	DefaultAttachmentMaxPages  = 200
	DefaultAttachmentTimeoutMs = 10000

	MinAttachmentBytes     = 1024
	MaxAttachmentBytes     = 32 << 20
	MaxAttachmentPages     = 2000
	MinAttachmentTimeoutMs = 100
	MaxAttachmentTimeoutMs = 60000
)

// AttachmentSpec is the attachments section as the proxy reads it, with the
// defaults filled in. Enabled is false when the section is absent: then no
// file is looked at.
type AttachmentSpec struct {
	Enabled  bool
	Extract  bool
	OCR      bool
	MaxBytes int
	MaxPages int
	Timeout  time.Duration
}

// AttachmentSpec returns the section as the proxy reads it.
func (p *Policy) AttachmentSpec() AttachmentSpec {
	if p == nil || p.Attachments == nil {
		return AttachmentSpec{}
	}
	c := p.Attachments
	spec := AttachmentSpec{
		Enabled:  true,
		MaxBytes: DefaultAttachmentMaxBytes,
		MaxPages: DefaultAttachmentMaxPages,
		Timeout:  DefaultAttachmentTimeoutMs * time.Millisecond,
	}
	if e := c.Extract; e != nil {
		spec.Extract = e.Enabled
		if e.MaxBytes > 0 {
			spec.MaxBytes = e.MaxBytes
		}
		if e.MaxPages > 0 {
			spec.MaxPages = e.MaxPages
		}
		if e.TimeoutMs > 0 {
			spec.Timeout = time.Duration(e.TimeoutMs) * time.Millisecond
		}
	}
	spec.OCR = spec.Extract && c.OCR != nil && c.OCR.Enabled
	return spec
}

// parseAttachmentAction reads unreadable or encrypted. ok is false for a
// value that is not one of the three.
func parseAttachmentAction(s string) (Action, bool) {
	switch a := Action(strings.ToUpper(strings.TrimSpace(s))); a {
	case "":
		return ActionPass, true
	case ActionPass, ActionWarn, ActionBlock:
		return a, true
	}
	return ActionPass, false
}

// attachmentAction is the section's action for an attachment_unreadable
// finding of the category, and false when the policy has no section.
func (p *Policy) attachmentAction(category string) (Action, bool) {
	if p == nil || p.Attachments == nil {
		return ActionPass, false
	}
	raw := p.Attachments.Unreadable
	if category == scanner.AttachmentEncrypted {
		raw = p.Attachments.Encrypted
	}
	a, _ := parseAttachmentAction(raw)
	return a, true
}

// attachmentIssues judges the section, enabled parts or not.
func attachmentIssues(p *Policy) []ValidationIssue {
	c := p.Attachments
	if c == nil {
		return nil
	}
	var issues []ValidationIssue
	for _, fv := range [][2]string{{"attachments.unreadable", c.Unreadable}, {"attachments.encrypted", c.Encrypted}} {
		field, value := fv[0], fv[1]
		if _, ok := parseAttachmentAction(value); !ok {
			issues = append(issues, boundError(field, "enum",
				"unknown action %q (use PASS, WARN or BLOCK: a file that cannot be read has nothing in it to mask, strip or route on)", printableValue(value)))
		}
	}
	if e := c.Extract; e != nil {
		if e.MaxBytes != 0 && (e.MaxBytes < MinAttachmentBytes || e.MaxBytes > MaxAttachmentBytes) {
			issues = append(issues, boundError("attachments.extract.max_bytes", "out_of_range",
				"max_bytes is %d, it must be between %d and %d (the largest file the analyzer takes)", e.MaxBytes, MinAttachmentBytes, MaxAttachmentBytes))
		}
		if e.MaxPages < 0 || e.MaxPages > MaxAttachmentPages {
			issues = append(issues, boundError("attachments.extract.max_pages", "out_of_range",
				"max_pages is %d, it must be between 1 and %d", e.MaxPages, MaxAttachmentPages))
		}
		if e.TimeoutMs != 0 && (e.TimeoutMs < MinAttachmentTimeoutMs || e.TimeoutMs > MaxAttachmentTimeoutMs) {
			issues = append(issues, boundError("attachments.extract.timeout_ms", "out_of_range",
				"timeout_ms is %d, it must be between %d and %d", e.TimeoutMs, MinAttachmentTimeoutMs, MaxAttachmentTimeoutMs))
		}
	}
	if c.OCR != nil && c.OCR.Enabled && (c.Extract == nil || !c.Extract.Enabled) {
		issues = append(issues, boundWarning("attachments.ocr.enabled", "no_effect",
			"ocr has no effect while attachments.extract.enabled is false: nothing is sent to the analyzer"))
	}
	if strings.EqualFold(strings.TrimSpace(c.Unreadable), string(ActionBlock)) && (c.Extract == nil || !c.Extract.Enabled) {
		issues = append(issues, boundWarning("attachments.unreadable", "blocks_every_file",
			"with attachments.extract off no file is read, so unreadable: BLOCK blocks every request that carries a file that is not text"))
	}
	return issues
}
