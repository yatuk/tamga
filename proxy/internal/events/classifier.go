package events

// ClassifierRecord is what the inline classifier and its shadow model said
// about a request's texts, without the texts. A request carries one only
// when the classifier service runs a shadow model (a candidate scored next
// to the model in service, never acted on), so that the two can be compared
// on real traffic; it is nil on every other request.
type ClassifierRecord struct {
	Model string `json:"model,omitempty"`
	// Threshold is scan.classifier.threshold; a text that carries a signal
	// was judged by its own (ClassifierText.Threshold).
	Threshold       float64 `json:"threshold"`
	ShadowModel     string  `json:"shadow_model"`
	ShadowThreshold float64 `json:"shadow_threshold"`
	// Texts are those the classifier was asked about, newest first, as it
	// was asked. Segments it skipped (flagged by the rules already, over
	// max_chars, nothing worth reading) are not here.
	Texts []ClassifierText `json:"texts"`
}

// ClassifierText is one text's scores. Seg is the segment's index in the
// request as the proxy read it, and Role its role.
type ClassifierText struct {
	Seg  int    `json:"seg"`
	Role string `json:"role,omitempty"`
	// Score is the primary model's; Flagged says it was at or above
	// Threshold, the threshold the text was judged by.
	Score     float64 `json:"score"`
	Threshold float64 `json:"threshold"`
	Flagged   bool    `json:"flagged"`
	Truncated bool    `json:"truncated,omitempty"`
	// Cached: the scores are those the text was given for an earlier
	// request (the proxy does not score a text twice within ten minutes).
	Cached bool `json:"cached,omitempty"`
	// Shadow is the shadow model's score, absent when it gave none for the
	// text (ShadowSkipped then says why: "late", "busy", "failed").
	// ShadowFlagged says it was at or above ShadowThreshold.
	Shadow        *float64 `json:"shadow,omitempty"`
	ShadowFlagged bool     `json:"shadow_flagged"`
	ShadowSkipped string   `json:"shadow_skipped,omitempty"`
}
