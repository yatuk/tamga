package proxy

import (
	"context"
	"errors"
	"fmt"

	"github.com/yatuk/tamga/internal/classifier"
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
var errClassifierNotConfigured = errors.New("scan.classifier is enabled but TAMGA_CLASSIFIER_ADDR is not set")

// classifySegments asks the classifier about the segments the rules left
// open. It is called only when the request is not already blocked.
//
// Segments are taken newest first until max_chars is used up, because in a
// conversation the newest text is the part no earlier request has vouched
// for. A segment the rules already flagged as an injection is skipped: the
// answer is known.
func classifySegments(ctx context.Context, guard *classifier.Guard, set policy.ClassifierSettings, requestID string, segs []extract.Segment, known []scanner.Finding) classifyOutcome {
	var out classifyOutcome
	flagged := make(map[int]bool)
	for _, f := range known {
		if f.Type == "injection" && f.Seg >= 0 {
			flagged[f.Seg] = true
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
		if res.Score < set.Threshold {
			continue
		}
		seg := segs[segIdx[j]]
		out.findings = append(out.findings, scanner.Finding{
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
		})
	}
	return out
}

// classifierSeverity is fixed: the score is a confidence, carried on the
// finding as such, and says nothing about how much harm the text would do.
func classifierSeverity(float64) string { return "high" }
