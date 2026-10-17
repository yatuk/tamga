// Package langid names the language a piece of prose is written in.
//
// The proxy asks through Detector, so that the library behind it can be
// replaced. The one in use is lingua-go in its low-accuracy mode, over all
// of its 75 languages. Why that one, and what it costs, is in
// docs/operations.md ("Languages a request may be written in"): of the two
// libraries measured it was the one that told Turkish written without
// Turkish letters from other languages, which whatlanggo did not in one
// short text out of two.
package langid

import (
	"sort"
	"strings"
	"sync"

	lingua "github.com/pemistahl/lingua-go"
)

// Guess is one language a detector holds a text may be written in.
type Guess struct {
	// Code is the language as an ISO 639-1 code, in lower case: "tr".
	Code string
	// Confidence is how much of the detector's belief the language holds,
	// from 0 to 1. The confidences of one answer add up to at most 1.
	Confidence float64
}

// Detector names the language of a text. It is safe for concurrent use.
type Detector interface {
	// Detect returns the languages text may be written in, most likely
	// first, at most MaxGuesses of them and none with no confidence at
	// all. It is empty when the detector cannot tell (no letters).
	Detect(text string) []Guess
}

// MaxGuesses bounds the answer of Detect.
const MaxGuesses = 5

// codes are the ISO 639-1 codes of the languages the detector can name.
// They are worked out from the library's list of languages, which loads no
// model, so that a policy can be checked without the cost of a detector.
var codes = func() map[string]bool {
	out := make(map[string]bool)
	for _, l := range lingua.AllLanguages() {
		out[strings.ToLower(l.IsoCode639_1().String())] = true
	}
	return out
}()

// Known reports whether code is an ISO 639-1 code the detector can name.
// Case is not significant.
func Known(code string) bool { return codes[strings.ToLower(strings.TrimSpace(code))] }

// Codes returns the codes Known accepts, sorted.
func Codes() []string {
	out := make([]string, 0, len(codes))
	for c := range codes {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}

var (
	defaultOnce sync.Once
	defaultDet  Detector
)

// Default returns the detector the proxy uses. It is built on the first
// call, which loads the models (about 50 MiB of memory, some tens of
// milliseconds); a proxy whose policy never asks for a language check never
// loads them.
func Default() Detector {
	defaultOnce.Do(func() { defaultDet = newLingua() })
	return defaultDet
}

// linguaDetector is Detector over lingua-go.
//
// Low-accuracy mode reads trigrams only. On the texts measured it was as
// right as the full mode for Turkish, English and the other languages
// tried, at a fifth of the memory (53 MiB against 990 MiB with every model
// loaded) and about half the time per call.
type linguaDetector struct {
	d lingua.LanguageDetector
}

func newLingua() Detector {
	d := lingua.NewLanguageDetectorBuilder().
		FromAllLanguages().
		WithLowAccuracyMode().
		WithPreloadedLanguageModels().
		Build()
	return linguaDetector{d: d}
}

func (l linguaDetector) Detect(text string) []Guess {
	var out []Guess
	for _, v := range l.d.ComputeLanguageConfidenceValues(text) {
		if len(out) == MaxGuesses || v.Value() <= 0 {
			break
		}
		out = append(out, Guess{Code: strings.ToLower(v.Language().IsoCode639_1().String()), Confidence: v.Value()})
	}
	return out
}
