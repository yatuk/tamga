package attachment

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"testing"
)

// zipOf builds a ZIP with the named entries. flags and extra are set on
// every entry.
func zipOf(t *testing.T, flags uint16, extra []byte, names ...string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, name := range names {
		fh := &zip.FileHeader{Name: name, Method: zip.Deflate, Flags: flags, Extra: extra}
		f, err := w.CreateHeader(fh)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte("<x/>")); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func aesExtra() []byte {
	// header id 0x9901, size 7: vendor version 2, "AE", strength 3, method 8.
	return []byte{0x01, 0x99, 0x07, 0x00, 0x02, 0x00, 'A', 'E', 0x03, 0x08, 0x00}
}

const pdfPlain = "%PDF-1.7\n1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n" +
	"xref\n0 1\n0000000000 65535 f \ntrailer\n<< /Size 3 /Root 1 0 R /ID [<01><01>] >>\nstartxref\n0\n%%EOF\n"

const pdfEncryptedTrailer = "%PDF-1.7\n1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n" +
	"4 0 obj\n<< /Filter /Standard /V 2 /R 3 /O <00> /U <00> /P -4 /EncryptMetadata true >>\nendobj\n" +
	"xref\n0 1\n0000000000 65535 f \ntrailer\n<< /Size 5 /Root 1 0 R /Encrypt 4 0 R /ID [<01><01>] >>\nstartxref\n0\n%%EOF\n"

const pdfEncryptedXRefStream = "%PDF-1.7\n1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n" +
	"9 0 obj\n<< /Type /XRef /Size 10 /W [1 2 1] /Root 1 0 R /Encrypt 8 0 R /Info << /Producer (x) >> /Length 0 >>\nstream\n\nendstream\nendobj\n%%EOF\n"

// A page's dictionary that mentions /Encrypt, and an Encrypt dictionary on
// its own, are not a trailer.
const pdfEncryptElsewhere = "%PDF-1.7\n1 0 obj\n<< /Type /Page /Encrypt 3 0 R >>\nendobj\n" +
	"2 0 obj\n<< /EncryptMetadata false >>\nendobj\ntrailer\n<< /Size 3 /Root 1 0 R >>\n%%EOF\n"

func cfb(names ...string) []byte {
	out := []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}
	out = append(out, make([]byte, 504)...)
	for _, n := range names {
		entry := make([]byte, 128)
		copy(entry, utf16le(n))
		out = append(out, entry...)
	}
	return out
}

func sevenZip(header []byte) []byte {
	out := []byte{'7', 'z', 0xBC, 0xAF, 0x27, 0x1C, 0, 4}
	out = append(out, 0, 0, 0, 0) // start header CRC
	off := make([]byte, 8)
	binary.LittleEndian.PutUint64(off, 3)
	size := make([]byte, 8)
	binary.LittleEndian.PutUint64(size, uint64(len(header)))
	out = append(out, off...)
	out = append(out, size...)
	out = append(out, 0, 0, 0, 0) // next header CRC
	out = append(out, 'a', 'b', 'c')
	return append(out, header...)
}

func rar4(mainFlags, fileFlags uint16) []byte {
	out := []byte("Rar!\x1a\x07\x00")
	main := []byte{0, 0, 0x73, 0, 0, 13, 0, 0, 0, 0, 0, 0, 0}
	binary.LittleEndian.PutUint16(main[3:], mainFlags)
	out = append(out, main...)
	file := make([]byte, 32)
	file[2] = 0x74
	binary.LittleEndian.PutUint16(file[3:], fileFlags)
	binary.LittleEndian.PutUint16(file[5:], 32)
	out = append(out, file...)
	return append(out, 0, 0, 0x7B, 0, 0, 7, 0)
}

func rar5(headers ...[]byte) []byte {
	out := []byte("Rar!\x1a\x07\x01\x00")
	for _, h := range headers {
		out = append(out, 0, 0, 0, 0)   // CRC32
		out = append(out, byte(len(h))) // header size as a one-byte vint
		out = append(out, h...)
	}
	return out
}

func TestDetect(t *testing.T) {
	// A RAR 5 file header: type 2, header flags 1 (extra area), extra area
	// size 3, file fields (flags 0, unpacked size 0, attributes 0,
	// compression 0, host 0, name length 1, name "a"), then the extra area:
	// one record of size 2, type 1 (encryption), one byte of data.
	rar5File := []byte{2, 0x01, 3, 0, 0, 0, 0, 0, 1, 'a', 2, 1, 0}
	rar5Plain := []byte{2, 0x00, 0, 0, 0, 0, 0, 1, 'a'}

	tests := []struct {
		name      string
		data      []byte
		kind      string
		encrypted bool
	}{
		{"pdf", []byte(pdfPlain), KindPDF, false},
		{"pdf with junk before the header", append([]byte("junk\n"), pdfPlain...), KindPDF, false},
		{"encrypted pdf, trailer", []byte(pdfEncryptedTrailer), KindPDF, true},
		{"encrypted pdf, xref stream", []byte(pdfEncryptedXRefStream), KindPDF, true},
		{"pdf naming /Encrypt outside a trailer", []byte(pdfEncryptElsewhere), KindPDF, false},
		{"zip", zipOf(t, 0, nil, "a.txt"), KindZIP, false},
		{"zip, traditional encryption", zipOf(t, 0x1, nil, "a.txt"), KindZIP, true},
		{"zip, AES extra field", zipOf(t, 0, aesExtra(), "a.txt"), KindZIP, true},
		{"docx", zipOf(t, 0, nil, "[Content_Types].xml", "word/document.xml"), KindDOCX, false},
		{"xlsx", zipOf(t, 0, nil, "[Content_Types].xml", "xl/workbook.xml"), KindXLSX, false},
		{"pptx", zipOf(t, 0, nil, "[Content_Types].xml", "ppt/presentation.xml"), KindPPTX, false},
		{"odt", zipOf(t, 0, nil, "mimetype", "content.xml"), KindODF, false},
		{"password-protected office file", cfb("Root Entry", "EncryptionInfo", "EncryptedPackage"), KindOffice, true},
		{"legacy office file", cfb("Root Entry", "WordDocument"), KindOffice, false},
		{"7z with an AES header", sevenZip([]byte{0x17, 0x06, 0x0B, 0x01, 0x24, 0x06, 0xF1, 0x07, 0x01, 0x00}), Kind7z, true},
		{"7z", sevenZip([]byte{0x01, 0x04, 0x06, 0x00, 0x01, 0x09, 0x00}), Kind7z, false},
		{"rar4, encrypted headers", rar4(0x0080, 0), KindRAR, true},
		{"rar4, encrypted file", rar4(0, 0x0004), KindRAR, true},
		{"rar4", rar4(0, 0), KindRAR, false},
		{"rar5, encrypted headers", rar5([]byte{4, 0, 0, 0}), KindRAR, true},
		{"rar5, encrypted file", rar5([]byte{1, 0, 0}, rar5File), KindRAR, true},
		{"rar5", rar5([]byte{1, 0, 0}, rar5Plain, []byte{5, 0, 0}), KindRAR, false},
		{"gzip", []byte{0x1F, 0x8B, 8, 0, 0, 0, 0, 0}, KindGzip, false},
		{"png", []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"), KindImage, false},
		{"jpeg", []byte{0xFF, 0xD8, 0xFF, 0xE0, 0, 0x10}, KindImage, false},
		{"text", []byte("hello\nworld\n"), KindText, false},
		{"pgp message", []byte("note\n-----BEGIN PGP MESSAGE-----\n\nhQEMA\n-----END PGP MESSAGE-----\n"), KindText, true},
		{"pgp public key", []byte("-----BEGIN PGP PUBLIC KEY BLOCK-----\nmQ\n-----END PGP PUBLIC KEY BLOCK-----\n"), KindText, false},
		{"binary", []byte{0, 1, 2, 3, 0xFE}, KindUnknown, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Detect(tt.data)
			if got.Kind != tt.kind || got.Encrypted != tt.encrypted {
				t.Fatalf("Detect = %+v, want kind %s encrypted %v", got, tt.kind, tt.encrypted)
			}
		})
	}
}

// A ZIP cut short before its central directory still says, in its local
// headers, that an entry is encrypted.
func TestDetectTruncatedZip(t *testing.T) {
	full := zipOf(t, 0x1, nil, "a.txt", "b.txt")
	cut := full[:len(full)/2]
	if got := Detect(cut); got.Kind != KindZIP || !got.Encrypted {
		t.Fatalf("Detect(truncated encrypted zip) = %+v", got)
	}
	if got := Detect(zipOf(t, 0, nil, "a.txt")[:40]); got.Encrypted {
		t.Fatalf("a truncated plain zip read as encrypted: %+v", got)
	}
}

func TestEncryptedText(t *testing.T) {
	for _, s := range []string{
		"-----BEGIN ENCRYPTED PRIVATE KEY-----\nMIIE\n-----END ENCRYPTED PRIVATE KEY-----",
		"-----BEGIN RSA PRIVATE KEY-----\nProc-Type: 4,ENCRYPTED\nDEK-Info: AES-128-CBC,00\n",
		"-----BEGIN AGE ENCRYPTED FILE-----\nYWdl\n-----END AGE ENCRYPTED FILE-----",
	} {
		if !EncryptedText(s) {
			t.Errorf("EncryptedText(%.40q) = false", s)
		}
	}
	for _, s := range []string{"", "plain text", "-----BEGIN PGP SIGNED MESSAGE-----\nHash: SHA256\n\nhello"} {
		if EncryptedText(s) {
			t.Errorf("EncryptedText(%.40q) = true", s)
		}
	}
}

// Every prefix of every fixture is read without a panic: Detect sees what
// the caller sends, and a cut file is the commonest malformed one.
func TestDetectPrefixesDoNotPanic(t *testing.T) {
	fixtures := [][]byte{
		[]byte(pdfEncryptedTrailer), []byte(pdfEncryptedXRefStream), zipOf(t, 0x1, aesExtra(), "a", "b"),
		cfb("EncryptionInfo", "EncryptedPackage"), sevenZip([]byte{0x17, 0x06, 0xF1, 0x07, 0x01}),
		rar4(0x80, 4), rar5([]byte{1, 0, 0}, []byte{2, 0x01, 3, 0, 0, 0, 0, 0, 1, 'a', 2, 1, 0}),
	}
	for _, f := range fixtures {
		for n := 0; n <= len(f); n++ {
			_ = Detect(f[:n])
		}
	}
	// Headers that claim more than there is.
	_ = Detect(append([]byte("Rar!\x1a\x07\x01\x00\x00\x00\x00\x00"), 0xFF, 0xFF, 0xFF, 0xFF, 0x7F))
	_ = Detect(append([]byte("Rar!\x1a\x07\x00"), 0, 0, 0x74, 0, 0x80, 7, 0, 0xFF, 0xFF, 0xFF, 0xFF))
	bad7z := sevenZip(nil)
	binary.LittleEndian.PutUint64(bad7z[12:], ^uint64(0))
	binary.LittleEndian.PutUint64(bad7z[20:], 16)
	_ = Detect(bad7z)
}
