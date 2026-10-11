package scanner

import (
	"strconv"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// TestPropertyLuhn_ChecksumInvariant verifies that valid Luhn numbers
// always satisfy sum%10==0, and that changing any single digit breaks validation.
func TestPropertyLuhn_ChecksumInvariant(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a valid 16-digit PAN
		panLen := rapid.IntRange(13, 19).Draw(t, "panLen")
		digits := make([]byte, panLen)

		// Generate first n-1 digits randomly
		checkSum := 0
		alt := true // parity for even-length
		// For double-and-add, we work right-to-left.
		// We'll fill digits[0:panLen-1] and compute the check digit.
		for i := panLen - 2; i >= 0; i-- {
			d := rapid.IntRange(0, 9).Draw(t, "digit")
			v := d
			if alt {
				v *= 2
				if v > 9 {
					v -= 9
				}
			}
			checkSum += v
			digits[i] = byte('0' + d)
			alt = !alt
		}
		checkDigit := (10 - (checkSum % 10)) % 10
		digits[panLen-1] = byte('0' + checkDigit)

		number := string(digits)

		// Invariant 1: Generated number must pass Luhn
		if !validLuhn(number) {
			t.Errorf("generated Luhn number %q failed validation", number)
		}

		// Invariant 2: Changing any single digit must fail Luhn
		for pos := 0; pos < len(number); pos++ {
			orig := digits[pos]
			for newD := 0; newD <= 9; newD++ {
				if byte('0'+newD) == orig {
					continue
				}
				mutated := []byte(number)
				mutated[pos] = byte('0' + newD)
				if validLuhn(string(mutated)) {
					t.Errorf("mutated Luhn number %q still passes (changed position %d from %c to %c)",
						string(mutated), pos, orig, mutated[pos])
				}
			}
		}
	})
}

// TestPropertyTCKN_ChecksumInvariant verifies the TCKN checksum algorithm.
func TestPropertyTCKN_ChecksumInvariant(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a valid 11-digit TCKN
		// d[0] must not be zero
		d := make([]int, 11)
		d[0] = rapid.IntRange(1, 9).Draw(t, "d0")

		// Generate d[1]..d[8] randomly
		for i := 1; i <= 8; i++ {
			d[i] = rapid.IntRange(0, 9).Draw(t, "d"+strconv.Itoa(i))
		}

		// Compute check digits
		odd := d[0] + d[2] + d[4] + d[6] + d[8]
		even := d[1] + d[3] + d[5] + d[7]
		d[9] = ((odd*7-even)%10 + 10) % 10

		sum10 := 0
		for i := 0; i < 10; i++ {
			sum10 += d[i]
		}
		d[10] = sum10 % 10

		// Build string
		tckn := ""
		for _, v := range d {
			tckn += strconv.Itoa(v)
		}

		// Invariant 1: Generated TCKN must pass validation
		if !validTCKN(tckn) {
			t.Errorf("generated TCKN %q failed validation", tckn)
		}

		// Invariant 2: Leading zero must fail
		zeroPrefixed := "0" + tckn[1:]
		if validTCKN(zeroPrefixed) {
			t.Errorf("zero-prefixed TCKN %q should have failed", zeroPrefixed)
		}

		// Invariant 3: Wrong length must fail
		if validTCKN(tckn[:10]) {
			t.Errorf("10-digit TCKN %q should have failed", tckn[:10])
		}
		if validTCKN(tckn + "0") {
			t.Errorf("12-digit TCKN %q should have failed", tckn+"0")
		}
	})
}

// TestPropertyMaskPAN_Invariant verifies maskPAN always preserves
// the number of digits and the last four, and shows no other digit.
func TestPropertyMaskPAN_Invariant(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		panLen := rapid.IntRange(13, 19).Draw(t, "panLen")
		digits := make([]byte, panLen)
		for i := range digits {
			digits[i] = byte('0' + rapid.IntRange(0, 9).Draw(t, "digit"))
		}
		pan := string(digits)

		masked := maskPAN(pan)

		// Invariant 1: one character per digit
		origDigits := digitsOnly(pan)
		if len(masked) != len(origDigits) {
			t.Fatalf("maskPAN(%q) = %q (len %d vs original numeric len %d)",
				pan, masked, len(masked), len(origDigits))
		}
		// Invariant 2: nothing before the last four is a digit
		if head := masked[:len(masked)-4]; head != strings.Repeat("*", len(head)) {
			t.Errorf("maskPAN(%q) shows more than the last four: %q", pan, masked)
		}
		// Invariant 3: Last 4 digits preserved
		if masked[len(masked)-4:] != origDigits[len(origDigits)-4:] {
			t.Errorf("maskPAN last 4 digits mismatch: %q vs %q",
				masked[len(masked)-4:], origDigits[len(origDigits)-4:])
		}
	})
}
