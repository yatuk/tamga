package proxy

import (
	"context"
	"fmt"

	"github.com/yatuk/tamga/internal/classifier"
	"github.com/yatuk/tamga/internal/errkind"
	"github.com/yatuk/tamga/internal/extract"
	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/scanner"
)

// classifyOutcome is what the inline classifier added to a request's scan.
type classifyOutcome struct {
	findings []scanner.Finding
	// asked is how many texts were sent or answered from the cache.
	asked int
	// partial means some text that qualified was not classified: it did not
	// fit in max_chars, or was too long for the model to read whole.
	partial bool
	// err is set when the classifier could not answer at all.
	err error
}

// errClassifierNotConfigured is the failure when the policy turns the
// classifier on and the proxy was started without one.
var errClassifierNotConfigured = errkind.New("not_configured", "scan.classifier is enabled but TAMGA_CLASSIFIER_ADDR is not set")

// classifySegments asks the classifier about the segments the rules left
// open. It is called only when the request is not already blocked.
//
// Segments are taken newest first until max_chars is used up, because in a
// conversation the newest text is the part no earlier request has vouched
// for. A segment the rules already flagged as an injection is skipped: the
// answer is known.
//
// A segment that carries a signal is judged by signal_threshold, which is
// lower. The signals are the rules' low-severity findings for text that
// hides something (scanner.Signal) and a change of writing system against
// the user's earlier messages or inside the text (classifier.ScriptSwitch).
// None of them is a verdict, and the second is not a finding at all.
func classifySegments(ctx context.Context, guard *classifier.Guard, set policy.ClassifierSettings, requestID string, segs []extract.Segment, known []scanner.Finding) classifyOutcome {
	var out classifyOutcome
	flagged := make(map[int]bool)
	signals := make(map[int]string)
	for _, f := range known {
		if f.Type != "injection" || f.Seg < 0 {
			continue
		}
		if scanner.Signal(f) {
			signals[f.Seg] = f.Category
		} else {
			flagged[f.Seg] = true
		}
	}

	// The script of the conversation before each segment: that of the
	// user's messages up to it.
	before := make([]classifier.Script, len(segs))
	var userText classifier.ScriptCounts
	for i, seg := range segs {
		before[i] = userText.Dominant()
		if seg.Role == extract.RoleUser {
			userText.Add(seg.Text)
		}
	}

	var texts []string
	var segIdx []int
	budget := set.MaxChars
	for i := len(segs) - 1; i >= 0; i-- {
		seg := segs[i]
		if !set.CoversRole(string(seg.Role)) || flagged[i] {
			continue
		}
		text := classifier.Prepare(seg.Text)
		if text == "" {
			continue
		}
		if len(text) > budget {
			out.partial = true
			continue
		}
		budget -= len(text)
		texts = append(texts, text)
		segIdx = append(segIdx, i)
		if _, ok := signals[i]; !ok {
			if _, switched := classifier.ScriptSwitch(before[i], seg.Text); switched {
				signals[i] = "script_switch"
			}
		}
	}
	if len(texts) == 0 {
		return out
	}
	out.asked = len(texts)

	results, err := guard.Score(ctx, requestID, texts, set.Timeout)
	if err != nil {
		out.err = err
		return out
	}
	for j, res := range results {
		if res.Truncated {
			out.partial = true
		}
		threshold := set.Threshold
		signal, signalled := signals[segIdx[j]]
		if signalled {
			threshold = set.SignalThreshold
		}
		if res.Score < threshold {
			continue
		}
		seg := segs[segIdx[j]]
		f := scanner.Finding{
			Type:       "injection",
			Category:   "classifier",
			Severity:   classifierSeverity(res.Score),
			Match:      fmt.Sprintf("classifier score %.2f", res.Score),
			Confidence: res.Score,
			Role:       string(seg.Role),
			Path:       seg.Path,
			Seg:        segIdx[j],
			// No StartPos/EndPos: the model judges the text as a whole.
			// There is no span to point at, so the finding cannot be
			// redacted, only acted on.
		}
		if signalled && res.Score < set.Threshold {
			// Reported only because of the signal: say which.
			f.Metadata = map[string]string{"signal": signal, "threshold": fmt.Sprintf("%.2f", threshold)}
		}
		out.findings = append(out.findings, f)
	}
	return out
}

// classifierSeverity is fixed: the score is a confidence, carried on the
// finding as such, and says nothing about how much harm the text would do.
func classifierSeverity(float64) string { return "high" }
