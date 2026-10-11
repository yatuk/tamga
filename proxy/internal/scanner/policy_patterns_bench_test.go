package scanner

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/yatuk/tamga/internal/extract"
)

// The policy's own patterns as a deployment of some size has them: a handful
// of custom entities and a few competitors.
func benchCustomSpecs() []CustomEntitySpec {
	return []CustomEntitySpec{
		{Name: "customer_no", Pattern: `\bCUST-\d{6}\b`, Severity: "high"},
		{Name: "project_code", Pattern: `\bPRJ-[A-Z]{3}-\d{4}\b`, Severity: "medium"},
		{Name: "employee_id", Pattern: `\bemp\d{5}\b`, Severity: "medium"},
		{Name: "contract_ref", Pattern: `\b(?:SOZ|CTR)/20\d{2}/\d{4,6}\b`, Severity: "high"},
		{Name: "internal_host", Pattern: `\b[a-z0-9-]+\.corp\.example\b`, Severity: "low"},
	}
}

func benchCompetitorSpecs() []CompetitorSpec {
	return []CompetitorSpec{
		{Name: "acme", Patterns: []string{`acme\s+cloud`, `acmecorp`}, Severity: "low", Action: "log", Enabled: true},
		{Name: "globex", Patterns: []string{`globex`, `globex\s+ai`}, Severity: "low", Action: "log", Enabled: true},
		{Name: "initech", Patterns: []string{`initech`}, Severity: "low", Action: "log", Enabled: true},
	}
}

// proseText returns n bytes or a little more of the text a support
// conversation holds: mostly plain letters, some Turkish ones, a number here
// and there. seed makes two texts differ.
func proseText(n, seed int) string {
	lines := []string{
		"Merhaba, geçen ayın satış raporunu özetler misin? Müşteri sayısı arttı ve gelir yükseldi. ",
		"The deployment finished at 14:32 and all health checks passed on the second attempt. ",
		"Sipariş takibi için müşteri temsilcisi yarın sabah dönüş yapacak, teşekkür ederiz. ",
		"Please review the attached summary before the meeting; section three has the open questions. ",
		"Toplantı notları paylaşıldı, bir sonraki adım bütçe onayı ve takvimin güncellenmesi. ",
	}
	var b strings.Builder
	b.Grow(n + 128)
	for i := 0; b.Len() < n; i++ {
		b.WriteString(lines[(i+seed)%len(lines)])
		b.WriteString("(ref ")
		b.WriteString(strconv.Itoa(seed*1000 + i))
		b.WriteString(") ")
	}
	return b.String()
}

// BenchmarkPolicyPatternScanners is the cost of one scanner on 2 KB of text.
// "scan" calls the scanner; "registry" goes through the pipeline, where a
// text of this length is remembered.
func BenchmarkPolicyPatternScanners(b *testing.B) {
	text := []byte(proseText(2<<10, 1))
	custom := NewCustomScanner(benchCustomSpecs)
	competitor := NewCompetitorScanner(benchCompetitorSpecs)
	ctx := context.Background()
	for _, bm := range []struct {
		name string
		s    Scanner
	}{
		{"custom_2KB", custom},
		{"competitor_2KB", competitor},
		{"pii_2KB", NewPIIScanner()},
		{"secret_2KB", NewSecretScanner()},
	} {
		b.Run(bm.name+"/scan", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_, _ = bm.s.Scan(ctx, text)
			}
		})
		b.Run(bm.name+"/registry", func(b *testing.B) {
			reg := NewRegistry()
			reg.Register(bm.s)
			ResetScanCache()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_, _ = reg.ScanAllWithConfig(ctx, text, PipelineConfig{Mode: ModeSync})
			}
		})
	}
	b.Run("all_four_2KB/uncached", func(b *testing.B) {
		reg := NewRegistry()
		reg.Register(NewPIIScanner())
		reg.Register(NewSecretScanner())
		reg.Register(custom)
		reg.Register(competitor)
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			ResetScanCache()
			_, _ = reg.ScanAllWithConfig(ctx, text, PipelineConfig{Mode: ModeSync})
		}
	})
}

// BenchmarkAgentHistory is an agent's conversation of 200 pieces of 3 KB
// each, sent twice: once as it is, then with one new piece at the end.
// first_turn scans the 200 with nothing remembered; next_turn scans the 201
// after the 200 have been scanned once.
func BenchmarkAgentHistory(b *testing.B) {
	reg := segmentRegistry()
	reg.Register(NewCustomScanner(benchCustomSpecs))
	reg.Register(NewCompetitorScanner(benchCompetitorSpecs))
	segs := make([]extract.Segment, 201)
	for i := range segs {
		role := extract.RoleUser
		if i%2 == 1 {
			role = extract.RoleTool
		}
		segs[i] = seg(role, "messages["+strconv.Itoa(i)+"].content", proseText(3<<10, i+1))
	}
	ctx := context.Background()
	cfg := PipelineConfig{Mode: ModeSync}

	b.Run("first_turn", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			ResetScanCache()
			_, _ = reg.ScanSegments(ctx, segs[:200], cfg)
		}
	})
	b.Run("next_turn", func(b *testing.B) {
		ResetScanCache()
		_, _ = reg.ScanSegments(ctx, segs[:200], cfg)
		turn := make([]extract.Segment, 201)
		copy(turn, segs[:200])
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			turn[200] = seg(extract.RoleUser, "messages[200].content", proseText(3<<10, 1000+i))
			_, _ = reg.ScanSegments(ctx, turn, cfg)
		}
	})
}
