package webhooks

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/yatuk/tamga/internal/events"
	"github.com/yatuk/tamga/internal/scanner"
)

// KindRuleNotify labels deliveries to a policy rule's notify URL in metrics
// and logs. Those URLs are not saved destinations and have no kind of their
// own.
const KindRuleNotify Kind = "rule_notify"

// Defaults for a Dispatcher. Alerts are a few per incident, not per request,
// so the queue is sized to ride out a burst while a receiver is slow, and
// the worst case is bounded: at most queue + workers bodies are held, and
// one delivery occupies a worker for at most attempts x timeout plus the
// backoffs (about 17 seconds).
const (
	defaultQueueSize      = 256
	defaultWorkers        = 4
	defaultAttemptTimeout = 5 * time.Second
	defaultMaxAttempts    = 3
	defaultBackoff        = 500 * time.Millisecond
	defaultThresholdSecs  = 300
	blocksPerMinuteWindow = 60
	// ruleNotifyCooldownSecs spaces out a rule's notify URL: see NotifyURLs.
	ruleNotifyCooldownSecs = 60
)

// DispatcherConfig tunes a Dispatcher. A zero value takes the defaults.
type DispatcherConfig struct {
	// DefaultOrg names the organisation of an event that carries none.
	DefaultOrg string
	QueueSize  int
	Workers    int
	// AttemptTimeout bounds one HTTP attempt.
	AttemptTimeout time.Duration
	// MaxAttempts counts the first try. A network error or a 5xx is tried
	// again; a 4xx is not, because the same request would get the same
	// answer.
	MaxAttempts int
	// Backoff is the wait before the second attempt; it grows fourfold
	// before each one after.
	Backoff time.Duration
	Client  *http.Client
}

// KindStats is the delivery counters of one destination kind.
type KindStats struct {
	Kind       string
	Sent       int64
	Failed     int64
	Dropped    int64
	Suppressed int64
}

type kindCounters struct {
	sent, failed, dropped, suppressed atomic.Int64
}

// delivery is one rendered request waiting for a worker.
type delivery struct {
	kind Kind
	// destID is the saved destination's id; empty for a rule's notify URL.
	destID string
	url    string
	header http.Header
	body   []byte
}

// Dispatcher delivers alerts to saved destinations and to policy rules'
// notify URLs. Callers hand it work and return at once: everything that
// touches the network happens on its own workers, behind a bounded queue.
// When the queue is full the alert is dropped and counted.
type Dispatcher struct {
	store *MemoryStore
	cfg   DispatcherConfig
	http  *http.Client
	log   zerolog.Logger

	// mu guards closed and the close of queue against a late enqueue.
	mu     sync.RWMutex
	closed bool
	queue  chan delivery
	wg     sync.WaitGroup
	// ctx ends in-flight attempts and backoff waits once the drain is over.
	ctx    context.Context
	cancel context.CancelFunc

	// rules spaces out deliveries to rule notify URLs. It is apart from the
	// store's engine, which belongs to saved destinations and may be absent.
	rules *CorrelationEngine

	statsMu sync.Mutex
	stats   map[Kind]*kindCounters
}

// NewDispatcher starts the workers. store may be nil, in which case only
// rule notify URLs are served. Call Stop when the process shuts down.
func NewDispatcher(store *MemoryStore, cfg DispatcherConfig) *Dispatcher {
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = defaultQueueSize
	}
	if cfg.Workers <= 0 {
		cfg.Workers = defaultWorkers
	}
	if cfg.AttemptTimeout <= 0 {
		cfg.AttemptTimeout = defaultAttemptTimeout
	}
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = defaultMaxAttempts
	}
	if cfg.Backoff <= 0 {
		cfg.Backoff = defaultBackoff
	}
	client := cfg.Client
	if client == nil {
		client = &http.Client{Timeout: cfg.AttemptTimeout}
	}
	ctx, cancel := context.WithCancel(context.Background())
	d := &Dispatcher{
		store:  store,
		cfg:    cfg,
		http:   client,
		log:    log.With().Str("component", "alerts").Logger(),
		queue:  make(chan delivery, cfg.QueueSize),
		ctx:    ctx,
		cancel: cancel,
		rules:  NewCorrelationEngine(0),
		stats:  make(map[Kind]*kindCounters),
	}
	for i := 0; i < cfg.Workers; i++ {
		d.wg.Add(1)
		go d.worker()
	}
	return d
}

// AlertFromEvent turns a bus event into an alert, and reports false for an
// event that is not a decision worth telling anyone about (a pass, a log, a
// hint). It copies type, category and severity out of each finding and
// leaves the event's body and the findings' matches behind.
func AlertFromEvent(e events.Event, defaultOrg string) (Alert, bool) {
	a := Alert{
		RequestID:  e.RequestID,
		Timestamp:  e.Timestamp,
		OrgID:      strings.TrimSpace(e.OrgID),
		Provider:   e.Provider,
		Model:      e.Model,
		EventType:  e.EventType,
		UserID:     e.UserID,
		InputRisk:  e.InputRisk.Percentage,
		OutputRisk: e.OutputRisk.Percentage,
	}
	if a.OrgID == "" {
		a.OrgID = defaultOrg
	}
	switch e.EventType {
	case "request_blocked", "request_scanned":
		a.Direction = "input"
		a.Action = strings.ToUpper(e.Action)
		a.Findings = AlertFindings(e.Findings)
	case "output_scanned":
		a.Direction = "output"
		a.Action = strings.ToUpper(e.OutputAction)
		a.Findings = AlertFindings(e.OutputFindings)
	default:
		return Alert{}, false
	}
	switch a.Action {
	case "BLOCK", "STRIP", "REDACT", "WARN":
		return a, true
	}
	return Alert{}, false
}

// AlertFindings keeps what kind of thing each finding is and drops the rest.
func AlertFindings(fs []scanner.Finding) []AlertFinding {
	out := make([]AlertFinding, len(fs))
	for i, f := range fs {
		out[i] = AlertFinding{Type: f.Type, Category: f.Category, Severity: f.Severity}
	}
	return out
}

// HandleEvent is the event bus subscriber. The bus runs its subscribers one
// after another on a single goroutine, so this does only in-memory work
// (filter, correlate, render) and leaves the sending to the queue.
func (d *Dispatcher) HandleEvent(e events.Event) {
	if d == nil || d.store == nil {
		return
	}
	a, ok := AlertFromEvent(e, d.cfg.DefaultOrg)
	if !ok {
		return
	}
	a = a.clean()
	a.DedupKey = "tamga-" + correlationKey(a)
	for _, w := range d.store.List() {
		if !w.Enabled || !wants(w, a) {
			continue
		}
		fire, n := d.correlate(w, a)
		if !fire {
			d.counters(w.Kind).suppressed.Add(1)
			continue
		}
		out := a
		out.CorrelatedCount = n
		body, err := Render(w, out)
		if err != nil {
			d.counters(w.Kind).failed.Add(1)
			d.log.Warn().Str("kind", string(w.Kind)).Str("destination_id", w.ID).Str("error", "render").Msg("alert not sent")
			continue
		}
		d.enqueue(delivery{kind: w.Kind, destID: w.ID, url: w.URL, header: deliveryHeaders(w), body: body})
	}
}

// NotifyURLs queues an alert for a policy rule's notify URLs. It returns at
// once; the caller is a request in flight.
func (d *Dispatcher) NotifyURLs(urls []string, a Alert) {
	if d == nil || len(urls) == 0 {
		return
	}
	body, err := RenderRuleNotify(a)
	if err != nil {
		d.counters(KindRuleNotify).failed.Add(1)
		return
	}
	a = a.clean()
	for _, u := range urls {
		// A policy can hold a placeholder that was never filled in
		// ("${SLACK_WEBHOOK_URL}"). That is not somewhere to post to.
		if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
			continue
		}
		// A rule's URL has no settings of its own, and a rule that redacts
		// can match every request. One message a minute for the same thing
		// (organisation, action, category) is what a channel can use.
		if fire, _ := d.rules.ShouldFire("rule:"+u, correlationKey(a), 1, ruleNotifyCooldownSecs, ruleNotifyCooldownSecs); !fire {
			d.counters(KindRuleNotify).suppressed.Add(1)
			continue
		}
		h := http.Header{}
		h.Set("Content-Type", "application/json")
		d.enqueue(delivery{kind: KindRuleNotify, url: u, header: h, body: body})
	}
}

// wants reports whether a destination is told about an alert.
//
// The destination record can filter on severity (rule.severity_at_least) and
// nothing else, so the actions are fixed: BLOCK, STRIP and WARN go to every
// enabled destination. REDACT goes only to one with a threshold above 1 or
// a cooldown set, because an address redacted on every request would
// otherwise be a message on every request.
func wants(w Webhook, a Alert) bool {
	switch a.Action {
	case "BLOCK", "STRIP", "WARN":
	case "REDACT":
		if w.ThresholdCount <= 1 && w.CooldownSecs <= 0 {
			return false
		}
	default:
		return false
	}
	if w.Rule != nil {
		if least := severityRank(w.Rule.SeverityAtLeast); least > 0 && a.level() < least {
			return false
		}
	}
	return true
}

// correlationKey groups alerts that say the same thing: same organisation,
// same side of the exchange, same action, same category of the most severe
// finding. The correlation engine adds the destination id itself. The
// request id is left out on purpose, or no two alerts would ever correlate.
func correlationKey(a Alert) string {
	category := "policy"
	if p, ok := a.primary(); ok && p.Category != "" {
		category = p.Category
	}
	return strings.Join([]string{a.OrgID, a.Direction, a.Action, category}, "|")
}

// correlate applies the destination's own rate settings and reports whether
// the alert goes out and how many events it stands for.
func (d *Dispatcher) correlate(w Webhook, a Alert) (bool, int) {
	ce := d.store.Correlator()
	// rule.blocks_per_minute: blocks are held back until more than N have
	// been seen within a minute, then one alert goes out and the count
	// starts again.
	if w.Rule != nil && w.Rule.BlocksPerMinute > 0 && a.Action == "BLOCK" {
		if ok, _ := ce.ShouldFire(w.ID, "blocks_per_minute|"+a.OrgID, w.Rule.BlocksPerMinute+1, blocksPerMinuteWindow, 0); !ok {
			return false, 0
		}
	}
	count, window, cooldown := w.ThresholdCount, w.ThresholdWindowSecs, w.CooldownSecs
	// The engine skips the cooldown when there is no threshold. A cooldown
	// on its own means "at most one per period", which is a threshold of 1.
	if count <= 0 && cooldown > 0 {
		count = 1
	}
	if count > 0 && window <= 0 {
		window = defaultThresholdSecs
	}
	return ce.ShouldFire(w.ID, correlationKey(a), count, window, cooldown)
}

func (d *Dispatcher) counters(k Kind) *kindCounters {
	d.statsMu.Lock()
	defer d.statsMu.Unlock()
	c, ok := d.stats[k]
	if !ok {
		c = &kindCounters{}
		d.stats[k] = c
	}
	return c
}

// Stats returns the counters by destination kind, sorted by kind.
func (d *Dispatcher) Stats() []KindStats {
	if d == nil {
		return nil
	}
	d.statsMu.Lock()
	out := make([]KindStats, 0, len(d.stats))
	for k, c := range d.stats {
		out = append(out, KindStats{
			Kind:       string(k),
			Sent:       c.sent.Load(),
			Failed:     c.failed.Load(),
			Dropped:    c.dropped.Load(),
			Suppressed: c.suppressed.Load(),
		})
	}
	d.statsMu.Unlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Kind < out[j].Kind })
	return out
}

// enqueue never waits: a full queue, or a dispatcher that has stopped, drops
// the delivery and counts it.
func (d *Dispatcher) enqueue(j delivery) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	reason := "stopped"
	if !d.closed {
		select {
		case d.queue <- j:
			return
		default:
		}
		reason = "queue_full"
	}
	// One line per hundred: a full queue is a burst by definition.
	if n := d.counters(j.kind).dropped.Add(1); n%100 == 1 {
		d.logFor(j, d.log.Warn()).Int64("dropped", n).Str("reason", reason).Msg("alert dropped")
	}
}

func (d *Dispatcher) worker() {
	defer d.wg.Done()
	for j := range d.queue {
		d.deliver(j)
	}
}

// logFor adds what may be logged about a destination: its kind and its id.
// The URL is a credential (the path of a Slack webhook is the secret), so
// only the host of a rule's notify URL is named, which has no id to go by.
func (d *Dispatcher) logFor(j delivery, e *zerolog.Event) *zerolog.Event {
	e = e.Str("kind", string(j.kind))
	if j.destID != "" {
		return e.Str("destination_id", j.destID)
	}
	if u, err := url.Parse(j.url); err == nil {
		e = e.Str("host", u.Hostname())
	}
	return e
}

func (d *Dispatcher) deliver(j delivery) {
	c := d.counters(j.kind)
	wait := d.cfg.Backoff
	var status int
	var class string
	for attempt := 1; attempt <= d.cfg.MaxAttempts; attempt++ {
		if d.ctx.Err() != nil {
			class = "shutdown"
			break
		}
		status, class = d.post(j)
		if class == "" {
			c.sent.Add(1)
			if j.destID != "" && d.store != nil {
				d.store.MarkFired(j.destID, time.Now().UTC())
			}
			d.logFor(j, d.log.Debug()).Int("status", status).Int("attempt", attempt).Msg("alert sent")
			return
		}
		// A 4xx would get the same answer again, and a URL that does not
		// parse will not parse next time either.
		if (status >= 300 && status < 500) || class == "bad_url" {
			break
		}
		if attempt == d.cfg.MaxAttempts {
			break
		}
		t := time.NewTimer(wait)
		select {
		case <-t.C:
		case <-d.ctx.Done():
			t.Stop()
		}
		wait *= 4
	}
	c.failed.Add(1)
	d.logFor(j, d.log.Warn()).Int("status", status).Str("error", class).Msg("alert not delivered")
}

// post makes one attempt. It returns the HTTP status (0 when there was no
// answer) and, unless the answer was a 2xx, a short class of what went
// wrong. The error text itself is not kept: it contains the URL.
func (d *Dispatcher) post(j delivery) (int, string) {
	ctx, cancel := context.WithTimeout(d.ctx, d.cfg.AttemptTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, j.url, bytes.NewReader(j.body))
	if err != nil {
		return 0, "bad_url"
	}
	req.Header = j.header.Clone()
	resp, err := d.http.Do(req)
	if err != nil {
		return 0, errClass(err)
	}
	// The body is read so the connection can be reused, and thrown away: a
	// receiver's answer is not ours to log.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	_ = resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return resp.StatusCode, ""
	}
	return resp.StatusCode, "http_status"
}

func errClass(err error) string {
	var netErr net.Error
	var dnsErr *net.DNSError
	var certErr *tls.CertificateVerificationError
	switch {
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.As(err, &dnsErr):
		return "dns"
	case errors.As(err, &certErr):
		return "tls"
	case errors.As(err, &netErr) && netErr.Timeout():
		return "timeout"
	}
	return "network"
}

// Stop refuses new work, gives what is queued and in flight up to drain to
// finish, then cuts the rest off and waits for the workers to return.
func (d *Dispatcher) Stop(drain time.Duration) {
	if d == nil {
		return
	}
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return
	}
	d.closed = true
	close(d.queue)
	d.mu.Unlock()

	done := make(chan struct{})
	go func() {
		d.wg.Wait()
		close(done)
	}()
	t := time.NewTimer(drain)
	defer t.Stop()
	select {
	case <-done:
	case <-t.C:
		d.cancel()
		<-done
	}
	d.cancel()
}
