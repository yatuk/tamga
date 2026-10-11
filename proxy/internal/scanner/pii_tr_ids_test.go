package scanner

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/yatuk/tamga/internal/extract"
)

// Test values that belong to nobody: a MERSİS number built from the tax
// number 1234567890 (which passes the VKN check), one built from the repo's
// test TCKN, and the IMEI that the literature uses as its example.
const (
	testMersisLegal   = "0123456789000015"
	testMersisNatural = "1000000014600015"
	testIMEI          = "490154203237518"
	testPassportTR    = "U12345678"
)

func scanPII(t *testing.T, text string) []Finding {
	t.Helper()
	found, err := cardRegistry().ScanSegments(context.Background(),
		[]extract.Segment{seg(extract.RoleUser, "m[0]", text)}, PipelineConfig{Mode: ModeSync})
	if err != nil {
		t.Fatal(err)
	}
	return found
}

func categoryNames(found []Finding) []string {
	var out []string
	for _, f := range found {
		out = append(out, f.Category)
	}
	return out
}

func TestTRIdentifiers_Found(t *testing.T) {
	if !validVKN("1234567890") || !validTCKN("10000000146") || !validIMEI(testIMEI) {
		t.Fatal("the test values must pass their own check digits")
	}
	cases := []struct {
		name, text, category, severity, masked string
	}{
		{"mersis of a company", "Şirketin MERSİS No: " + testMersisLegal + " olarak kayıtlı", categoryMersis, "high", "Şirketin MERSİS No: [mersis_no_REDACTED] olarak kayıtlı"},
		{"mersis in ASCII capitals", "MERSIS NO " + testMersisLegal, categoryMersis, "high", "MERSIS NO [mersis_no_REDACTED]"},
		{"mersis with hyphens", "Mersis: 0-1234-5678-9000015.", categoryMersis, "high", "Mersis: [mersis_no_REDACTED]."},
		{"mersis of a sole proprietor", "mersis numarası " + testMersisNatural, categoryMersis, "critical", "mersis numarası [mersis_no_REDACTED]"},
		{"word after the number", testMersisLegal + " MERSİS numaralı şirket", categoryMersis, "high", "[mersis_no_REDACTED] MERSİS numaralı şirket"},
		{"imei", "Cihazın IMEI numarası " + testIMEI + " kayıt dışı görünüyor", categoryIMEI, "high", "Cihazın IMEI numarası [imei_REDACTED] kayıt dışı görünüyor"},
		{"imei as printed", "IMEI: 49-015420-323751-8", categoryIMEI, "high", "IMEI: [imei_REDACTED]"},
		{"imei with a suffix on the word", "telefonun IMEI'si " + testIMEI, categoryIMEI, "high", "telefonun IMEI'si [imei_REDACTED]"},
		{"passport", "Pasaport No: " + testPassportTR + " ile giriş yaptı", categoryPassportTR, "high", "Pasaport No: [passport_tr_REDACTED] ile giriş yaptı"},
		{"passport, word with an ending", "pasaportumun numarası " + testPassportTR, categoryPassportTR, "high", "pasaportumun numarası [passport_tr_REDACTED]"},
		{"passport, english", "Turkish passport " + testPassportTR + " expires in May", categoryPassportTR, "high", "Turkish passport [passport_tr_REDACTED] expires in May"},
		{"plate, word after", "34 ABC 123 plakalı araç kazaya karıştı", categoryPlateTR, "medium", "[license_plate_tr_REDACTED] plakalı araç kazaya karıştı"},
		{"plate, word before", "Plaka: 06 AB 1234", categoryPlateTR, "medium", "Plaka: [license_plate_tr_REDACTED]"},
		{"plate without spaces", "aracın plakası 35A12345", categoryPlateTR, "medium", "aracın plakası [license_plate_tr_REDACTED]"},
		{"plate, english", "license plate 81-TK-1907 was seen", categoryPlateTR, "medium", "license plate [license_plate_tr_REDACTED] was seen"},
	}
	for _, tc := range cases {
		found := scanPII(t, tc.text)
		got := byCategory(found, tc.category)
		if len(got) != 1 {
			t.Errorf("%s: %d %s findings in %q, want 1 (all: %v)", tc.name, len(got), tc.category, tc.text, categoryNames(found))
			continue
		}
		f := got[0]
		if f.Severity != tc.severity {
			t.Errorf("%s: severity %q, want %q", tc.name, f.Severity, tc.severity)
		}
		if f.fp == 0 {
			t.Errorf("%s: no fingerprint", tc.name)
		}
		if !f.Placed() {
			t.Errorf("%s: the finding has no position", tc.name)
		}
		if masked := string(RedactContent([]byte(tc.text), got)); masked != tc.masked {
			t.Errorf("%s: masked = %q, want %q", tc.name, masked, tc.masked)
		}
	}
}

// Every shape a plate has, and the shapes it does not have.
func TestPlateTR_Shapes(t *testing.T) {
	for plate, want := range map[string]bool{
		"34 A 1234": true, "34 A 12345": true, "34 AB 123": true, "34 AB 1234": true,
		"34 ABC 12": true, "34 ABC 123": true, "01 ZZ 999": true, "81 A 0001": true,
		"34 A 123": false, "34 AB 12": false, "34 AB 12345": false, "34 ABC 1234": false,
		"34 ABCD 12": false, "00 AB 123": false, "82 AB 123": false, "99 AB 123": false,
		"34 QA 123": false, "34 AW 123": false, "34 XB 123": false, "34 ab 123": false,
	} {
		found := byCategory(scanPII(t, "plaka "+plate+" olarak girildi"), categoryPlateTR)
		if (len(found) == 1) != want {
			t.Errorf("%q: %d findings, want found=%v", plate, len(found), want)
		}
	}
}

// The same shapes without the word that names them, with the word too far
// away or inside another word, and with check digits that fail: nothing.
func TestTRIdentifiers_NotFound(t *testing.T) {
	far := strings.Repeat("lorem ipsum dolor sit amet ", 5)
	for name, text := range map[string]string{
		"sixteen digits, no word":         "sipariş numarası " + testMersisLegal + " kargoya verildi",
		"mersis, tax number fails":        "MERSİS No: 0123456788000015",
		"mersis, national id fails":       "MERSİS No: 1000000014700015",
		"mersis, a longer run":            "MERSİS " + testMersisLegal + "77",
		"mersis, a longer grouped run":    "MERSİS 0123 4567 8900 0015 7788",
		"mersis, word too far before":     "MERSİS " + far + testMersisLegal,
		"mersis, all the same digit":      "MERSİS No: 0000000000000000",
		"word inside another word":        "supermersis " + testMersisLegal,
		"passport, word after":            testPassportTR + " numaralı pasaport",
		"passport, word on another line":  "pasaport_no VARCHAR(9)\nbuild B20261011",
		"passport, word too far before":   "pasaport_no sütunu eklendi, migration dosyasına bak; build B20261011",
		"imei, no word":                   "takip no " + "86" + "0000000000000",
		"imei, check digit fails":         "IMEI 490154203237519",
		"imei, sixteen digits (IMEISV)":   "IMEI 4901542032375181",
		"passport, no word":               "bilet kodu " + testPassportTR + " onaylandı",
		"passport, lower case letter":     "pasaport no u12345678",
		"passport, seven digits":          "pasaport no U1234567",
		"passport, one repeated digit":    "pasaport no A00000000",
		"passport, part of a longer code": "pasaport dosyası XU12345678Z",
		"plate, no word":                  "parça kodu 34 ABC 123 stokta yok",
		"plate, word is template":         "the template 34 ABC 123 is used for the report",
		"plate, lower case":               "plaka sayısı 34 ve 123 olarak değişti",
		"plate, word too far after":       "34 ABC 123 " + far + far + "plaka",
		"plain turkish text":              "MERSİS kaydı, IMEI sorgusu, pasaport başvurusu ve plaka değişikliği için e-Devlet kullanılır.",
	} {
		found := scanPII(t, text)
		for _, f := range found {
			switch f.Category {
			case categoryMersis, categoryIMEI, categoryPassportTR, categoryPlateTR:
				t.Errorf("%s: %s found in %q", name, f.Category, text)
			}
		}
	}
}

// An IMEI passes the Luhn check by construction and used to be reported as
// a card number. Named as an IMEI it is one; without the name, or when it
// starts like a fifteen digit card, it is still a card.
func TestIMEI_IsNotACardWhenNamed(t *testing.T) {
	named := scanPII(t, "IMEI: "+testIMEI)
	if p, _ := countCategory(named, "credit_card"); p != 0 {
		t.Errorf("a named IMEI is also reported as a card: %v", categoryNames(named))
	}
	if p, _ := countCategory(named, categoryIMEI); p != 1 {
		t.Errorf("a named IMEI: %d imei findings, want 1", p)
	}
	bare := scanPII(t, "numara: "+testIMEI)
	if p, _ := countCategory(bare, "credit_card"); p != 1 {
		t.Errorf("fifteen Luhn-valid digits without a name: %v, want credit_card as before", categoryNames(bare))
	}
	// An American Express test number next to the word IMEI stays a card.
	amex := scanPII(t, "IMEI: 378282246310005")
	if p, _ := countCategory(amex, "credit_card"); p != 1 {
		t.Errorf("an AmEx number named IMEI: %v, want credit_card", categoryNames(amex))
	}
	// And so does a sixteen digit card.
	card := scanPII(t, "IMEI: "+cardA)
	if p, _ := countCategory(card, "credit_card"); p != 1 {
		t.Errorf("a sixteen digit card named IMEI: %v, want credit_card", categoryNames(card))
	}
}

// One MERSİS number in ten passes the Luhn check. Named as a MERSİS number
// and built from a valid tax number it is not a card; a card number next to
// the word MERSİS is still a card.
func TestMersis_IsNotACardWhenNamed(t *testing.T) {
	luhnValid := ""
	for i := 0; i < 100000 && luhnValid == ""; i++ {
		if m := fmt.Sprintf("01234567890%05d", i); validLuhn(m) {
			luhnValid = m
		}
	}
	if luhnValid == "" || mersisKind(luhnValid) != mersisLegalEntity {
		t.Fatalf("no Luhn-valid MERSİS number built: %q", luhnValid)
	}
	named := scanPII(t, "MERSİS No: "+luhnValid)
	if p, _ := countCategory(named, "credit_card"); p != 0 {
		t.Errorf("a named MERSİS number is also reported as a card: %v", categoryNames(named))
	}
	if p, _ := countCategory(named, categoryMersis); p != 1 {
		t.Errorf("a named MERSİS number: %d findings, want 1", p)
	}
	if p, _ := countCategory(scanPII(t, "ödeme: "+luhnValid), "credit_card"); p != 1 {
		t.Error("the same digits without the name are a card, as before")
	}
	card := scanPII(t, "MERSİS No: "+cardA)
	if p, _ := countCategory(card, "credit_card"); p != 1 {
		t.Errorf("a card number named MERSİS: %v, want credit_card", categoryNames(card))
	}
	if p, _ := countCategory(card, categoryMersis); p != 0 {
		t.Errorf("a card number named MERSİS is reported as a MERSİS number: %v", categoryNames(card))
	}
}

// The match keeps what the other PII masks keep, the first two and the last
// two characters, and two numbers with the same mask are told apart by
// fingerprint: the second is not an echo of the first, and both are masked.
func TestTRIdentifiers_MaskAndFingerprint(t *testing.T) {
	// The same first two and last two digits, another tax number between.
	other := "0100000000000015"
	if mersisKind(other) != mersisLegalEntity {
		t.Fatal("the second MERSİS number must be valid")
	}
	text := "MERSİS " + testMersisLegal + " ve MERSİS " + other
	found := byCategory(scanPII(t, text), categoryMersis)
	if len(found) != 2 {
		t.Fatalf("%d findings, want 2", len(found))
	}
	if found[0].Match != found[1].Match || found[0].fp == found[1].fp {
		t.Fatalf("the two must share a match and differ in fingerprint: %q %q", found[0].Match, found[1].Match)
	}
	if got := dropOverlapDuplicates([]Finding{found[0], found[1], found[0]}); len(got) != 2 {
		t.Fatalf("%d findings after the chunk overlap, want both numbers once", len(got))
	}
	if masked := string(RedactContent([]byte(text), found)); masked != "MERSİS [mersis_no_REDACTED] ve MERSİS [mersis_no_REDACTED]" {
		t.Fatalf("masked = %q", masked)
	}
	if idFingerprint("0-1234-5678-9000015") != idFingerprint(testMersisLegal) {
		t.Error("one number written with hyphens must be the same number")
	}

	longDigits := regexp.MustCompile(`\d{5,}`)
	for _, tc := range []struct{ text, category, match string }{
		{"MERSİS " + testMersisLegal, categoryMersis, "01************15"},
		{"IMEI " + testIMEI, categoryIMEI, "49***********18"},
		{"pasaport " + testPassportTR, categoryPassportTR, "U1*****78"},
		{"plaka 34 ABC 123", categoryPlateTR, "34 *** *23"},
	} {
		got := byCategory(scanPII(t, tc.text), tc.category)
		if len(got) != 1 {
			t.Errorf("%q: %d findings", tc.text, len(got))
			continue
		}
		if got[0].Match != tc.match {
			t.Errorf("%q: match = %q, want %q", tc.text, got[0].Match, tc.match)
		}
		raw, err := json.Marshal(got[0])
		if err != nil {
			t.Fatal(err)
		}
		if longDigits.Match(raw) {
			t.Errorf("%q: the encoded finding holds a run of the number: %s", tc.text, raw)
		}
		if back := ProtoToFindings(FindingsToProto(got)); back[0].fp != 0 {
			t.Errorf("%q: the fingerprint crossed the wire", tc.text)
		}
	}
}

// A number written in fullwidth digits is read in the normalised view and
// reported where it stands.
func TestTRIdentifiers_NormalisedReading(t *testing.T) {
	text := "MERSIS " + fullwidth(testMersisLegal) + " kaydı"
	found := byCategory(scanPII(t, text), categoryMersis)
	if len(found) != 1 || !found[0].Placed() {
		t.Fatalf("fullwidth MERSİS number: %v", scanPII(t, text))
	}
	if masked := string(RedactContent([]byte(text), found)); masked != "MERSIS [mersis_no_REDACTED] kaydı" {
		t.Errorf("masked = %q", masked)
	}
}

// A foreigner's identity number (yabancı kimlik numarası) starts with 99
// and follows the TCKN check digits; it is found as tc_kimlik.
func TestForeignerIDIsFoundAsTCKimlik(t *testing.T) {
	const ykn = "99000000042"
	if !validTCKN(ykn) {
		t.Fatal("the test number must pass the check")
	}
	found := byCategory(scanPII(t, "yabancı kimlik no "+ykn), "tc_kimlik")
	if len(found) != 1 || found[0].Severity != "critical" {
		t.Fatalf("yabancı kimlik no: %v", found)
	}
}

func TestWordNear(t *testing.T) {
	for _, tc := range []struct {
		text string
		word string
		want bool
	}{
		{"MERSİS 123", "mersis", true},
		{"MERSIS 123", "mersis", true},
		{"mersis'e kayıtlı 123", "mersis", true},
		{"(plaka) 123", "plaka", true},
		{"template 123", "plate", false},
		{"şplaka 123", "plaka", false},
		{"1plate 123", "plate", false},
		{"123 plakalı", "plaka", true},
		{"123", "plaka", false},
	} {
		at := strings.Index(tc.text, "123")
		if got := wordNear(tc.text, at, at+3, []string{tc.word}); got != tc.want {
			t.Errorf("wordNear(%q, %q) = %v, want %v", tc.text, tc.word, got, tc.want)
		}
	}
	// A window cut inside a word does not make its tail a word.
	text := strings.Repeat("x", trWordWindowBefore-5) + "template 123"
	at := strings.Index(text, "123")
	if wordNear(text, at, at+3, []string{"plate"}) {
		t.Error("the tail of a word cut by the window was taken for a word")
	}
	if !anyStemFold("Mersİs", []string{"mers"}) || !anyStemFold("xIMEIx", []string{"imei"}) || anyStemFold("ime", []string{"imei"}) {
		t.Error("anyStemFold")
	}
}
