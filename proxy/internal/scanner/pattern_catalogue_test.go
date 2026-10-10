package scanner

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

type catalogueCategory struct {
	count  int
	lo, hi string
}

type cataloguePhrase struct {
	cell       string // the phrase as the table writes it, pipes unescaped
	confidence string
}

// readPatternCatalogue reads the tables of patterns/README.md: the category
// table (count, lowest and highest confidence, and the total), and the
// phrase rows under each "### `category`" heading.
func readPatternCatalogue(t *testing.T) (cats map[string]catalogueCategory, total int, phrases map[string][]cataloguePhrase) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("patterns", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	cats = map[string]catalogueCategory{}
	phrases = map[string][]cataloguePhrase{}
	total = -1
	section, sub := "", ""
	for _, line := range strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n") {
		switch {
		case strings.HasPrefix(line, "## "):
			section, sub = strings.TrimSpace(line[3:]), ""
			continue
		case strings.HasPrefix(line, "### "):
			sub = strings.Trim(strings.TrimSpace(line[4:]), "`")
			continue
		}
		if section != "Categories" || !strings.HasPrefix(line, "|") || strings.HasPrefix(line, "|---") {
			continue
		}
		// A pipe inside a phrase is written "\|".
		const escaped = "\x00"
		cells := strings.Split(strings.Trim(strings.ReplaceAll(line, `\|`, escaped), "|"), "|")
		for i := range cells {
			cells[i] = strings.TrimSpace(strings.ReplaceAll(cells[i], escaped, "|"))
		}
		if sub == "" {
			// | `category` | Phrases | Lowest confidence | Highest confidence |
			if len(cells) != 4 || cells[0] == "Category" {
				continue
			}
			n, err := strconv.Atoi(cells[1])
			if err != nil {
				t.Fatalf("patterns/README.md: count %q in row %q", cells[1], line)
			}
			if cells[0] == "Total" {
				total = n
				continue
			}
			cats[strings.Trim(cells[0], "`")] = catalogueCategory{count: n, lo: cells[2], hi: cells[3]}
			continue
		}
		// | `phrase` | Base confidence |
		if len(cells) != 2 || cells[0] == "Phrase" {
			continue
		}
		phrases[sub] = append(phrases[sub], cataloguePhrase{cell: cells[0], confidence: cells[1]})
	}
	if len(cats) == 0 || total < 0 {
		t.Fatal("patterns/README.md: the category table was not found")
	}
	return cats, total, phrases
}

// The pattern catalogue is the one place that says how many injection
// phrases there are, and it says what the code holds. A count that is
// written down and not checked is wrong after the next phrase: the header of
// injection.go said 83 when there were 91.
func TestPatternCatalogueMatchesTheCode(t *testing.T) {
	cats, total, phrases := readPatternCatalogue(t)

	type have struct {
		count  int
		lo, hi float64
	}
	code := map[string]*have{}
	conf := func(v float64) string { return fmt.Sprintf("%.2f", v) }
	for _, p := range injectionPatterns {
		h := code[p.category]
		if h == nil {
			h = &have{lo: p.baseConfidence, hi: p.baseConfidence}
			code[p.category] = h
		}
		h.count++
		if p.baseConfidence < h.lo {
			h.lo = p.baseConfidence
		}
		if p.baseConfidence > h.hi {
			h.hi = p.baseConfidence
		}

		// The phrase has its row, with its confidence.
		found := false
		for _, row := range phrases[p.category] {
			if strings.Contains(row.cell, p.phrase) && row.confidence == conf(p.baseConfidence) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("patterns/README.md has no row for %q (%s) under `%s`", p.phrase, conf(p.baseConfidence), p.category)
		}
	}

	if total != len(injectionPatterns) {
		t.Errorf("patterns/README.md says %d phrases in total, injectionPatterns has %d", total, len(injectionPatterns))
	}
	for name, h := range code {
		doc, ok := cats[name]
		if !ok {
			t.Errorf("patterns/README.md has no row for category `%s` (%d phrases)", name, h.count)
			continue
		}
		if doc.count != h.count || doc.lo != conf(h.lo) || doc.hi != conf(h.hi) {
			t.Errorf("patterns/README.md says `%s` has %d phrases from %s to %s; the code has %d from %s to %s",
				name, doc.count, doc.lo, doc.hi, h.count, conf(h.lo), conf(h.hi))
		}
		if rows := len(phrases[name]); rows != h.count {
			t.Errorf("patterns/README.md lists %d phrases under `%s`, the code has %d", rows, name, h.count)
		}
	}
	for name := range cats {
		if code[name] == nil {
			t.Errorf("patterns/README.md has a row for category `%s`, which no phrase uses", name)
		}
	}
}
