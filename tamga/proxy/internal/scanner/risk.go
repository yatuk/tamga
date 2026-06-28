package scanner

import (
	"math"
	"strings"
)

// RiskScore summarizes aggregate risk from scanner findings (PwC-style 0–100%).
type RiskScore struct {
	Score      float64            `json:"score"`      // 0.0 - 1.0
	Percentage int                `json:"percentage"` // 0 - 100
	Level      string             `json:"level"`      // none, low, medium, high, critical
	Breakdown  map[string]float64 `json:"breakdown"`  // contribution by finding type (pii, secret, ...)
}

func severityWeight(sev string) float64 {
	switch strings.ToLower(strings.TrimSpace(sev)) {
	case "critical":
		return 1.0
	case "high":
		return 0.7
	case "medium":
		return 0.4
	case "low":
		return 0.1
	default:
		return 0.2
	}
}

func riskLevelFromScore(score float64) string {
	if score <= 0 {
		return "none"
	}
	if score <= 0.25 {
		return "low"
	}
	if score <= 0.50 {
		return "medium"
	}
	if score <= 0.75 {
		return "high"
	}
	return "critical"
}

// CalculateRisk aggregates findings into a normalized score and level.
// Multiple findings of the same type add a cumulative bump (+0.10 per extra finding, capped).
// Across types, the maximum type score dominates the overall score.
func CalculateRisk(findings []Finding) RiskScore {
	if len(findings) == 0 {
		return RiskScore{
			Score:      0,
			Percentage: 0,
			Level:      "none",
			Breakdown:  map[string]float64{},
		}
	}

	byType := map[string][]Finding{}
	for _, f := range findings {
		t := f.Type
		if t == "" {
			t = "unknown"
		}
		byType[t] = append(byType[t], f)
	}

	breakdown := make(map[string]float64, len(byType))
	var overall float64

	for typ, group := range byType {
		maxW := 0.0
		for _, f := range group {
			w := severityWeight(f.Severity)
			if w > maxW {
				maxW = w
			}
		}
		n := len(group)
		cumulative := 0.1 * float64(n-1)
		if cumulative > 0.5 {
			cumulative = 0.5
		}
		typeScore := maxW + cumulative
		if typeScore > 1 {
			typeScore = 1
		}
		breakdown[typ] = math.Round(typeScore*1000) / 1000
		if typeScore > overall {
			overall = typeScore
		}
	}

	overall = math.Min(1, overall)
	pct := int(math.Round(overall * 100))
	if pct > 100 {
		pct = 100
	}

	return RiskScore{
		Score:      math.Round(overall*1000) / 1000,
		Percentage: pct,
		Level:      riskLevelFromScore(overall),
		Breakdown:  breakdown,
	}
}
