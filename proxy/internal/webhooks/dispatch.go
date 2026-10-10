package webhooks

import (
	"context"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/yatuk/tamga/internal/egress"
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

// Isolation between destinations. One queue and one pool serve them all, so
// two limits keep a destination that hangs from holding up the rest.
//
// A destination occupies at most half the workers (2 of 4). Alerts for it
// that arrive meanwhile wait in a line of its own, not in a worker.
//
// After defaultBreakerThreshold deliveries in a row have failed on the
// network or with a 5xx, the destination is skipped for
// defaultBreakerCooldown. Then one alert is let through with a single
// attempt: it closes the breaker, or opens it for another cooldown.
//
// Three failures, not one or two, so a receiver that restarts while two
// alerts are in flight is not cut off for it. Thirty seconds, so a receiver
// that comes back is alerted again within half a minute.
//
// Worst case, for a destination that accepts the connection and never
// answers: a delivery lasts 3 x 5s plus 0.5s and 2s of backoff, 17.5s. Two
// run at once, so failures one and two land at 17.5s and the third at 35s.
// Until then it holds two workers and the other two serve everyone else.
// After that it costs one worker 5s in every 35s, a twenty-eighth of the
// pool, so some two dozen dead destinations still leave room for live ones.
//
// The state is bounded. A healthy destination keeps none between alerts. A
// failing one keeps an entry until it recovers, has been idle for
// destStateTTL, or was deleted from the store; there are never more than
// maxDestStates entries, and never more than a queue's worth of alerts
// waiting in the per-destination lines.
const (
	defaultBreakerThreshold = 3
	defaultBreakerCooldown  = 30 * time.Second
	destStateTTL            = 10 * time.Minute
	destSweepEvery          = time.Minute
	maxDestStates           = 1024
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
	// Guard judges every address a delivery connects to. Nil is the strict
	// default: https only, nothing internal.
	Guard *Guard
	// BreakerThreshold is how many deliveries in a row must fail before a
	// destination is skipped, and BreakerCooldown for how long.
	BreakerThreshold int
	BreakerCooldown  time.Duration

	// now is the clock; tests replace it.
	now func() time.Time
}

// KindStats is the delivery counters of one destination kind.
type KindStats struct {
	Kind       string
	Sent       int64
	Failed     int64
	Dropped    int64
	Suppressed int64
	// Blocked counts requests the egress guard refused, probes included.
	Blocked int64
	// Skipped counts alerts not attempted because the destination's breaker
	// was open.
	Skipped int64
}

type kindCounters struct {
	sent, failed, dropped, suppressed, blocked, skipped atomic.Int64
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

// key names the destination for isolation: a saved destination by its id, a
// rule's notify URL by the URL.
func (j delivery) key() string {
	if j.destID != "" {
		return "dest:" + j.destID
	}
	return "rule:" + j.url
}

// destState is what the dispatcher remembers about one destination.
type destState struct {
	// inflight is how many workers are delivering to it now.
	inflight int
	// pending are alerts waiting for one of those workers.
	pending []delivery
	// fails counts deliveries that failed in a row.
	fails int
	// openUntil is when a skipped destination is tried again; zero when the
	// breaker is closed.
	openUntil time.Time
	// probing is set while the one trial delivery is in flight.
	probing  bool
	lastUsed time.Time
}

// outcome is what a delivery means to the destination's breaker.
type outcome int

const (
	// outcomeNeutral says nothing about the receiver: shutdown.
	outcomeNeutral outcome = iota
	// outcomeAlive: the receiver answered, or was never contacted.
	outcomeAlive
	// outcomeDown: timeout, network error or 5xx.
	outcomeDown
)

// Dispatcher delivers alerts to saved destinations and to policy rules'
// notify URLs. Callers hand it work and return at once: everything that
// touches the network happens on its own workers, behind a bounded queue.
// When the queue is full the alert is dropped and counted.
type Dispatcher struct {
	store *MemoryStore
	cfg   DispatcherConfig
	guard *Guard
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

	// destMu guards the per-destination state below.
	destMu    sync.Mutex
	dests     map[string]*destState
	parked    int
	lastSweep time.Time
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
	if cfg.Guard == nil {
		cfg.Guard = NewGuard(GuardConfig{})
	}
	if cfg.BreakerThreshold <= 0 {
		cfg.BreakerThreshold = defaultBreakerThreshold
	}
	if cfg.BreakerCooldown <= 0 {
		cfg.BreakerCooldown = defaultBreakerCooldown
	}
	if cfg.now == nil {
		cfg.now = time.Now
	}
	ctx, cancel := context.WithCancel(context.Background())
	d := &Dispatcher{
		store:  store,
		cfg:    cfg,
		guard:  cfg.Guard,
		log:    log.With().Str("component", "alerts").Logger(),
		queue:  make(chan delivery, cfg.QueueSize),
		ctx:    ctx,
		cancel: cancel,
		rules:  NewCorrelationEngine(0),
		stats:  make(map[Kind]*kindCounters),
		dests:  make(map[string]*destState),
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
		if e.Direction == "output" {
			// A check of an answer alone (the LiteLLM route).
			a.Direction = "output"
		}
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
	// Probes are counted by the store, which makes them.
	var probes map[Kind]int64
	if d.store != nil {
		probes = d.store.BlockedProbes()
	}
	d.statsMu.Lock()
	out := make([]KindStats, 0, len(d.stats)+len(probes))
	for k, c := range d.stats {
		out = append(out, KindStats{
			Kind:       string(k),
			Sent:       c.sent.Load(),
			Failed:     c.failed.Load(),
			Dropped:    c.dropped.Load(),
			Suppressed: c.suppressed.Load(),
			Blocked:    c.blocked.Load() + probes[k],
			Skipped:    c.skipped.Load(),
		})
		delete(probes, k)
	}
	d.statsMu.Unlock()
	for k, n := range probes {
		out = append(out, KindStats{Kind: string(k), Blocked: n})
	}
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
	d.drop(j, reason)
}

// drop counts an alert that was never attempted. One line per hundred: a
// full queue is a burst by definition.
func (d *Dispatcher) drop(j delivery, reason string) {
	if n := d.counters(j.kind).dropped.Add(1); n%100 == 1 {
		d.logFor(j, d.log.Warn()).Int64("dropped", n).Str("reason", reason).Msg("alert dropped")
	}
}

func (d *Dispatcher) worker() {
	defer d.wg.Done()
	for j := range d.queue {
		d.run(j)
	}
}

// perDestination is how many workers one destination may hold at once.
func (d *Dispatcher) perDestination() int {
	if n := d.cfg.Workers / 2; n > 1 {
		return n
	}
	return 1
}

// run delivers j, unless its destination already holds its share of the
// workers: then j joins that destination's line and this worker goes back to
// the queue. A worker that finishes a delivery takes the next from the line
// before it returns, so nothing in a line is left behind.
func (d *Dispatcher) run(j delivery) {
	key := j.key()
	for {
		d.destMu.Lock()
		st := d.state(key)
		if st.inflight >= d.perDestination() {
			full := d.parked >= d.cfg.QueueSize
			if !full {
				st.pending = append(st.pending, j)
				d.parked++
			}
			d.destMu.Unlock()
			if full {
				d.drop(j, "destination_busy")
			}
			return
		}
		st.inflight++
		attempts, trial, skip := d.admit(st)
		d.destMu.Unlock()

		result := outcomeAlive
		if skip {
			if n := d.counters(j.kind).skipped.Add(1); n%100 == 1 {
				d.logFor(j, d.log.Warn()).Int64("skipped", n).Msg("alert skipped, destination is failing")
			}
		} else {
			result = d.deliver(j, attempts)
		}

		d.destMu.Lock()
		st.inflight--
		if !skip {
			d.settle(st, result, trial)
		}
		if len(st.pending) == 0 {
			d.release(key, st)
			d.destMu.Unlock()
			return
		}
		j = st.pending[0]
		st.pending[0] = delivery{}
		st.pending = st.pending[1:]
		d.parked--
		d.destMu.Unlock()
	}
}

// state returns the entry for a destination, making one if need be. The
// caller holds destMu.
func (d *Dispatcher) state(key string) *destState {
	now := d.cfg.now()
	if now.Sub(d.lastSweep) >= destSweepEvery {
		d.sweep(now)
	}
	st, ok := d.dests[key]
	if !ok {
		if len(d.dests) >= maxDestStates {
			d.sweep(now)
		}
		if len(d.dests) >= maxDestStates {
			d.evictOldest()
		}
		st = &destState{}
		d.dests[key] = st
	}
	st.lastUsed = now
	return st
}

// admit decides what a delivery about to start may do: the usual attempts,
// one attempt as the trial of a destination whose cooldown is over, or
// nothing at all. The caller holds destMu.
func (d *Dispatcher) admit(st *destState) (attempts int, trial, skip bool) {
	if st.openUntil.IsZero() {
		return d.cfg.MaxAttempts, false, false
	}
	if st.probing || d.cfg.now().Before(st.openUntil) {
		return 0, false, true
	}
	st.probing = true
	return 1, true, false
}

// settle records how a delivery ended. The caller holds destMu.
func (d *Dispatcher) settle(st *destState, result outcome, trial bool) {
	if trial {
		st.probing = false
	}
	switch result {
	case outcomeAlive:
		st.fails = 0
		st.openUntil = time.Time{}
	case outcomeDown:
		st.fails++
		if trial || st.fails >= d.cfg.BreakerThreshold {
			st.openUntil = d.cfg.now().Add(d.cfg.BreakerCooldown)
		}
	}
}

// open reports whether a destination is being skipped; a delivery already
// in flight uses it to give up its remaining attempts.
func (d *Dispatcher) open(key string) bool {
	d.destMu.Lock()
	defer d.destMu.Unlock()
	st, ok := d.dests[key]
	return ok && !st.openUntil.IsZero()
}

// release forgets a destination that has nothing in flight, nothing waiting
// and no failures to remember. The caller holds destMu.
func (d *Dispatcher) release(key string, st *destState) {
	if st.inflight == 0 && len(st.pending) == 0 && st.fails == 0 && st.openUntil.IsZero() {
		delete(d.dests, key)
	}
}

// sweep forgets idle destinations that have not been used for destStateTTL
// or that the store no longer holds. The caller holds destMu.
func (d *Dispatcher) sweep(now time.Time) {
	d.lastSweep = now
	for key, st := range d.dests {
		if st.inflight > 0 || len(st.pending) > 0 {
			continue
		}
		gone := now.Sub(st.lastUsed) >= destStateTTL
		if id, saved := strings.CutPrefix(key, "dest:"); saved && !gone && d.store != nil {
			_, err := d.store.Get(id)
			gone = err != nil
		}
		if gone {
			delete(d.dests, key)
		}
	}
}

// evictOldest makes room by forgetting the idle destination used longest
// ago. Its breaker closes with it, which costs a few attempts at most. The
// caller holds destMu.
func (d *Dispatcher) evictOldest() {
	var oldest string
	var at time.Time
	for key, st := range d.dests {
		if st.inflight > 0 || len(st.pending) > 0 {
			continue
		}
		if oldest == "" || st.lastUsed.Before(at) {
			oldest, at = key, st.lastUsed
		}
	}
	if oldest != "" {
		delete(d.dests, oldest)
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

// deliver makes up to attempts tries and reports what the result says about
// the receiver.
func (d *Dispatcher) deliver(j delivery, attempts int) outcome {
	c := d.counters(j.kind)
	key := j.key()
	wait := d.cfg.Backoff
	var status int
	var class string
	for attempt := 1; attempt <= attempts; attempt++ {
		if d.ctx.Err() != nil {
			class = "shutdown"
			break
		}
		// Another delivery opened the breaker while this one was waiting.
		if attempt > 1 && d.open(key) {
			break
		}
		status, class = d.post(j)
		if class == "" {
			c.sent.Add(1)
			if j.destID != "" && d.store != nil {
				d.store.MarkFired(j.destID, time.Now().UTC())
			}
			d.logFor(j, d.log.Debug()).Int("status", status).Int("attempt", attempt).Msg("alert sent")
			return outcomeAlive
		}
		if class == "blocked" {
			// Kind and id only, not even the host: where the guard said no
			// is not for the log. One line per hundred, because a refused
			// destination is refused every time until someone changes it.
			if n := c.blocked.Add(1); n%100 == 1 {
				e := d.log.Warn().Str("kind", string(j.kind))
				if j.destID != "" {
					e = e.Str("destination_id", j.destID)
				}
				e.Int64("blocked", n).Str("error", class).Msg("alert destination refused by the egress guard")
			}
			return outcomeAlive
		}
		// A 4xx would get the same answer again, a URL that does not parse
		// will not parse next time either, and a redirect loop stays one.
		if (status >= 300 && status < 500) || class == "bad_url" || class == "redirects" {
			break
		}
		if attempt == attempts {
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
	switch {
	case class == "shutdown" || class == "canceled":
		return outcomeNeutral
	case (status >= 300 && status < 500) || class == "bad_url" || class == "redirects":
		return outcomeAlive
	}
	return outcomeDown
}

// post makes one attempt. It returns the HTTP status (0 when there was no
// answer) and, unless the answer was a 2xx, a short class of what went
// wrong. The error text itself is not kept: it contains the URL.
func (d *Dispatcher) post(j delivery) (int, string) {
	ctx, cancel := context.WithTimeout(d.ctx, d.cfg.AttemptTimeout)
	defer cancel()
	status, err := d.guard.Post(ctx, j.url, j.header, j.body)
	if err != nil {
		return 0, egress.Class(err)
	}
	if status >= 200 && status < 300 {
		return status, ""
	}
	return status, "http_status"
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
