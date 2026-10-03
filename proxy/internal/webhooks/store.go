// Package webhooks stores outbound alert destinations (Slack / Teams / SIEM).
//
// The working copy is in memory. With a Persister attached it is loaded from
// and written through to the database. A webhook's URL and token are
// credentials, so the persister it is given must encrypt.
package webhooks

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/yatuk/tamga/internal/docstore"
)

type Kind string

const (
	KindSlack      Kind = "slack"
	KindTeams      Kind = "teams"
	KindSplunk     Kind = "splunk"     // JSON over HTTP Event Collector (legacy preset)
	KindSplunkHEC  Kind = "splunk_hec" // CEF body over Splunk HEC raw endpoint
	KindSentinel   Kind = "sentinel"   // Azure Sentinel Common Event Format
	KindQRadar     Kind = "qradar"     // IBM QRadar LEEF 2.0 over syslog-http gateway
	KindDatadog    Kind = "datadog"
	KindJira       Kind = "jira"
	KindPagerDuty  Kind = "pagerduty"  // Events API v2, routing_key in body
	KindOpsgenie   Kind = "opsgenie"   // Alerts API v2, GenieKey in Authorization
	KindServiceNow Kind = "servicenow" // Table API /api/now/table/incident, Basic auth
	KindGeneric    Kind = "generic"
)

// AlertRule describes when an outbound alert should fire.
type AlertRule struct {
	// BlocksPerMinute fires when the proxy blocks more than N requests/min.
	BlocksPerMinute int `json:"blocks_per_minute,omitempty"`
	// SeverityAtLeast fires when any finding severity ≥ given level.
	SeverityAtLeast string `json:"severity_at_least,omitempty"`
}

type Webhook struct {
	ID              string            `json:"id"`
	Label           string            `json:"label"`
	Kind            Kind              `json:"kind"`
	URL             string            `json:"url"`
	Enabled         bool              `json:"enabled"`
	Rule            *AlertRule        `json:"rule,omitempty"`
	Headers         map[string]string `json:"headers,omitempty"`
	PayloadTemplate string            `json:"payload_template,omitempty"`
	// ProjectKey is Jira-specific: the target project key (e.g. "SEC"). Jira
	// Cloud v3 /rest/api/3/issue rejects create requests without it.
	ProjectKey string `json:"project_key,omitempty"`
	// IssueType is Jira-specific: the issue type name (e.g. "Task", "Bug").
	// When empty the renderer falls back to "Task".
	IssueType string `json:"issue_type,omitempty"`
	// AuthToken is used by integrations that require a body-inline secret
	// (PagerDuty `routing_key`) OR a header-bearer key (Opsgenie
	// `Authorization: GenieKey <token>`). Store-side write-only; redacted
	// in List responses by the caller.
	AuthToken string    `json:"auth_token,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	LastFired time.Time `json:"last_fired,omitempty"`
	// ThresholdCount is the minimum number of correlated events required
	// before the webhook fires. 0 means fire immediately (no correlation).
	ThresholdCount int `json:"threshold_count,omitempty"`
	// ThresholdWindowSecs is the sliding observation window in seconds.
	// Default 300 (5 minutes) when ThresholdCount > 0.
	ThresholdWindowSecs int `json:"threshold_window_secs,omitempty"`
	// CooldownSecs is the minimum seconds between re-fires for the same
	// correlation key. Default 0 (no cooldown).
	CooldownSecs int `json:"cooldown_secs,omitempty"`
}

type Store interface {
	List() []Webhook
	Get(id string) (Webhook, error)
	Create(w Webhook) (Webhook, error)
	Update(id string, w Webhook) (Webhook, error)
	Delete(id string) error
	Test(ctx context.Context, id string) (int, error)
	// Notify fires a webhook with correlation suppression. It accepts a
	// correlationKey used for grouping duplicate events. Returns
	// (should_fire, correlated_count, status_code, error).
	Notify(ctx context.Context, id, correlationKey string, payload map[string]interface{}) (bool, int, int, error)
}

type MemoryStore struct {
	// wmu serialises changes; mu guards the working copy.
	wmu        sync.Mutex
	persist    docstore.Persister
	mu         sync.RWMutex
	data       map[string]Webhook
	http       *http.Client
	correlator *CorrelationEngine
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		data:       make(map[string]Webhook),
		http:       &http.Client{Timeout: 5 * time.Second},
		correlator: NewCorrelationEngine(0),
	}
}

// NewMemoryStoreWithCorrelator is used for test injection.
// It returns a MemoryStore with the given correlation engine.
func NewMemoryStoreWithCorrelator(ce *CorrelationEngine) *MemoryStore {
	if ce == nil {
		ce = NewCorrelationEngine(0)
	}
	return &MemoryStore{
		data:       make(map[string]Webhook),
		http:       &http.Client{Timeout: 5 * time.Second},
		correlator: ce,
	}
}

// Correlator exposes the correlation engine for external management
// (e.g. periodic expiry).
func (s *MemoryStore) Correlator() *CorrelationEngine {
	return s.correlator
}

var ErrNotFound = errors.New("webhook not found")

// Persist attaches backing storage and loads what it holds. From then on a
// change is written there before it is applied here.
func (s *MemoryStore) Persist(p docstore.Persister) error {
	s.wmu.Lock()
	s.persist = p
	s.wmu.Unlock()
	return s.Reload()
}

// Reload replaces the working copy with what the backing storage holds.
// When a webhook last fired is known only to the replica that fired it, so
// that is kept.
func (s *MemoryStore) Reload() error {
	s.wmu.Lock()
	defer s.wmu.Unlock()
	if s.persist == nil {
		return nil
	}
	fresh := make(map[string]Webhook)
	err := s.persist.All(func(id string, raw []byte) error {
		var w Webhook
		if err := json.Unmarshal(raw, &w); err != nil {
			return err
		}
		fresh[id] = w
		return nil
	})
	if err != nil {
		return err
	}
	s.mu.Lock()
	for id, old := range s.data {
		if w, ok := fresh[id]; ok && old.LastFired.After(w.LastFired) {
			w.LastFired = old.LastFired
			fresh[id] = w
		}
	}
	s.data = fresh
	s.mu.Unlock()
	return nil
}

func (s *MemoryStore) List() []Webhook {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Webhook, 0, len(s.data))
	for _, w := range s.data {
		out = append(out, w)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (s *MemoryStore) Get(id string) (Webhook, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	w, ok := s.data[id]
	if !ok {
		return Webhook{}, ErrNotFound
	}
	return w, nil
}

func (s *MemoryStore) Create(w Webhook) (Webhook, error) {
	if w.URL == "" {
		return Webhook{}, errors.New("url required")
	}
	if w.Kind == "" {
		w.Kind = KindGeneric
	}
	buf := make([]byte, 6)
	_, _ = rand.Read(buf)
	w.ID = hex.EncodeToString(buf)
	w.CreatedAt = time.Now().UTC()
	s.wmu.Lock()
	defer s.wmu.Unlock()
	if s.persist != nil {
		if err := s.persist.Put(w.ID, w); err != nil {
			return Webhook{}, err
		}
	}
	s.mu.Lock()
	s.data[w.ID] = w
	s.mu.Unlock()
	return w, nil
}

func (s *MemoryStore) Update(id string, w Webhook) (Webhook, error) {
	s.wmu.Lock()
	defer s.wmu.Unlock()
	cur, err := s.Get(id)
	if err != nil {
		return Webhook{}, err
	}
	if strings.TrimSpace(w.Label) != "" {
		cur.Label = strings.TrimSpace(w.Label)
	}
	if w.Kind != "" {
		cur.Kind = w.Kind
	}
	if w.URL != "" {
		cur.URL = w.URL
	}
	if w.Headers != nil {
		cur.Headers = w.Headers
	}
	if w.PayloadTemplate != "" {
		cur.PayloadTemplate = w.PayloadTemplate
	}
	if strings.TrimSpace(w.ProjectKey) != "" {
		cur.ProjectKey = strings.TrimSpace(w.ProjectKey)
	}
	if strings.TrimSpace(w.IssueType) != "" {
		cur.IssueType = strings.TrimSpace(w.IssueType)
	}
	if strings.TrimSpace(w.AuthToken) != "" {
		cur.AuthToken = strings.TrimSpace(w.AuthToken)
	}
	cur.Enabled = w.Enabled
	cur.Rule = w.Rule
	// Correlation fields: a non-zero value means the caller wanted it.
	// Zero means "keep current" to preserve backward compatibility with
	// partial PATCH-style updates.
	if w.ThresholdCount > 0 || w.ThresholdWindowSecs > 0 || w.CooldownSecs > 0 {
		cur.ThresholdCount = w.ThresholdCount
		cur.ThresholdWindowSecs = w.ThresholdWindowSecs
		cur.CooldownSecs = w.CooldownSecs
	}
	if s.persist != nil {
		if err := s.persist.Put(id, cur); err != nil {
			return Webhook{}, err
		}
	}
	s.mu.Lock()
	s.data[id] = cur
	s.mu.Unlock()
	return cur, nil
}

func (s *MemoryStore) Delete(id string) error {
	s.wmu.Lock()
	defer s.wmu.Unlock()
	if _, err := s.Get(id); err != nil {
		return err
	}
	if s.persist != nil {
		if err := s.persist.Delete(id); err != nil {
			return err
		}
	}
	s.mu.Lock()
	delete(s.data, id)
	s.mu.Unlock()
	return nil
}

// Test posts a probe payload to the webhook URL and returns the HTTP status.
func (s *MemoryStore) Test(ctx context.Context, id string) (int, error) {
	s.mu.RLock()
	w, ok := s.data[id]
	s.mu.RUnlock()
	if !ok {
		return 0, ErrNotFound
	}
	body, err := RenderTestPayload(w)
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.URL, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", WebhookContentType(w.Kind))
	// Per-kind auto-authentication: Opsgenie's Alert API requires
	// `Authorization: GenieKey <token>`; we inject it from AuthToken when
	// the operator hasn't set the header manually. PagerDuty puts its
	// routing_key in the JSON body (see RenderTestPayload).
	if w.Kind == KindOpsgenie && w.AuthToken != "" {
		if _, ok := w.Headers["Authorization"]; !ok {
			req.Header.Set("Authorization", "GenieKey "+w.AuthToken)
		}
	}
	for k, v := range w.Headers {
		req.Header.Set(k, v)
	}
	resp, err := s.http.Do(req)
	if err != nil {
		return 0, fmt.Errorf("test failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	s.mu.Lock()
	if cur, ok := s.data[id]; ok {
		cur.LastFired = time.Now().UTC()
		s.data[id] = cur
	}
	s.mu.Unlock()
	return resp.StatusCode, nil
}

// Notify fires a webhook with correlation suppression. The correlationKey
// groups similar events (e.g. "credential_leak/high"). If the webhook has
// threshold_count > 0, ShouldFire is checked against the correlation
// engine; events below the threshold or within the cooldown period are
// silently suppressed (no error returned).
//
// When fired, "correlated_count": N is injected into the payload so SIEM
// receivers have situational awareness of the burst size.
//
// Returns (fired, correlatedCount, statusCode, error).
func (s *MemoryStore) Notify(ctx context.Context, id, correlationKey string, payload map[string]interface{}) (bool, int, int, error) {
	s.mu.RLock()
	w, ok := s.data[id]
	s.mu.RUnlock()
	if !ok {
		return false, 0, 0, ErrNotFound
	}
	if !w.Enabled {
		return false, 0, 0, nil
	}

	// Correlation check.
	shouldFire, correlatedCount := s.correlator.ShouldFire(
		id, correlationKey,
		w.ThresholdCount, w.ThresholdWindowSecs, w.CooldownSecs,
	)
	if !shouldFire {
		return false, 0, 0, nil
	}

	// Inject correlated_count into the payload.
	if payload == nil {
		payload = make(map[string]interface{})
	}
	payload["correlated_count"] = correlatedCount

	body, err := json.Marshal(payload)
	if err != nil {
		return false, correlatedCount, 0, fmt.Errorf("notify marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.URL, bytes.NewReader(body))
	if err != nil {
		return false, correlatedCount, 0, fmt.Errorf("notify build request: %w", err)
	}
	req.Header.Set("Content-Type", WebhookContentType(w.Kind))
	if w.Kind == KindOpsgenie && w.AuthToken != "" {
		if _, ok := w.Headers["Authorization"]; !ok {
			req.Header.Set("Authorization", "GenieKey "+w.AuthToken)
		}
	}
	for k, v := range w.Headers {
		req.Header.Set(k, v)
	}

	resp, err := s.http.Do(req)
	if err != nil {
		return false, correlatedCount, 0, fmt.Errorf("notify post: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	s.mu.Lock()
	if cur, ok := s.data[id]; ok {
		cur.LastFired = time.Now().UTC()
		s.data[id] = cur
	}
	s.mu.Unlock()

	return true, correlatedCount, resp.StatusCode, nil
}
