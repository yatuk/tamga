package scanner

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/yatuk/tamga/internal/extract"
)

func largeRegistry() *Registry {
	r := NewRegistry()
	r.Register(NewPIIScanner())
	r.Register(NewSecretScanner())
	r.Register(NewInjectionScanner())
	r.Register(NewJailbreakScanner())
	return r
}

// codeText returns about n bytes of source-like text in which no two lines
// are the same, so no part of it is answered from the scan cache.
func codeText(n int) string {
	var b strings.Builder
	b.Grow(n + 128)
	for i := 0; b.Len() < n; i++ {
		b.WriteString("func handler")
		b.WriteString(strconv.Itoa(i))
		b.WriteString("(w http.ResponseWriter, r *http.Request) { serve(w, r, ")
		b.WriteString(strconv.Itoa(i * 7))
		b.WriteString(") }\n")
	}
	return b.String()[:n]
}

// An agent sends file contents and long tool output in one segment. Scan
// time has to grow with the size of the text, not with its square: before
// this was fixed, a megabyte did not finish in four minutes.
func TestScanSegments_LargeSegmentScalesLinearly(t *testing.T) {
	// As in the running proxy: phrase matching goes through the automaton.
	if err := InitDFA(); err != nil {
		t.Fatal(err)
	}
	reg := largeRegistry()
	// One worker, so the measurement is of the scanners and not of how many
	// cores the machine has.
	cfg := PipelineConfig{Mode: ModeWorkerPool}
	timeOf := func(n int) time.Duration {
		ResetScanCache()
		segs := []extract.Segment{{Role: extract.RoleTool, Kind: "tool_result", Path: "messages[0].content[0].text", Text: codeText(n)}}
		start := time.Now()
		if _, err := reg.ScanSegments(context.Background(), segs, cfg); err != nil {
			t.Fatalf("%d bytes: %v", n, err)
		}
		return time.Since(start)
	}
	timeOf(16 << 10) // warm up
	small, large := timeOf(64<<10), timeOf(512<<10)
	t.Logf("one core: 64 KB in %v, 512 KB in %v (x%.1f for x8 the text)", small, large, float64(large)/float64(small))
	// Linear would be x8. Allow slack for noise; quadratic would be x64.
	if float64(large) > 24*float64(small) {
		t.Fatalf("scan time grew x%.0f for x8 the text: not linear", float64(large)/float64(small))
	}
}

// With every core in use a megabyte of new text has to scan in about a
// second; an agent's request is otherwise held up for longer than the model
// takes to answer it.
func TestScanSegments_LargeSegmentInParallel(t *testing.T) {
	if err := InitDFA(); err != nil {
		t.Fatal(err)
	}
	reg := largeRegistry()
	ResetScanCache()
	segs := []extract.Segment{{Role: extract.RoleTool, Kind: "tool_result", Path: "messages[0].content[0].text", Text: codeText(1 << 20)}}
	start := time.Now()
	if _, err := reg.ScanSegments(context.Background(), segs, PipelineConfig{}); err != nil {
		t.Fatal(err)
	}
	took := time.Since(start)
	t.Logf("1 MB of new text, all cores: %v", took)
	// Generous: CI has a few cores and runs with the race detector.
	if took > 60*time.Second {
		t.Fatalf("1 MB of text took %v to scan", took)
	}
}
