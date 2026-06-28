package scanner

import "testing"

func TestCalculateConfidenceThresholds(t *testing.T) {
	tests := []struct {
		name   string
		f      ConfidenceFactor
		total  int
		action string
	}{
		{
			name:   "format only stays pass",
			f:      ConfidenceFactor{Format: WFormat},
			total:  30,
			action: ActionPass,
		},
		{
			name:   "format plus algorithm is pass_log",
			f:      ConfidenceFactor{Format: WFormat, Algorithm: WAlgorithm},
			total:  60,
			action: ActionPassLog,
		},
		{
			name:   "format algorithm database is redact",
			f:      ConfidenceFactor{Format: WFormat, Algorithm: WAlgorithm, Database: WDatabase},
			total:  80,
			action: ActionRedact,
		},
		{
			name:   "all factors reaches block",
			f:      ConfidenceFactor{Format: WFormat, Algorithm: WAlgorithm, Database: WDatabase, Context: WContext},
			total:  100,
			action: ActionBlock,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateConfidence(tt.f)
			if got.Total != tt.total {
				t.Fatalf("total mismatch: got=%d want=%d", got.Total, tt.total)
			}
			if got.Action != tt.action {
				t.Fatalf("action mismatch: got=%s want=%s", got.Action, tt.action)
			}
			if got.Reasoning == "" {
				t.Fatal("reasoning should not be empty")
			}
		})
	}
}
