package proxy

import (
	"strings"
	"testing"
)

// A value longer than the fixed hold-back used to leave part of itself on
// the wire. An e-mail address is only an address once the "@" and a domain
// have arrived, and by then the start of a long local part had been
// released; and it was masked as soon as "name@mail.example" read as an
// address, so the ".com" that came next went out after the marker. With the
// fixed hold-back alone this text arrived as
// "muhasebe.v[email_REDACTED].com". The run of characters a value is
// written in is now held until it ends, and a finding inside it is not
// acted on before then.
func TestStreamGuard_LongAddressSplitAnywhereIsRedactedWhole(t *testing.T) {
	// 62 characters before the "@": within what an address may have, and
	// with the domain more than the 64 the guard always keeps back.
	const local = "muhasebe.ve.finans.departmani.fatura.bildirimleri.avrupa.yakasi"
	const full = "Faturayı şu adrese gönderin: " + local + "@faturalar.example.com ve bir kopyasını saklayın, lütfen."
	const want = "Faturayı şu adrese gönderin: [email_REDACTED] ve bir kopyasını saklayın, lütfen."

	for size := 1; size <= 48; size++ {
		var events [][]byte
		for _, p := range pieces(full, size) {
			events = append(events, openAIChunk(p))
		}
		events = append(events, openAIFinish, openAIDone)

		wire, text, stopped := run(t, scanningGuard(), events...)
		if stopped {
			t.Fatalf("pieces of %d: the stream was stopped", size)
		}
		for _, part := range []string{"muhasebe", "departmani", "yakasi@", "example.com"} {
			if strings.Contains(wire, part) {
				t.Fatalf("pieces of %d: %q of the address is on the wire:\n%s", size, part, wire)
			}
		}
		if text != want {
			t.Fatalf("pieces of %d:\n got %q\nwant %q", size, text, want)
		}
	}
}

// The same for a value that ends the stream: nothing of it goes out first.
func TestStreamGuard_TCKNSplitAnywhereNothingOfItIsSent(t *testing.T) {
	const full = "Kayıtlara göre müşterinin TC kimlik numarası 10000000146 olarak görünüyor."
	for size := 1; size <= 12; size++ {
		var events [][]byte
		for _, p := range pieces(full, size) {
			events = append(events, anthropicDelta(0, p))
		}
		events = append(events, anthropicStop(0))
		wire, text, stopped := run(t, scanningGuard(), events...)
		if !stopped {
			t.Fatalf("pieces of %d: the stream was not stopped", size)
		}
		if strings.Contains(text, "1000") || strings.Contains(wire, "10000000146") {
			t.Fatalf("pieces of %d: digits of the number were sent: %q", size, text)
		}
	}
}

// Eleven digits that are a valid TC identity number, followed in the next
// event by more digits: the number in the text is another one. A finding
// that reached the end of what had arrived used to end the stream there.
func TestStreamGuard_BlockedValueThatGoesOnIsNotBlocked(t *testing.T) {
	const full = "Sipariş takip kodunuz 1000000014678901234 olarak kaydedildi, kargoya verildiğinde haber vereceğiz."
	for size := 1; size <= 16; size++ {
		var events [][]byte
		for _, p := range pieces(full, size) {
			events = append(events, anthropicDelta(0, p))
		}
		events = append(events, anthropicStop(0))
		_, text, stopped := run(t, scanningGuard(), events...)
		if stopped {
			t.Fatalf("pieces of %d: the stream was stopped", size)
		}
		if text != full {
			t.Fatalf("pieces of %d:\n got %q\nwant %q", size, text, full)
		}
	}
}

// A paragraph in a script written without spaces is not one run: it would
// be held until its end.
func TestOpenRunStart(t *testing.T) {
	for _, tt := range []struct {
		text  string
		limit int
		want  int
	}{
		{"", 16, 0},
		{"one two", 16, 4},
		{"one two ", 16, 8},
		{"one\ntwo", 16, 4},
		{"abcdefghij", 4, -1},
		{"abcd", 4, 0},
		{"x 東京都", 16, len("x 東京都")},
		{"x 東京都abc", 16, len("x 東京都")},
		{"ไทยabc", 16, len("ไทย")},
		{"çay şeker", 16, len("çay ")},
	} {
		if got := openRunStart(tt.text, tt.limit); got != tt.want {
			t.Errorf("openRunStart(%q, %d) = %d, want %d", tt.text, tt.limit, got, tt.want)
		}
	}
}
