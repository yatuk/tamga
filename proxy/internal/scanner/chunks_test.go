package scanner

import (
	"context"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/yatuk/tamga/internal/extract"
)

func TestSplitChunks(t *testing.T) {
	short := "a short message"
	if got := splitChunks(short); len(got) != 1 || got[0] != (chunk{0, len(short)}) {
		t.Fatalf("short text: %v", got)
	}

	text := strings.Repeat("kelime ğüşiöç word 12345 ", 20000) // ~560 KB, multi-byte runes
	chunks := splitChunks(text)
	if len(chunks) < 10 {
		t.Fatalf("got %d chunks", len(chunks))
	}
	prevEnd := 0
	for i, c := range chunks {
		if c.end-c.start > chunkSize {
			t.Fatalf("chunk %d is %d bytes", i, c.end-c.start)
		}
		if !utf8.ValidString(text[c.start:c.end]) {
			t.Fatalf("chunk %d cuts a UTF-8 sequence", i)
		}
		if i > 0 {
			if c.start >= prevEnd {
				t.Fatalf("chunk %d does not overlap the one before: start %d, previous end %d", i, c.start, prevEnd)
			}
			if c.start > 0 && text[c.start-1] != ' ' {
				t.Fatalf("chunk %d starts inside a word", i)
			}
		}
		prevEnd = c.end
	}
	if chunks[0].start != 0 || chunks[len(chunks)-1].end != len(text) {
		t.Fatal("the chunks do not cover the text")
	}

	// One token longer than a chunk: no whitespace to cut at. It must
	// still terminate and cover everything.
	blob := strings.Repeat("x", 3*chunkSize+17)
	bc := splitChunks(blob)
	if bc[len(bc)-1].end != len(blob) {
		t.Fatal("an unbroken run is not covered")
	}
}

func scanOneSegment(t *testing.T, text string) []Finding {
	t.Helper()
	segs := []extract.Segment{{Role: extract.RoleTool, Kind: "tool_result", Path: "messages[0].content", Text: text}}
	found, err := largeRegistry().ScanSegments(context.Background(), segs, PipelineConfig{})
	if err != nil {
		t.Fatal(err)
	}
	return found
}

// What is found, and where, must not depend on how the text was cut.
func TestScanSegments_ValueDeepInALongSegment(t *testing.T) {
	ResetScanCache()
	filler := codeText(300 << 10)
	const id = "10000000146"
	const key = "AKIAIOSFODNN7EXAMPLE"
	text := filler + "\nTC kimlik no: " + id + "\n" + filler + "\naws key " + key + "\n" + filler

	found := scanOneSegment(t, text)
	var gotID, gotKey int
	for _, f := range found {
		if !f.Placed() {
			continue
		}
		switch text[f.StartPos:f.EndPos] {
		case id:
			gotID++
			if f.Role != "tool" || f.Seg != 0 || f.Path == "" {
				t.Fatalf("finding lost its place: %+v", f)
			}
		case key:
			gotKey++
		}
	}
	if gotID != 1 || gotKey != 1 {
		t.Fatalf("ID found %d times, key %d times, want once each (%d findings)", gotID, gotKey, len(found))
	}
}

// A value that lies across a cut is whole in the overlap of one chunk, and
// is reported once although two chunks contain it.
func TestScanSegments_ValueOnAChunkBoundary(t *testing.T) {
	ResetScanCache()
	const id = "10000000146"
	// Find where the first cut falls for plain filler, then put the value
	// across it.
	filler := strings.Repeat("word ", (2*chunkSize)/5)
	cut := splitChunks(filler)[0].end
	for _, shift := range []int{-5, -1, 0, 3} {
		at := cut + shift - len(id)/2
		text := filler[:at] + " " + id + " " + filler[at:]
		n := 0
		for _, f := range scanOneSegment(t, text) {
			if f.Placed() && text[f.StartPos:f.EndPos] == id {
				n++
			}
		}
		if n != 1 {
			t.Fatalf("shift %d: the ID across the cut was found %d times, want 1", shift, n)
		}
	}
}

// An agent resends the same long text every turn.
func TestScanCache_RepeatedTextIsNotScannedAgain(t *testing.T) {
	ResetScanCache()
	text := codeText(512<<10) + "\nTC kimlik no: 10000000146\n"

	start := time.Now()
	first := scanOneSegment(t, text)
	cold := time.Since(start)
	h0, _ := ScanCacheStats()

	start = time.Now()
	second := scanOneSegment(t, text)
	warm := time.Since(start)
	h1, _ := ScanCacheStats()

	if h1 == h0 {
		t.Fatal("the second scan did not use the cache")
	}
	if len(first) != len(second) || len(first) == 0 {
		t.Fatalf("findings differ: %d then %d", len(first), len(second))
	}
	for i := range first {
		if first[i].Category != second[i].Category || first[i].StartPos != second[i].StartPos || first[i].Role != second[i].Role {
			t.Fatalf("finding %d differs: %+v vs %+v", i, first[i], second[i])
		}
	}
	t.Logf("512 KB: first scan %v, repeat %v", cold, warm)
	if warm > cold/3 {
		t.Fatalf("the repeat took %v against %v cold", warm, cold)
	}
}

// What a caller does to the findings it got must not change what the next
// caller gets.
func TestScanCache_ReturnsCopies(t *testing.T) {
	ResetScanCache()
	text := codeText(8<<10) + "\nTC kimlik no: 10000000146\n"
	first := scanOneSegment(t, text)
	if len(first) == 0 {
		t.Fatal("no findings")
	}
	for i := range first {
		first[i].Category = "tampered"
		first[i].StartPos = -99
		if first[i].ConfidenceScore != nil {
			first[i].ConfidenceScore.Total = -1
		}
	}
	for _, f := range scanOneSegment(t, text) {
		if f.Category == "tampered" || f.StartPos == -99 || (f.ConfidenceScore != nil && f.ConfidenceScore.Total == -1) {
			t.Fatalf("a cached finding was changed by a caller: %+v", f)
		}
	}
}

// Short text is not cached at all, and a scanner whose answer depends on
// more than the text never is.
func TestScanCache_OnlyLongTextAndPureScanners(t *testing.T) {
	ResetScanCache()
	h0, m0 := ScanCacheStats()
	scanOneSegment(t, "TC kimlik no: 10000000146")
	scanOneSegment(t, "TC kimlik no: 10000000146")
	if h1, m1 := ScanCacheStats(); h1 != h0 || m1 != m0 {
		t.Fatal("a short message went through the cache")
	}
	if _, pure := pureScannerName(NewCustomScanner(func() []CustomEntitySpec { return nil })); pure {
		t.Fatal("the custom scanner depends on patterns edited at runtime and must not be cached")
	}
}
