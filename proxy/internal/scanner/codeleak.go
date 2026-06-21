package scanner

import (
	"context"
	"regexp"
	"strings"
)

// ──────────────────────────────────────────────────────────────────────────────
// CodeLeakScanner — detects source code leaked in LLM responses (output-only).
// ──────────────────────────────────────────────────────────────────────────────
//
// Detection phases (ordered from cheapest to most expensive):
//   1. Shebang lines (single regex)
//   2. Multi-line code fences with language hints
//   3. Standalone import/require statements (line-start anchored)
//   4. Function and class definitions
//   5. SQL DDL/DML in code context
//
// Confidence scoring:
//   - 1-2 suspicious lines:  0.40
//   - 3+ suspicious lines OR shebang+import:  0.70
//   - Full function/class body (3+ lines inside fence): 0.90
//
// All standalone pattern regexes are line-start-anchored ((?m)^\s*…) so that
// inline prose mentions (e.g. "In Python you use `import os` to …") are NOT
// flagged.

// ──────────────────────────── Pre-compiled patterns ────────────────────────────

var (
	// Phase 1: shebang — cheapest check, single regex.
	shebangRe = regexp.MustCompile(`(?m)^#![^\r\n]*`)

	// Phase 2: code fence opening. Captures the language hint in group 1.
	fenceOpenRe = regexp.MustCompile("(?m)^`{3}([a-zA-Z][a-zA-Z0-9_]*)[^\n]*$")

	// Phase 2: code fence closing — a line that is only ``` with optional whitespace.
	fenceCloseRe = regexp.MustCompile("(?m)^`{3}\\s*$")

	// Phase 3: import / require patterns (standalone lines only — ^ anchored).
	// Each pattern is designed to minimize cross-language overlap. The Java
	// pattern requires at least one dot in the package path (e.g. java.util.*)
	// to avoid matching bare Python "import os".
	importPatterns = []*regexp.Regexp{
		// Python: import os   or   from django.db import models
		regexp.MustCompile(`(?m)^\s*(?:import\s+\w+|from\s+[\w.]+\s+import\s+\w)`),
		// JavaScript / TypeScript: import { ... } from '...'   or   require('...')
		regexp.MustCompile(`(?m)^\s*(?:import\s+.*?\s+from\s+['"\x60]|require\s*\(\s*['"\x60])`),
		// Go: import "fmt"  or  import (
		regexp.MustCompile(`(?m)^\s*import\s+(?:"[\w./\-]+"|\()`),
		// Java: requires at least one dot (package path) — import java.util.*;
		regexp.MustCompile(`(?m)^\s*import\s+(?:static\s+)?[\w]+\.[\w.*]+`),
	}

	// Phase 4: function / class definition patterns (standalone lines only).
	funcClassPatterns = []*regexp.Regexp{
		// Python function: def name(args):
		regexp.MustCompile(`(?m)^\s*def\s+\w+\s*\(`),
		// Python class: class Name(...):  or  class Name:
		regexp.MustCompile(`(?m)^\s*class\s+\w+\s*[(:{]`),
		// Go function: func Name(  or  func (r *Receiver) Name(
		regexp.MustCompile(`(?m)^\s*func\s+(?:\(\w+\s+\*?\w+\)\s+)?\w+\s*\(`),
		// Java / C# / TypeScript class: class Foo  or  public class Foo
		regexp.MustCompile(`(?m)^\s*(?:public\s+)?(?:class|interface|enum)\s+\w+`),
		// Java-style method: public static void main(  or  public void foo(
		regexp.MustCompile(`(?m)^\s*(?:public|private|protected)\s+(?:static\s+)?\w+\s+\w+\s*\(`),
		// JavaScript function: function name(  or  async function
		regexp.MustCompile(`(?m)^\s*(?:async\s+)?function\s+\w+\s*\(`),
	}

	// Phase 5: SQL — dangerous DDL/DML matched anywhere in the text.
	// SELECT ... FROM uses line-start anchor to reduce false positives from
	// prose like "to select something from the menu".
	sqlDDLPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?im)\b(?:CREATE\s+TABLE|DROP\s+TABLE|ALTER\s+TABLE)\s+\w+`),
		regexp.MustCompile(`(?im)\b(?:INSERT\s+INTO\s+\w+|UPDATE\s+\w+\s+SET|DELETE\s+FROM\s+\w+)`),
		regexp.MustCompile(`(?im)^\s*SELECT\s+.*?\s+FROM\s+\w+`),
	}
)

// ────────────────────────────── Fence block data ──────────────────────────────

// fenceBlock holds a single extracted code-fence block with its byte range,
// line count, and whether it contains function/class/import definitions.
type fenceBlock struct {
	startByte  int // byte offset of the opening ``` line start (inclusive)
	endByte    int // byte offset of the closing ``` line end (exclusive)
	innerStart int // byte offset of first content byte after opening line
	innerEnd   int // byte offset of last content byte before closing line
	lineCount  int
	hasFuncDef bool
	hasImport  bool
}

// ──────────────────────────────── Scanner struct ────────────────────────────────

// CodeLeakScanner detects source code patterns in LLM output text.
// It is designed to be registered for output-only scanning.
type CodeLeakScanner struct{}

// NewCodeLeakScanner creates an output-only code leak detector.
func NewCodeLeakScanner() *CodeLeakScanner {
	return &CodeLeakScanner{}
}

// Name returns the canonical scanner name used in policy rules ("code_leak").
func (s *CodeLeakScanner) Name() string { return "code_leak" }

// Scan runs source code detection on the provided content and returns zero or
// more findings. When no code patterns are present, the findings slice is empty.
func (s *CodeLeakScanner) Scan(ctx context.Context, content []byte) ([]Finding, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	contentStr := string(content)
	if len(strings.TrimSpace(contentStr)) == 0 {
		return nil, nil
	}

	// ── Phase 1: shebang (cheapest) ──
	hasShebang := shebangRe.Match(content)

	// ── Phase 2: extract code-fence blocks ──
	blocks := extractFenceBlocks(contentStr)

	// Calculate totals from fence blocks.
	fenceLines := 0
	fenceHasFuncClass := false
	fenceHasImportCount := 0
	for _, b := range blocks {
		fenceLines += b.lineCount
		if b.hasFuncDef {
			fenceHasFuncClass = true
		}
		if b.hasImport {
			fenceHasImportCount++
		}
	}

	// ── Phase 3-4: count standalone suspicious lines outside fences ──
	isExcluded := buildByteExclusion(blocks)
	standaloneImportCount := countMatches(content, isExcluded, importPatterns)
	standaloneFuncClassCount := countMatches(content, isExcluded, funcClassPatterns)

	hasImport := standaloneImportCount > 0 || fenceHasImportCount > 0
	hasFuncOrClass := standaloneFuncClassCount > 0 || fenceHasFuncClass

	shebangCount := 0
	if hasShebang {
		shebangCount = 1
	}

	totalSuspicious := standaloneImportCount + standaloneFuncClassCount + shebangCount + fenceHasImportCount
	if len(blocks) > 0 {
		// Each fence block is itself a code indicator.
		totalSuspicious += len(blocks)
	}

	// ── Phase 5: SQL detection ──
	var hasSQL bool
	for _, b := range blocks {
		innerText := contentStr[b.innerStart:b.innerEnd]
		for _, re := range sqlDDLPatterns {
			if re.MatchString(innerText) {
				hasSQL = true
				break
			}
		}
		if hasSQL {
			break
		}
	}
	if !hasSQL && len(blocks) == 0 {
		// Outside fences: dangerous SQL keywords are a strong code signal.
		for _, re := range sqlDDLPatterns {
			if re.MatchString(contentStr) {
				hasSQL = true
				break
			}
		}
	}
	if hasSQL {
		totalSuspicious++
	}

	// ── Scoring ──
	var confidence float64
	var severity string

	switch {
	case fenceLines >= 3 && fenceHasFuncClass:
		// Full function/class body inside code fence.
		confidence = 0.90
		severity = "high"
	case totalSuspicious >= 3 || (hasShebang && hasImport) || fenceLines > 0:
		confidence = 0.70
		severity = "medium"
	case totalSuspicious >= 1:
		// 1-2 suspicious indicators.
		confidence = 0.40
		severity = "low"
	default:
		return nil, nil
	}

	finding := Finding{
		Type:       "code_leak",
		Category:   "source_code",
		Severity:   severity,
		Match:      buildCodeLeakSummary(hasShebang, hasImport, hasFuncOrClass, fenceLines, hasSQL),
		StartPos:   0,
		EndPos:     len(content),
		Confidence: confidence,
	}

	return []Finding{finding}, nil
}

// ────────────────────────────── Helpers ─────────────────────────────────────────

// extractFenceBlocks finds all ```language … ``` code blocks in s.
func extractFenceBlocks(s string) []fenceBlock {
	openMatches := fenceOpenRe.FindAllStringSubmatchIndex(s, -1)
	if len(openMatches) == 0 {
		return nil
	}
	closeMatches := fenceCloseRe.FindAllStringIndex(s, -1)

	var blocks []fenceBlock
	ci := 0 // index into closeMatches

	for _, om := range openMatches {
		// om[0:2] = full match, om[2:4] = language capture group.
		openEnd := om[1] // byte after the opening ``` line

		// Find the next close ``` after this open.
		for ci < len(closeMatches) && closeMatches[ci][0] < openEnd {
			ci++
		}
		if ci >= len(closeMatches) {
			break
		}

		closeStart := closeMatches[ci][0] // byte of closing ```
		closeEnd := closeMatches[ci][1]   // byte after closing ``` line

		// Trim leading/trailing whitespace and newlines so empty fences
		// (```python\n```) correctly report 0 content lines.
		rawInner := s[openEnd:closeStart]
		trimmed := strings.Trim(rawInner, " \t\r\n")
		lineCount := 0
		if trimmed != "" {
			lineCount = strings.Count(trimmed, "\n") + 1
		}

		block := fenceBlock{
			startByte:  om[0],
			endByte:    closeEnd,
			innerStart: openEnd,
			innerEnd:   closeStart,
			lineCount:  lineCount,
		}

		// Check for import patterns inside the fence.
		for _, re := range importPatterns {
			if re.MatchString(rawInner) {
				block.hasImport = true
				break
			}
		}

		// Check for function/class definitions inside the fence.
		for _, re := range funcClassPatterns {
			if re.MatchString(rawInner) {
				block.hasFuncDef = true
				break
			}
		}

		blocks = append(blocks, block)
		ci++
	}
	return blocks
}

// buildByteExclusion creates a predicate that reports whether a byte position
// lies within any extracted fence block.
func buildByteExclusion(blocks []fenceBlock) func(int) bool {
	if len(blocks) == 0 {
		return func(int) bool { return false }
	}
	return func(pos int) bool {
		for _, b := range blocks {
			if pos >= b.startByte && pos < b.endByte {
				return true
			}
		}
		return false
	}
}

// countMatches counts regex matches across the given regexps whose start
// position is NOT inside any fence block.
func countMatches(content []byte, isExcluded func(int) bool, patterns []*regexp.Regexp) int {
	count := 0
	for _, re := range patterns {
		matches := re.FindAllIndex(content, -1)
		for _, loc := range matches {
			if !isExcluded(loc[0]) {
				count++
			}
		}
	}
	return count
}

// buildCodeLeakSummary constructs a human-readable summary for the Match field.
func buildCodeLeakSummary(hasShebang, hasImport, hasFuncOrClass bool, fenceLines int, hasSQL bool) string {
	parts := make([]string, 0, 5)
	if hasShebang {
		parts = append(parts, "shebang")
	}
	if hasImport {
		parts = append(parts, "import statements")
	}
	if hasFuncOrClass {
		parts = append(parts, "function/class definitions")
	}
	if fenceLines > 0 {
		parts = append(parts, "code fence blocks")
	}
	if hasSQL {
		parts = append(parts, "SQL statements")
	}
	if len(parts) == 0 {
		return "source code patterns detected"
	}
	return "source code detected: " + strings.Join(parts, ", ")
}
