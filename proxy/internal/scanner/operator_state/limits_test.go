package operator_state

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/scanner"
)

func TestDecisionPatternLimits(t *testing.T) {
	for _, tc := range []struct{ pattern, limit string }{
		{strings.Repeat("(?:a|b)", 20000), "the limit is 1024"},
		{`(?s).{0,1000}secret`, "the limit is 500"},
		{`\p{L}{1000}`, "the limit is 500"},
	} {
		assertion := AssertionRule{DecisionPattern: tc.pattern}
		auth := AuthRule{DecisionPattern: tc.pattern}
		for _, err := range []error{assertion.Compile(), auth.Compile()} {
			if err == nil || !strings.Contains(err.Error(), tc.limit) || len(err.Error()) > 300 {
				t.Errorf("%.20s: err = %v, want one naming %q", tc.pattern, err, tc.limit)
			}
		}
		// A rule that did not compile matches nothing; it is never run.
		if assertion.MatchesDecision("D-2026-01-01-001") || auth.MatchesDecision("D-2026-01-01-001") {
			t.Errorf("%.20s: an uncompiled rule matched", tc.pattern)
		}
	}
	// A decision pattern is tried against an id, so "everything" is a fair
	// thing for it to say.
	all := AssertionRule{DecisionPattern: `.*`}
	if err := all.Compile(); err != nil {
		t.Errorf(".*: %v", err)
	}
}

func TestOperatorStateRuleCountLimits(t *testing.T) {
	cfg := func(assertions, auths int) *policy.OperatorStateConfig {
		c := &policy.OperatorStateConfig{Enabled: true}
		for i := 0; i < assertions; i++ {
			c.Assertions = append(c.Assertions, policy.OperatorStateAssertion{DecisionPattern: fmt.Sprintf("D-%04d-.*", i), RequiredState: "locked"})
		}
		for i := 0; i < auths; i++ {
			c.Authorization = append(c.Authorization, policy.OperatorAuthorization{DecisionPattern: fmt.Sprintf("D-%04d-.*", i), AllowedOperators: []string{"a"}})
		}
		return c
	}
	max := scanner.MaxOperatorStatePatterns
	if _, err := LoadRulesFromPolicy(cfg(max, max)); err != nil {
		t.Errorf("at the limit: %v", err)
	}
	if _, err := LoadRulesFromPolicy(cfg(max+1, 0)); err == nil || !strings.Contains(err.Error(), "the limit is 50") {
		t.Errorf("assertions over the limit: %v", err)
	}
	if _, err := LoadRulesFromPolicy(cfg(0, max+1)); err == nil || !strings.Contains(err.Error(), "the limit is 50") {
		t.Errorf("authorization over the limit: %v", err)
	}
}

// An id of any length can be written into a prompt. The patterns see its
// first 128 bytes, which is where a real id ends several times over.
func TestLongDecisionIDIsMatchedByItsStart(t *testing.T) {
	rule := AssertionRule{DecisionPattern: `D-2026-.*`}
	if err := rule.Compile(); err != nil {
		t.Fatal(err)
	}
	long := "D-2026-01-01-" + strings.Repeat("9", 1<<20)
	if !rule.MatchesDecision(long) {
		t.Error("a governed id stopped being governed by being long")
	}
	if got := matchableID(long); len(got) != maxDecisionIDBytes {
		t.Errorf("matched against %d bytes", len(got))
	}
	if got := matchableID("D-2026-01-01-001"); got != "D-2026-01-01-001" {
		t.Errorf("a short id was changed: %q", got)
	}
}

func TestOperatorStateScanStopsWithTheContext(t *testing.T) {
	rules, err := LoadRulesFromPolicy(&policy.OperatorStateConfig{
		Enabled:    true,
		Assertions: []policy.OperatorStateAssertion{{DecisionPattern: "D-.*", RequiredState: "locked"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	s := NewOperatorStateScannerWithRules(NewProjection(), rules)
	text := []byte("as decided in D-2026-01-01-001 and D-2026-01-01-002")

	found, err := s.Scan(context.Background(), text)
	if err != nil || len(found) != 2 {
		t.Fatalf("live context: %d findings, err %v", len(found), err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	found, err = s.Scan(ctx, text)
	if !errors.Is(err, scanner.ErrScanDegraded) || len(found) != 0 {
		t.Errorf("ended context: %d findings, err %v; want none and ErrScanDegraded", len(found), err)
	}
}
