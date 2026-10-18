package scanner

// An attachment the proxy could not read: a file in a request (a PDF, an
// office document, an image, an archive) whose text no scanner saw. The
// finding says why and carries nothing of the file: no match, no
// position, only the path of the attachment in the body. It is made by
// internal/attachment when the policy has an attachments section, and the
// section's actions decide about it (policy/attachments.go).
const (
	AttachmentFindingType = "attachment_unreadable"

	// AttachmentEncrypted is a file that is encrypted or password
	// protected: a ZIP with encrypted entries, a PDF with an /Encrypt
	// dictionary, an office document saved with a password, an encrypted
	// 7z or RAR archive, a PGP message or an encrypted private key.
	AttachmentEncrypted = "encrypted"
	// AttachmentUnsupportedType is a file of a kind nothing here reads, or
	// of a kind that is read only when attachments.extract is enabled.
	AttachmentUnsupportedType = "unsupported_type"
	// AttachmentTooLarge is a file over attachments.extract.max_bytes, one
	// whose text was cut short by a limit (pages, characters), or one that
	// would unpack to far more than it weighs.
	AttachmentTooLarge = "too_large"
	// AttachmentExtractFailed is a file the analyzer could not read: it is
	// damaged, the analyzer is not reachable, or reading took too long.
	AttachmentExtractFailed = "extract_failed"
	// AttachmentOCRUnavailable is an image, or a PDF without a text layer,
	// whose text could only be read by OCR, and OCR is off or not installed.
	AttachmentOCRUnavailable = "ocr_unavailable"
)

// AttachmentCategories are the categories of an attachment_unreadable
// finding.
var AttachmentCategories = []string{
	AttachmentEncrypted, AttachmentUnsupportedType, AttachmentTooLarge, AttachmentExtractFailed, AttachmentOCRUnavailable,
}
