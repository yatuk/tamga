package scanner

import "testing"

// FuzzValidLuhn ensures validLuhn never panics on arbitrary input.
func FuzzValidLuhn(f *testing.F) {
	// Seed corpus
	f.Add("4532015112830366")     // Valid Visa
	f.Add("5555555555554444")     // Valid Mastercard
	f.Add("378282246310005")      // Valid Amex
	f.Add("")                     // Empty
	f.Add("abc")                  // Non-numeric
	f.Add("1")                    // Too short
	f.Add("12345678901234567890") // Too long

	f.Fuzz(func(t *testing.T, s string) {
		// Must never panic
		_ = validLuhn(s)
	})
}

// FuzzValidTCKN ensures validTCKN never panics on arbitrary input.
func FuzzValidTCKN(f *testing.F) {
	f.Add("10000000146")  // Ataturk
	f.Add("11111111110")  // Sequential
	f.Add("")             // Empty
	f.Add("abc")          // Non-numeric
	f.Add("1")            // Too short
	f.Add("123456789012") // Too long

	f.Fuzz(func(t *testing.T, s string) {
		_ = validTCKN(s)
	})
}

// FuzzMaskPAN ensures maskPAN never panics on arbitrary input.
func FuzzMaskPAN(f *testing.F) {
	f.Add("4532015112830366")
	f.Add("")                    // Empty
	f.Add("a")                   // Non-numeric
	f.Add("1")                   // Single digit
	f.Add("1234567890123456")    // 16-digit without dashes
	f.Add("1234-5678-9012-3456") // With dashes

	f.Fuzz(func(t *testing.T, s string) {
		result := maskPAN(s)
		// maskPAN must never return a longer result than the number of digits
		if len(result) > len(digitsOnly(s)) && len(digitsOnly(s)) >= 13 {
			t.Errorf("masked PAN length %d exceeds input digit count %d for valid-length PAN", len(result), len(digitsOnly(s)))
		}
		_ = result
	})
}

// FuzzValidIBAN ensures validIBAN never panics.
func FuzzValidIBAN(f *testing.F) {
	f.Add("TR330006100519786457841326") // Valid Turkish IBAN
	f.Add("GB29NWBK60161331926819")     // Valid UK IBAN
	f.Add("")                           // Empty
	f.Add("abc")                        // Non-numeric

	f.Fuzz(func(t *testing.T, s string) {
		_ = validIBAN(s)
	})
}
