package policy

import (
	"fmt"
	"strings"
	"time"
)

// ClassifierConfig is scan.classifier: the inline prompt-injection
// classifier, a local model the proxy asks when its own rules found nothing
// that blocks the request.
type ClassifierConfig struct {
	Enabled bool `yaml:"enabled" json:"enabled"`
	// TimeoutMs is how long a request may wait for the classifier.
	// Default 150. A typical call takes 15 to 30 ms; the deadline is set
	// well above that because a missed deadline is a failed scan, and with
	// scan.on_error: block a failed scan is a 503.
	TimeoutMs int `yaml:"timeout_ms" json:"timeout_ms,omitempty"`
	// Threshold is the score at or above which a text is reported as an
	// injection finding. Default 0.98, the lowest value that flagged no
	// benign sample on the tuning corpus. Lower catches more and flags more
	// ordinary text; measure on your own traffic before lowering it.
	Threshold float64 `yaml:"threshold" json:"threshold,omitempty"`
	// SignalThreshold is the threshold for a text that carries a signal:
	// an unreadable block or hidden characters in it (the low-severity
	// injection findings unreadable_block and hidden_text), a request that
	// is an attack or not depending on where it stands (above_text_request,
	// translation_abandoned), a change of writing system against the
	// conversation so far, or the shape of an order to the model (a
	// persona, a system label, a turn to a new task, a dictated answer, an
	// order inside a tool result: classifier.Structure). Such a text is not
	// an attack for that, and is judged more strictly. Default 0.90; a
	// value above threshold is the same as threshold, which turns the
	// signals off. Not measured with a model: on the published sweep 0.90
	// flagged 1 of 117 and 2 of 85 benign samples when applied to every
	// text, and here it applies only to the few that carry a signal.
	SignalThreshold float64 `yaml:"signal_threshold" json:"signal_threshold,omitempty"`
	// ShadowThreshold is the threshold the shadow model is read at, when the
	// classifier service runs one (TAMGA_CLASSIFIER_SHADOW_MODEL_DIR): a
	// candidate model scored next to the one in service, whose scores are
	// recorded and never acted on. It only sorts the shadow's scores into
	// "would have flagged" and "would not" for the comparison metric and the
	// stored record, so it is the threshold the candidate would ship with.
	// Default: threshold. Choose it with classifier/finetune/sweep.py.
	ShadowThreshold float64 `yaml:"shadow_threshold" json:"shadow_threshold,omitempty"`
	// Roles are the message roles whose text is sent to the classifier.
	// Default: user and tool, the text that reaches the model from outside
	// the application.
	Roles []string `yaml:"roles" json:"roles,omitempty"`
	// MaxChars bounds the text one request may send to the classifier,
	// newest first. What does not fit is not classified, and the response
	// says so. Default 6000.
	MaxChars int `yaml:"max_chars" json:"max_chars,omitempty"`
}

// Classifier defaults.
const (
	DefaultClassifierTimeoutMs       = 150
	DefaultClassifierThreshold       = 0.98
	DefaultClassifierSignalThreshold = 0.90
	DefaultClassifierMaxChars        = 6000
)

var defaultClassifierRoles = []string{"user", "tool"}

func (c *ClassifierConfig) validate() error {
	if c == nil {
		return nil
	}
	if c.TimeoutMs < 0 || c.TimeoutMs > 5000 {
		return fmt.Errorf("scan.classifier.timeout_ms: %d is out of range (1 to 5000)", c.TimeoutMs)
	}
	if c.Threshold < 0 || c.Threshold > 1 {
		return fmt.Errorf("scan.classifier.threshold: %v is out of range (0 to 1)", c.Threshold)
	}
	if c.SignalThreshold < 0 || c.SignalThreshold > 1 {
		return fmt.Errorf("scan.classifier.signal_threshold: %v is out of range (0 to 1)", c.SignalThreshold)
	}
	if c.ShadowThreshold < 0 || c.ShadowThreshold > 1 {
		return fmt.Errorf("scan.classifier.shadow_threshold: %v is out of range (0 to 1)", c.ShadowThreshold)
	}
	if c.MaxChars < 0 {
		return fmt.Errorf("scan.classifier.max_chars: %d must not be negative", c.MaxChars)
	}
	for _, role := range c.Roles {
		if _, ok := MessageRoles[strings.ToLower(strings.TrimSpace(role))]; !ok {
			return fmt.Errorf("scan.classifier.roles: unknown role %q (known: system, user, assistant, tool, tool_definition, request)", role)
		}
	}
	return nil
}

// ClassifierSettings is scan.classifier with its defaults filled in.
type ClassifierSettings struct {
	Timeout   time.Duration
	Threshold float64
	// SignalThreshold is the threshold for text that carries a signal. It
	// is never above Threshold.
	SignalThreshold float64
	// ShadowThreshold is the shadow model's threshold (see ClassifierConfig).
	ShadowThreshold float64
	MaxChars        int
	roles           map[string]bool
}

// CoversRole reports whether text of this role is sent to the classifier.
func (s ClassifierSettings) CoversRole(role string) bool { return s.roles[role] }

// ClassifierSettings returns the classifier configuration, and false when
// the policy does not turn the classifier on.
func (p *Policy) ClassifierSettings() (ClassifierSettings, bool) {
	if p == nil || p.Scan == nil || p.Scan.Classifier == nil || !p.Scan.Classifier.Enabled {
		return ClassifierSettings{}, false
	}
	c := p.Scan.Classifier
	s := ClassifierSettings{
		Timeout:   time.Duration(c.TimeoutMs) * time.Millisecond,
		Threshold: c.Threshold,
		MaxChars:  c.MaxChars,
		roles:     make(map[string]bool),
	}
	if c.TimeoutMs == 0 {
		s.Timeout = DefaultClassifierTimeoutMs * time.Millisecond
	}
	if c.Threshold == 0 {
		s.Threshold = DefaultClassifierThreshold
	}
	s.SignalThreshold = c.SignalThreshold
	if s.SignalThreshold == 0 {
		s.SignalThreshold = DefaultClassifierSignalThreshold
	}
	if s.SignalThreshold > s.Threshold {
		s.SignalThreshold = s.Threshold
	}
	s.ShadowThreshold = c.ShadowThreshold
	if s.ShadowThreshold == 0 {
		s.ShadowThreshold = s.Threshold
	}
	if c.MaxChars == 0 {
		s.MaxChars = DefaultClassifierMaxChars
	}
	roles := c.Roles
	if len(roles) == 0 {
		roles = defaultClassifierRoles
	}
	for _, r := range roles {
		s.roles[strings.ToLower(strings.TrimSpace(r))] = true
	}
	return s, true
}
