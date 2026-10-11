package scanner

import (
	"context"
	"strings"
	"testing"
)

// What the Turkish rules cost on Turkish text, where their gate words are
// in nearly every sentence, and on English text that holds the few gate
// words the two languages share ("model", "system", "rules", an address).
func BenchmarkTurkishRules(b *testing.B) {
	turkish := strings.Repeat("Müdürün talimatı doğrultusunda raporu cuma gününe kadar teslim edeceğiz; kuralları ve yönergeleri ekte bulabilirsiniz. ", 34)
	english := strings.Repeat("The model follows the system rules of the team; write to dev@example.invalid if the mode is unclear. ", 40)
	for _, tc := range []struct{ name, text string }{
		{"turkish_prose_4KB", turkish},
		{"english_shared_words_4KB", english},
	} {
		content := []byte(tc.text)
		b.Run(tc.name, func(b *testing.B) {
			s := NewJailbreakScanner()
			b.SetBytes(int64(len(content)))
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := s.Scan(context.Background(), content); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
