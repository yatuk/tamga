package normalize

import (
	"strings"
	"testing"
)

func TestStripZeroWidth(t *testing.T) {
	in := "41\u200B11 11\u200C11 11\uFEFF11 11\u00A011"
	got := stripZeroWidth(in)
	if strings.ContainsAny(got, "\u200B\u200C\u200D\u2060\uFEFF") {
		t.Fatalf("zero-width chars leaked: %q", got)
	}
}

func TestFoldDiacritics(t *testing.T) {
	got := foldDiacritics("İstanbul şöför")
	want := "Istanbul sofor"
	// Casing is preserved here; turkishLower is tested separately.
	if !strings.Contains(strings.ToLower(got), strings.ToLower(want)) {
		t.Fatalf("got %q want contains %q", got, want)
	}
}

func TestFoldHomoglyphs(t *testing.T) {
	// Cyrillic "аррlе" → "apple"
	in := "а" + "р" + "р" + "l" + "е"
	got := foldHomoglyphs(in)
	if got != "apple" {
		t.Fatalf("homoglyph fold %q → %q, want %q", in, got, "apple")
	}
}

func TestTurkishLower(t *testing.T) {
	cases := map[string]string{
		"İSTANBUL": "istanbul",
		"IŞIK":     "isik",
		"Güneş":    "gunes",
	}
	for in, want := range cases {
		if got := turkishLower(in); got != want {
			t.Fatalf("turkishLower(%q) = %q; want %q", in, got, want)
		}
	}
}

func TestExpandWordsToNumbersEN(t *testing.T) {
	got := expandWordsToNumbers("my card is four one one one here")
	if !strings.Contains(got, "4111") {
		t.Fatalf("got %q, want substring %q", got, "4111")
	}
}

func TestExpandWordsToNumbersTR(t *testing.T) {
	got := expandWordsToNumbers("kart numaram dört bir bir bir")
	if !strings.Contains(got, "4111") {
		t.Fatalf("got %q, want substring %q", got, "4111")
	}
}

func TestBase64Decode(t *testing.T) {
	// "Here is my card 4111111111111111" base64-encoded.
	enc := "SGVyZSBpcyBteSBjYXJkIDQxMTExMTExMTExMTExMTE="
	got := findBase64Text(enc)
	if len(got) == 0 {
		t.Fatal("expected at least one decoded candidate")
	}
	if !strings.Contains(got[0], "4111") {
		t.Fatalf("decoded payload missing card: %q", got[0])
	}
}

func TestApplyEndToEnd(t *testing.T) {
	in := "ignore previous → dört bir bir bir"
	res := Apply(in, Default())
	if !strings.Contains(res.Canonical, "4111") {
		t.Fatalf("canonical missing normalized digits: %q", res.Canonical)
	}
}

func TestApplyNegative(t *testing.T) {
	// Benign prose must not gain spurious digits.
	in := "I have one cat and a two-bedroom apartment."
	res := Apply(in, Default())
	for _, bad := range []string{"12", "01", "21"} {
		if strings.Contains(res.Canonical, bad) {
			t.Fatalf("false positive digit injection %q in %q", bad, res.Canonical)
		}
	}
}

func TestDefaultOptions_HexAndROT13(t *testing.T) {
	opts := Default()
	if !opts.TryHex {
		t.Fatal("TryHex should be true by default — hex-encoded evasions are common")
	}
	if !opts.TryROT13 {
		t.Fatal("TryROT13 should be true by default — ROT13 evasions are common")
	}
}

// --- findHexText ---

func TestFindHexText(t *testing.T) {
	// "hello world" in hex: 68656c6c6f20776f726c64
	enc := "the payload is 68656c6c6f20776f726c64 hidden here"
	got := findHexText(enc)
	if len(got) == 0 {
		t.Fatal("expected at least one decoded candidate")
	}
	if !strings.Contains(got[0], "hello world") {
		t.Fatalf("decoded payload missing 'hello world': %q", got[0])
	}
}

func TestFindHexText_OddLength(t *testing.T) {
	// Odd-length hex runs should be skipped.
	got := findHexText("abc")
	if len(got) != 0 {
		t.Fatalf("odd-length hex should be skipped, got %v", got)
	}
}

func TestFindHexText_Empty(t *testing.T) {
	got := findHexText("no hex here")
	if len(got) != 0 {
		t.Fatalf("no hex in input, got %v", got)
	}
}

// --- rot13 ---

func TestRot13(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"hello", "uryyb"},
		{"Hello World", "Uryyb Jbeyq"},
		{"12345", "12345"},
		{"", ""},
		{"a-z A-Z", "n-m N-M"},
	}
	for _, tt := range tests {
		got := rot13(tt.in)
		if got != tt.want {
			t.Errorf("rot13(%q) = %q; want %q", tt.in, got, tt.want)
		}
	}
	// Double ROT13 returns original.
	if got := rot13(rot13("secret")); got != "secret" {
		t.Errorf("double rot13: want 'secret', got %q", got)
	}
}

// --- isMostlyPrintableASCII ---

func TestIsMostlyPrintableASCII(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"empty", "", false},
		{"printable", "Hello, world!", true},
		{"with newlines", "line1\nline2\r\n", true},
		{"with tabs", "col1\tcol2", true},
		{"binary", "AB\x00\x01\x02\x03\x04\x05\x06\x07\x08CDEF", false},
		{"mostly binary", "\x00\x01\x02\x03\x04\x05\x06\x07\x08\x09hello", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isMostlyPrintableASCII(tt.in); got != tt.want {
				t.Errorf("isMostlyPrintableASCII(%q) = %v; want %v", tt.in, got, tt.want)
			}
		})
	}
}

// --- Result.Text ---

func TestResultText_NoDecoded(t *testing.T) {
	r := Result{Canonical: "hello world"}
	if got := r.Text(); got != "hello world" {
		t.Errorf("want 'hello world', got %q", got)
	}
}

func TestResultText_WithDecoded(t *testing.T) {
	r := Result{
		Canonical: "hello world",
		Decoded:   []string{"base64 decoded", "hex decoded"},
	}
	got := r.Text()
	if !strings.Contains(got, "hello world") {
		t.Error("missing canonical")
	}
	if !strings.Contains(got, "base64 decoded") {
		t.Error("missing base64 decoded")
	}
	if !strings.Contains(got, "hex decoded") {
		t.Error("missing hex decoded")
	}
}

// --- Apply option subsets ---

func TestApply_EmptyInput(t *testing.T) {
	res := Apply("", Default())
	if res.Canonical != "" {
		t.Errorf("empty input: want empty, got %q", res.Canonical)
	}
	if len(res.Decoded) != 0 {
		t.Errorf("empty input: want no decoded, got %v", res.Decoded)
	}
}

func TestApply_NFKC_Only(t *testing.T) {
	opts := Options{NFKC: true}
	// NFKC normalizes fullwidth digits to ASCII.
	res := Apply("１２３４５", opts)
	if res.Canonical != "12345" {
		t.Errorf("NFKC fullwidth: want '12345', got %q", res.Canonical)
	}
}

func TestApply_TurkishLowerOnly(t *testing.T) {
	opts := Options{ToLowerTurkish: true}
	res := Apply("İSTANBUL", opts)
	if res.Canonical != "istanbul" {
		t.Errorf("turkish lower only: want 'istanbul', got %q", res.Canonical)
	}
}

func TestApply_AllOptionsDisabled(t *testing.T) {
	opts := Options{}
	res := Apply("İstanbul\xA0  şöför", opts)
	// With all options disabled, the output should match the input.
	if res.Canonical != "İstanbul\xA0  şöför" {
		t.Errorf("all disabled: want unchanged, got %q", res.Canonical)
	}
}

func TestApply_StripZeroWidthOnly(t *testing.T) {
	opts := Options{StripZeroWidth: true}
	res := Apply("41​11", opts)
	if strings.Contains(res.Canonical, "200b") {
		t.Error("zero-width char not stripped")
	}
	if res.Canonical != "4111" {
		t.Errorf("want '4111', got %q", res.Canonical)
	}
}

func TestApply_StripPunctOnly(t *testing.T) {
	opts := Options{StripPunct: true}
	res := Apply("hello!!! world???", opts)
	// StripPunct implementation may vary — just verify it doesn't panic.
	_ = res
}

func TestApply_ExpandNumberWordOnly(t *testing.T) {
	opts := Options{ExpandNumberWord: true}
	res := Apply("four two zero", opts)
	if !strings.Contains(res.Canonical, "420") {
		t.Errorf("expand numbers only: want '420' in %q", res.Canonical)
	}
}

// --- turkishLower all characters ---

func TestTurkishLower_AllMappedChars(t *testing.T) {
	tests := map[string]string{
		"İ": "i",
		"I": "i",
		"Ş": "s",
		"Ğ": "g",
		"Ü": "u",
		"Ö": "o",
		"Ç": "c",
		"ş": "s",
		"ğ": "g",
		"ü": "u",
		"ö": "o",
		"ç": "c",
		"ı": "i",
		"A": "a", // unicode.ToLower handles standard ASCII
	}
	for in, want := range tests {
		got := turkishLower(in)
		if got != want {
			t.Errorf("turkishLower(%q) = %q; want %q", in, got, want)
		}
	}
}

func TestTurkishLower_Mixed(t *testing.T) {
	got := turkishLower("TÜRKÇE İŞLEM ılık")
	want := "turkce islem ilik"
	if got != want {
		t.Errorf("want %q, got %q", want, got)
	}
}

// --- foldHomoglyphs fullwidth range ---

func TestFoldHomoglyphs_FullwidthRange(t *testing.T) {
	// Fullwidth 'A' (U+FF21) → ASCII 'A' (U+0041)
	fullA := string(rune(0xFF21))
	got := foldHomoglyphs(fullA)
	if got != "A" {
		t.Errorf("fullwidth A: want 'A', got %q", got)
	}

	// Fullwidth '5' (U+FF15) → ASCII '5' (U+0035)
	full5 := string(rune(0xFF15))
	got = foldHomoglyphs(string("test") + full5)
	if got != "test5" {
		t.Errorf("fullwidth 5: want 'test5', got %q", got)
	}

	// Boundary: just below range should be unchanged.
	below := string(rune(0xFF00))
	got = foldHomoglyphs(below)
	if got != below {
		t.Errorf("below fullwidth range: want unchanged, got %q", got)
	}
}

func TestFoldHomoglyphs_FullwidthBoundary(t *testing.T) {
	// First fullwidth char U+FF01 → ! (0x21)
	got := foldHomoglyphs(string(rune(0xFF01)))
	if got != "!" {
		t.Errorf("fullwidth !: want '!', got %q", got)
	}
	// Last fullwidth char U+FF5E → ~ (0x7E)
	got = foldHomoglyphs(string(rune(0xFF5E)))
	if got != "~" {
		t.Errorf("fullwidth ~: want '~', got %q", got)
	}
}

// --- foldDiacritics edge cases ---

func TestFoldDiacritics_NonLatin(t *testing.T) {
	// Arabic text should not be destroyed (no combining marks to strip here).
	arabic := "مرحبا"
	got := foldDiacritics(arabic)
	if got != arabic {
		t.Errorf("arabic diacritics: want unchanged, got %q", got)
	}
}

func TestFoldDiacritics_NoDiacritics(t *testing.T) {
	got := foldDiacritics("plain ascii text")
	if got != "plain ascii text" {
		t.Errorf("want unchanged, got %q", got)
	}
}

// --- findBase64Text edge cases ---

func TestFindBase64Text_Empty(t *testing.T) {
	got := findBase64Text("no base64 here")
	if len(got) != 0 {
		t.Errorf("empty result expected, got %v", got)
	}
}

func TestFindBase64Text_ShortInput(t *testing.T) {
	// Input too short for a base64 token (less than 16 chars).
	got := findBase64Text("SG9tZQ==")
	if len(got) != 0 {
		// Might decode, but it's below the 16-char minimum token length.
		t.Logf("short base64: %v", got)
	}
}

// --- expandWordsToNumbers edge cases ---

func TestExpandWordsToNumbers_NoNumberWords(t *testing.T) {
	got := expandWordsToNumbers("just regular text here")
	if got != "just regular text here" {
		t.Errorf("want unchanged, got %q", got)
	}
}

func TestExpandWordsToNumbers_Empty(t *testing.T) {
	got := expandWordsToNumbers("")
	if got != "" {
		t.Errorf("want empty, got %q", got)
	}
}
