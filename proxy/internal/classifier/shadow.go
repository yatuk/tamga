package classifier

import (
	"sort"
	"sync"
	"sync/atomic"
)

// How a text's shadow score compared with the primary's verdict on it. The
// primary is read at the threshold the text was judged by, the shadow at
// scan.classifier.shadow_threshold.
const (
	ShadowBoth        = "both"         // both flagged it
	ShadowNeither     = "neither"      // neither did
	ShadowOnly        = "shadow_only"  // the shadow would have flagged it, the primary did not
	ShadowPrimaryOnly = "primary_only" // the primary flagged it, the shadow would not have
	ShadowMissing     = "missing"      // the shadow gave no score (late, busy, failed)
)

// ShadowOutcome sorts one text into the outcomes above.
func ShadowOutcome(primaryFlagged bool, r Result, shadowThreshold float64) string {
	if !r.HasShadow {
		return ShadowMissing
	}
	shadow := r.ShadowScore >= shadowThreshold
	switch {
	case primaryFlagged && shadow:
		return ShadowBoth
	case shadow:
		return ShadowOnly
	case primaryFlagged:
		return ShadowPrimaryOnly
	}
	return ShadowNeither
}

type shadowKey struct{ model, outcome string }

var shadowCounts sync.Map // shadowKey -> *atomic.Int64

// RecordShadow counts one text's outcome for the named shadow model. It is
// exported as tamga_classifier_shadow_texts_total. Count a text when it is
// scored, not when it is answered from the cache, or a conversation counts
// its first message once per turn.
func RecordShadow(model, outcome string) {
	v, _ := shadowCounts.LoadOrStore(shadowKey{model, outcome}, new(atomic.Int64))
	v.(*atomic.Int64).Add(1)
}

// ShadowCount is one series of tamga_classifier_shadow_texts_total.
type ShadowCount struct {
	Model, Outcome string
	Texts          int64
}

// ShadowStats returns the counts, sorted by model and outcome. Empty until
// a shadow model has answered.
func ShadowStats() []ShadowCount {
	var out []ShadowCount
	shadowCounts.Range(func(k, v any) bool {
		key := k.(shadowKey)
		out = append(out, ShadowCount{Model: key.model, Outcome: key.outcome, Texts: v.(*atomic.Int64).Load()})
		return true
	})
	sort.Slice(out, func(i, j int) bool {
		if out[i].Model != out[j].Model {
			return out[i].Model < out[j].Model
		}
		return out[i].Outcome < out[j].Outcome
	})
	return out
}
