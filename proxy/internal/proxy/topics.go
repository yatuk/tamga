package proxy

import (
	"context"
	"fmt"
	"net/http"
	"unicode/utf8"

	"github.com/yatuk/tamga/internal/classifier"
	"github.com/yatuk/tamga/internal/errkind"
	"github.com/yatuk/tamga/internal/extract"
	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/scanner"
)

// topicOutcome is what the banned-topic check added to a request's scan.
type topicOutcome struct {
	findings []scanner.Finding
	// asked is how many texts were sent or answered from the cache.
	asked int
	// partial means some text that qualified was not read whole: it did
	// not fit in max_chars, or was too long for the model.
	partial bool
	// err is set when the topic model could not answer at all.
	err error
}

// errTopicsNotConfigured is the failure when the policy turns topics on and
// the proxy was started without a classifier service.
var errTopicsNotConfigured = errkind.New("not_configured", "topics is enabled but TAMGA_CLASSIFIER_ADDR is not set")

// topicSegments asks the topic model whether the request is about one of
// the policy's banned topics. It is called only when the request is not
// already blocked.
//
// Segments are taken newest first until max_chars is used up, as for the
// inline classifier. A segment longer than what is left is read in part,
// from its start, and the outcome is partial: a request is usually stated
// at the start of a message, and reading some of a long one is better than
// reading none of it.
//
// A finding names the topic and the score. It carries neither the text nor
// the topic's description.
func topicSegments(ctx context.Context, guard *classifier.TopicGuard, set policy.TopicSettings, requestID string, segs []extract.Segment) topicOutcome {
	var out topicOutcome
	var texts []string
	var segIdx []int
	budget := set.MaxChars
	for i := len(segs) - 1; i >= 0 && budget > 0; i-- {
		seg := segs[i]
		// A number or a key in a tool call's arguments is not text.
		if !set.CoversRole(string(seg.Role)) || seg.Literal != "" {
			continue
		}
		text := classifier.Prepare(seg.Text)
		if text == "" {
			continue
		}
		if len(text) > budget {
			// What is left of the budget may be too little to ask about.
			text = classifier.Prepare(cutUTF8(text, budget))
			out.partial = true
			if text == "" {
				continue
			}
		}
		budget -= len(text)
		texts = append(texts, text)
		segIdx = append(segIdx, i)
	}
	if len(texts) == 0 {
		return out
	}
	out.asked = len(texts)

	results, err := guard.Score(ctx, requestID, texts, set.Descriptions(), set.Timeout)
	if err != nil {
		out.err = err
		return out
	}
	for j, res := range results {
		if res.Truncated {
			out.partial = true
		}
		seg := segs[segIdx[j]]
		for k, topic := range set.Topics {
			if k >= len(res.Scores) || res.Scores[k] < topic.Threshold {
				continue
			}
			score := res.Scores[k]
			out.findings = append(out.findings, scanner.Finding{
				Type:       "topic",
				Category:   topic.Name,
				Severity:   "medium",
				Match:      fmt.Sprintf("topic score %.2f", score),
				Confidence: score,
				Role:       string(seg.Role),
				Path:       seg.Path,
				Seg:        segIdx[j],
				Metadata:   map[string]string{"threshold": fmt.Sprintf("%.2f", topic.Threshold)},
				// No StartPos/EndPos: a topic is a verdict on the text as
				// a whole, so the finding can be acted on, not redacted.
			})
		}
	}
	return out
}

// cutUTF8 returns at most n bytes of s, cut at a character boundary.
func cutUTF8(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// markDegraded adds what to X-Tamga-Scan-Degraded, keeping what an earlier
// stage put there: "classifier,topics" when both could not answer.
func markDegraded(w http.ResponseWriter, what string) {
	if prev := w.Header().Get("X-Tamga-Scan-Degraded"); prev != "" && prev != what {
		w.Header().Set("X-Tamga-Scan-Degraded", prev+","+what)
		return
	}
	w.Header().Set("X-Tamga-Scan-Degraded", what)
}
