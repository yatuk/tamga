package proxy

import (
	"context"
	"sync"
	"time"

	"github.com/yatuk/tamga/internal/events"
	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/scanner"
)

// requestOutcome is what the proxy learns about a request after it was
// decided: the tokens and cost the provider's answer reported, and what the
// scan of that answer found. A request's event is published once, when the
// request is over, and takes all of this with it, so the stored row is
// written once and whole. It also holds what was true when the request was
// decided and may not be later: the policy, and whether the caller was
// identified by a key.
//
// The upstream callbacks fill it from other goroutines (a stream is scanned
// as it is relayed), hence the lock.
type requestOutcome struct {
	mu sync.Mutex

	policyName, policyVersion string
	// verified: the caller's organisation came from a key, not from a header.
	verified bool

	// Both zero: the provider reported no usage the proxy could read.
	inTok, outTok int
	costUSD       float64
	costKnown     bool

	outputScanned  bool
	outputAction   string
	outputFindings []scanner.Finding
}

type outcomeKey struct{}

func withOutcome(ctx context.Context, o *requestOutcome) context.Context {
	return context.WithValue(ctx, outcomeKey{}, o)
}

// outcomeFrom returns the request's outcome, or nil when ctx carries none.
// Every method of requestOutcome accepts nil.
func outcomeFrom(ctx context.Context) *requestOutcome {
	o, _ := ctx.Value(outcomeKey{}).(*requestOutcome)
	return o
}

// setUsage records the token counts the provider reported.
func (o *requestOutcome) setUsage(inTok, outTok int) {
	if o == nil || inTok+outTok <= 0 {
		return
	}
	o.mu.Lock()
	o.inTok, o.outTok = inTok, outTok
	o.mu.Unlock()
}

// setCost records what the request was charged. It is not called when the
// model has no price: that cost is unknown, not zero.
func (o *requestOutcome) setCost(usd float64) {
	if o == nil {
		return
	}
	o.mu.Lock()
	o.costUSD, o.costKnown = usd, true
	o.mu.Unlock()
}

// recordOutput records a decision on the response. scanned says the response
// was read by the scanners (a clean one has no findings and action PASS); it
// is false when the response was acted on without a verdict of theirs, as
// when it is withheld because it could not be scanned.
//
// The findings are stored as the request's are: the scanners mask what they
// matched. A canary finding is made without the token (canaryLeakFinding);
// its match is set again here, so that one built any other way is recorded
// without it too.
func (o *requestOutcome) recordOutput(findings []scanner.Finding, action policy.Action, scanned bool) {
	if o == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if scanned || len(findings) > 0 {
		o.outputScanned = true
	}
	for _, f := range findings {
		if f.Type == "system_prompt_leak" {
			f.Match = canaryMasked
		}
		o.outputFindings = append(o.outputFindings, f)
	}
	if outputActionRank(string(action)) > outputActionRank(o.outputAction) {
		o.outputAction = string(action)
	}
}

// outputActionRank orders the actions a response can get, so that of two
// decisions on one response (a canary leak, then the scan) the one that
// did more is kept.
func outputActionRank(action string) int {
	switch policy.Action(action) {
	case policy.ActionBlock:
		return 5
	case policy.ActionRedact, policy.ActionStrip:
		return 4
	case policy.ActionWarn:
		return 3
	case policy.ActionLog:
		return 2
	case "":
		return 0
	}
	return 1 // PASS
}

// fill puts the outcome on the request's event.
func (o *requestOutcome) fill(e *events.Event) {
	if o == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	e.PolicyName, e.PolicyVersion = o.policyName, o.policyVersion
	e.OrgVerified = o.verified
	e.InputTokens, e.OutputTokens = o.inTok, o.outTok
	if o.costKnown {
		e.CostKnown, e.CostUSD = true, o.costUSD
	}
	e.OutputAction = o.outputAction
	if len(o.outputFindings) > 0 {
		e.OutputFindings = append([]scanner.Finding(nil), o.outputFindings...)
	}
	if o.outputScanned {
		e.OutputScanned = true
		e.OutputRisk = scanner.CalculateRisk(o.outputFindings)
	}
}

// recordOutput records a decision on the response in the outcome ctx carries.
// It is called where the decision is made, before the response goes on, so
// the request's event, published after the response, has it.
func recordOutput(ctx context.Context, findings []scanner.Finding, action policy.Action, scanned bool) {
	outcomeFrom(ctx).recordOutput(findings, action, scanned)
}

// durationMs is d in milliseconds, with the microseconds as its fraction. A
// scan takes less than a millisecond more often than not.
func durationMs(d time.Duration) float64 {
	return float64(d.Microseconds()) / 1000
}
