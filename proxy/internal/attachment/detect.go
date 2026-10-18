// Package attachment reads what it can of the files a request carries:
// which kind of file each is, whether it is encrypted, and, through the
// analyzer's Extract call, the text of the kinds the analyzer reads (PDF,
// DOCX, XLSX, PPTX, and images with OCR). What it cannot read becomes an
// attachment_unreadable finding, and the policy's attachments section
// decides what is done with it.
//
// Detection here has no dependencies beyond the standard library and never
// decompresses anything: it reads signatures, the ZIP central directory, a
// PDF's trailer dictionaries and archive headers, all within fixed bounds.
package attachment

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"strings"
	"unicode/utf8"
)

// Kinds of file Detect tells apart.
const (
	KindPDF     = "pdf"
	KindDOCX    = "docx"
	KindXLSX    = "xlsx"
	KindPPTX    = "pptx"
	KindImage   = "image"
	KindText    = "text"
	KindZIP     = "zip"
	KindOffice  = "office_legacy" // a CFB container: .doc, .xls, .ppt, .msg
	Kind7z      = "7z"
	KindRAR     = "rar"
	KindGzip    = "gzip"
	KindODF     = "opendocument"
	KindUnknown = "unknown"
)

// Info is what Detect found out about a file.
type Info struct {
	Kind string
	// Encrypted is set when the file is encrypted or password protected,
	// in a way its own structure records.
	Encrypted bool
	// Image is the image format when Kind is KindImage: "png", "jpeg", …
	Image string
}

// Readable reports whether the analyzer reads this kind of file:
// documents always, images only by OCR.
func (i Info) Readable() bool {
	switch i.Kind {
	case KindPDF, KindDOCX, KindXLSX, KindPPTX, KindImage:
		return true
	}
	return false
}

// Bounds of what Detect looks at. A file is never read beyond them, so a
// crafted header costs no more than an ordinary one.
const (
	// pdfDictWindow bounds how far a dictionary that holds /Encrypt is
	// looked for around it, either way.
	pdfDictWindow = 64 << 10
	// pdfMaxEncryptKeys bounds how many occurrences of /Encrypt are looked
	// at. A trailer names it once; a file can be made to say it often.
	pdfMaxEncryptKeys = 64
	// zipMaxLocalHeaders bounds the local headers read when a ZIP has no
	// readable central directory.
	zipMaxLocalHeaders = 4096
	// rarMaxHeaders bounds the headers of a RAR archive that are walked.
	rarMaxHeaders = 256
	// sevenZipMaxHeader bounds the size of a 7z header that is read.
	sevenZipMaxHeader = 1 << 20
)

// Detect tells what kind of file data is, and whether it is encrypted. The
// declared type and name are not trusted for either: they are what the
// caller says, and a file can be given any name.
func Detect(data []byte) Info {
	switch {
	case pdfSignature(data):
		return Info{Kind: KindPDF, Encrypted: pdfEncrypted(data)}
	case bytes.HasPrefix(data, []byte("PK\x03\x04")), bytes.HasPrefix(data, []byte("PK\x05\x06")):
		return zipInfo(data)
	case bytes.HasPrefix(data, []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}):
		return cfbInfo(data)
	case bytes.HasPrefix(data, []byte{'7', 'z', 0xBC, 0xAF, 0x27, 0x1C}):
		return Info{Kind: Kind7z, Encrypted: sevenZipEncrypted(data)}
	case bytes.HasPrefix(data, []byte("Rar!\x1a\x07\x01\x00")):
		return Info{Kind: KindRAR, Encrypted: rar5Encrypted(data)}
	case bytes.HasPrefix(data, []byte("Rar!\x1a\x07\x00")):
		return Info{Kind: KindRAR, Encrypted: rar4Encrypted(data)}
	case bytes.HasPrefix(data, []byte{0x1F, 0x8B}):
		return Info{Kind: KindGzip}
	}
	if img := imageFormat(data); img != "" {
		return Info{Kind: KindImage, Image: img}
	}
	if isText(data) {
		return Info{Kind: KindText, Encrypted: EncryptedText(string(data))}
	}
	return Info{Kind: KindUnknown}
}

// pdfSignature reports whether data is a PDF. The header may follow some
// bytes of junk, which readers accept within the first kilobyte.
func pdfSignature(data []byte) bool {
	return bytes.Contains(data[:min(len(data), 1024)], []byte("%PDF-"))
}

func imageFormat(data []byte) string {
	switch {
	case bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")):
		return "png"
	case bytes.HasPrefix(data, []byte{0xFF, 0xD8, 0xFF}):
		return "jpeg"
	case bytes.HasPrefix(data, []byte("GIF87a")), bytes.HasPrefix(data, []byte("GIF89a")):
		return "gif"
	case len(data) >= 12 && bytes.Equal(data[:4], []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")):
		return "webp"
	case bytes.HasPrefix(data, []byte("II*\x00")), bytes.HasPrefix(data, []byte("MM\x00*")):
		return "tiff"
	case bytes.HasPrefix(data, []byte("BM")) && len(data) >= 26:
		return "bmp"
	}
	return ""
}

// isText reports whether data reads as text: valid UTF-8 without NUL.
func isText(data []byte) bool {
	return len(data) > 0 && utf8.Valid(data) && bytes.IndexByte(data, 0) < 0
}

// ── PDF ─────────────────────────────────────────────────────────────────

// pdfEncrypted reports whether a PDF names an /Encrypt dictionary in its
// trailer, or in the dictionary of a cross-reference stream, which takes
// the trailer's place in newer files. Both always hold /Root and /Size;
// no other dictionary of a PDF has /Encrypt next to them. Content streams
// are mostly compressed, and one that is not cannot put /Encrypt inside a
// dictionary that also has /Root without that dictionary being a trailer
// as far as any reader is concerned.
func pdfEncrypted(data []byte) bool {
	key := []byte("/Encrypt")
	from := 0
	for n := 0; n < pdfMaxEncryptKeys; n++ {
		i := bytes.Index(data[from:], key)
		if i < 0 {
			return false
		}
		i += from
		from = i + len(key)
		// /EncryptMetadata and the like are other names.
		if from < len(data) && pdfNameChar(data[from]) {
			continue
		}
		dict, ok := pdfEnclosingDict(data, i)
		if ok && bytes.Contains(dict, []byte("/Root")) && bytes.Contains(dict, []byte("/Size")) {
			return true
		}
	}
	return false
}

// pdfNameChar reports whether c continues a PDF name.
func pdfNameChar(c byte) bool {
	switch c {
	case ' ', '\t', '\r', '\n', '\f', 0, '/', '<', '>', '[', ']', '(', ')', '{', '}', '%':
		return false
	}
	return true
}

// pdfEnclosingDict returns the dictionary "<< … >>" that holds the byte at
// at, looked for within pdfDictWindow either way.
func pdfEnclosingDict(data []byte, at int) ([]byte, bool) {
	start := -1
	depth := 0
	for i := at - 1; i > 0 && at-i < pdfDictWindow; i-- {
		if data[i] == '>' && data[i-1] == '>' {
			depth++
			i--
			continue
		}
		if data[i] == '<' && data[i-1] == '<' {
			if depth == 0 {
				start = i - 1
				break
			}
			depth--
			i--
		}
	}
	if start < 0 {
		return nil, false
	}
	depth = 0
	for i := start; i+1 < len(data) && i-start < 2*pdfDictWindow; i++ {
		switch {
		case data[i] == '<' && data[i+1] == '<':
			depth++
			i++
		case data[i] == '>' && data[i+1] == '>':
			depth--
			i++
			if depth == 0 {
				return data[start : i+1], true
			}
		}
	}
	return nil, false
}

// ── ZIP and the formats built on it ─────────────────────────────────────

// zipAESExtra is the header ID of the WinZip AES extra field.
const zipAESExtra = 0x9901

// zipInfo tells a ZIP from the office formats built on it, and whether any
// entry is encrypted: general-purpose flag bit 0 (traditional PKWARE
// encryption, and also set for AES) or the AES extra field.
func zipInfo(data []byte) Info {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		// No central directory that can be read (a truncated file): the
		// local headers still carry the flags.
		return Info{Kind: KindZIP, Encrypted: zipLocalEncrypted(data)}
	}
	info := Info{Kind: KindZIP}
	names := map[string]bool{}
	for _, f := range r.File {
		names[f.Name] = true
		if f.Flags&0x1 != 0 || zipHasExtra(f.Extra, zipAESExtra) {
			info.Encrypted = true
		}
	}
	switch {
	case names["word/document.xml"]:
		info.Kind = KindDOCX
	case names["xl/workbook.xml"]:
		info.Kind = KindXLSX
	case names["ppt/presentation.xml"]:
		info.Kind = KindPPTX
	case names["mimetype"] && (names["content.xml"] || names["META-INF/manifest.xml"]):
		info.Kind = KindODF
	}
	return info
}

// zipHasExtra reports whether a ZIP extra field holds a record with the id.
func zipHasExtra(extra []byte, id uint16) bool {
	for len(extra) >= 4 {
		tag := binary.LittleEndian.Uint16(extra)
		size := int(binary.LittleEndian.Uint16(extra[2:]))
		if tag == id {
			return true
		}
		if 4+size > len(extra) {
			return false
		}
		extra = extra[4+size:]
	}
	return false
}

// zipLocalEncrypted reads the flags of the local file headers.
func zipLocalEncrypted(data []byte) bool {
	sig := []byte("PK\x03\x04")
	from := 0
	for n := 0; n < zipMaxLocalHeaders; n++ {
		i := bytes.Index(data[from:], sig)
		if i < 0 {
			return false
		}
		i += from
		if i+30 > len(data) {
			return false
		}
		flags := binary.LittleEndian.Uint16(data[i+6:])
		nameLen := int(binary.LittleEndian.Uint16(data[i+26:]))
		extraLen := int(binary.LittleEndian.Uint16(data[i+28:]))
		if flags&0x1 != 0 {
			return true
		}
		if end := i + 30 + nameLen + extraLen; end <= len(data) && zipHasExtra(data[i+30+nameLen:end], zipAESExtra) {
			return true
		}
		from = i + 4
	}
	return false
}

// ── CFB: legacy office files, and password-protected OOXML ─────────────

// cfbInfo looks at a Compound File Binary container. Office saves a
// password-protected .docx, .xlsx or .pptx as one, with the streams
// EncryptionInfo and EncryptedPackage in place of the ZIP. The names are
// in the container's directory as UTF-16LE; looking for them there is
// enough, and costs one pass.
func cfbInfo(data []byte) Info {
	info := Info{Kind: KindOffice}
	if bytes.Contains(data, utf16le("EncryptedPackage")) && bytes.Contains(data, utf16le("EncryptionInfo")) {
		info.Encrypted = true
	}
	return info
}

func utf16le(s string) []byte {
	out := make([]byte, 0, 2*len(s))
	for i := 0; i < len(s); i++ {
		out = append(out, s[i], 0)
	}
	return out
}

// ── 7z ──────────────────────────────────────────────────────────────────

// sevenZipAES is the coder id of 7zAES.
var sevenZipAES = []byte{0x06, 0xF1, 0x07, 0x01}

// sevenZipEncrypted reports whether the header of a 7z archive names the
// AES coder: the header itself is encrypted (7z -mhe), or it is stored
// plain and lists encrypted folders. A header that is compressed and not
// encrypted hides its folders' coders, and is not unpacked here: such an
// archive reads as not encrypted, and is an unsupported type all the same.
func sevenZipEncrypted(data []byte) bool {
	if len(data) < 32 {
		return false
	}
	off := binary.LittleEndian.Uint64(data[12:])
	size := binary.LittleEndian.Uint64(data[20:])
	if size == 0 || size > sevenZipMaxHeader || off > uint64(len(data)) {
		return false
	}
	start := 32 + off
	end := start + size
	if end > uint64(len(data)) || end < start {
		return false
	}
	return bytes.Contains(data[start:end], sevenZipAES)
}

// ── RAR ─────────────────────────────────────────────────────────────────

// rar4Encrypted walks the blocks of a RAR 1.5–4 archive: the main header's
// MHD_PASSWORD flag says the headers are encrypted, a file header's
// LHD_PASSWORD flag says that file is.
func rar4Encrypted(data []byte) bool {
	pos := 7
	for n := 0; n < rarMaxHeaders && pos+7 <= len(data); n++ {
		typ := data[pos+2]
		flags := binary.LittleEndian.Uint16(data[pos+3:])
		size := int(binary.LittleEndian.Uint16(data[pos+5:]))
		switch typ {
		case 0x73: // main header
			if flags&0x0080 != 0 {
				return true
			}
		case 0x74: // file header
			if flags&0x0004 != 0 {
				return true
			}
		case 0x7B: // end of archive
			return false
		}
		if size < 7 {
			return false
		}
		next := pos + size
		if flags&0x8000 != 0 && pos+11 <= len(data) {
			next += int(binary.LittleEndian.Uint32(data[pos+7:]))
		}
		if next <= pos {
			return false
		}
		pos = next
	}
	return false
}

// rar5Encrypted walks the headers of a RAR 5 archive: an archive
// encryption header (type 4) says the headers are encrypted, a file
// header (type 2) with an encryption record (type 1) in its extra area
// says that file is.
func rar5Encrypted(data []byte) bool {
	pos := 8
	for n := 0; n < rarMaxHeaders; n++ {
		if pos+4 > len(data) {
			return false
		}
		p := pos + 4 // CRC32
		hsize, w := uvarint(data, p)
		if w == 0 || hsize == 0 || hsize > uint64(len(data)) {
			return false
		}
		p += w
		hstart := p
		hend := hstart + int(hsize)
		if hend > len(data) {
			return false
		}
		typ, w := uvarint(data[:hend], p)
		if w == 0 {
			return false
		}
		p += w
		if typ == 4 {
			return true
		}
		if typ == 5 { // end of archive
			return false
		}
		hflags, w := uvarint(data[:hend], p)
		if w == 0 {
			return false
		}
		p += w
		var extraSize, dataSize uint64
		if hflags&0x1 != 0 {
			if extraSize, w = uvarint(data[:hend], p); w == 0 {
				return false
			}
			p += w
		}
		if hflags&0x2 != 0 {
			if dataSize, w = uvarint(data[:hend], p); w == 0 {
				return false
			}
		}
		if typ == 2 && extraSize > 0 && extraSize <= uint64(hend-hstart) && rar5HasRecord(data[hend-int(extraSize):hend], 1) {
			return true
		}
		next := uint64(hend) + dataSize
		if next > uint64(len(data)) || next <= uint64(pos) {
			return false
		}
		pos = int(next)
	}
	return false
}

// rar5HasRecord reports whether an extra area holds a record of the type.
func rar5HasRecord(extra []byte, want uint64) bool {
	p := 0
	for p < len(extra) {
		size, w := uvarint(extra, p)
		if w == 0 || size == 0 {
			return false
		}
		typ, tw := uvarint(extra, p+w)
		if tw == 0 {
			return false
		}
		if typ == want {
			return true
		}
		next := p + w + int(size)
		if next <= p || next > len(extra) {
			return false
		}
		p = next
	}
	return false
}

// uvarint reads a RAR 5 variable-length integer at data[p:], and returns
// its width, 0 when it does not end within data or within ten bytes.
func uvarint(data []byte, p int) (uint64, int) {
	var v uint64
	for i := 0; i < 10 && p+i < len(data); i++ {
		b := data[p+i]
		v |= uint64(b&0x7F) << (7 * i)
		if b&0x80 == 0 {
			return v, i + 1
		}
	}
	return 0, 0
}

// ── Text ────────────────────────────────────────────────────────────────

// encryptedTextMarkers are the armour lines of encrypted text: an OpenPGP
// message, an encrypted PKCS#8 key, an age file, and the header a
// traditional PEM key carries when it is encrypted.
var encryptedTextMarkers = []string{
	"-----BEGIN PGP MESSAGE-----",
	"-----BEGIN ENCRYPTED PRIVATE KEY-----",
	"-----BEGIN AGE ENCRYPTED FILE-----",
	"Proc-Type: 4,ENCRYPTED",
}

// EncryptedText reports whether a text holds an encrypted block: what it
// says cannot be read by any scanner. A signed PGP message, a public key
// and a private key that is not encrypted are not encrypted text (the last
// is a secret, which the secret scanners find).
func EncryptedText(s string) bool {
	if !strings.Contains(s, "-----BEGIN ") && !strings.Contains(s, "Proc-Type:") {
		return false
	}
	for _, m := range encryptedTextMarkers {
		if strings.Contains(s, m) {
			return true
		}
	}
	return false
}
