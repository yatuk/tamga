package api

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/yatuk/tamga/internal/auth"
	"github.com/yatuk/tamga/internal/incidents"
)

// Kinds of the audit entries a refused management request leaves.
const (
	// auditRefused is the first refusal of a source for a reason.
	auditRefused = "auth.refused"
	// auditRefusedRepeated says how many more followed within the window.
	auditRefusedRepeated = "auth.refused.repeated"
)

// Reasons a management request is refused, as the audit entry names them.
// A session token that is valid and opens nothing is refused with the code
// its answer carries (codeSessionRevoked, codeAccountNotAssigned), and a
// role without the permission with codePermissionDenied.
const (
	// refusedInvalidKey: X-Tamga-Admin-Key was sent and is neither the
	// admin key nor a live API key (a wrong, mistyped or revoked key).
	refusedInvalidKey = "invalid_key"
	// refusedInvalidToken: a bearer token that is malformed or not signed
	// with this deployment's secret.
	refusedInvalidToken = "invalid_token"
	// refusedTokenExpired: a bearer token this deployment signed, past its
	// expiry.
	refusedTokenExpired = "token_expired"
	// refusedProxyKey: a live key of the proxy path, which opens nothing
	// here.
	refusedProxyKey = "proxy_key"
)

const (
	// refusalWindow is how long the refusals of one source for one reason
	// are counted together.
	refusalWindow = time.Minute
	// refusalSources is how many source-and-reason pairs are followed at a
	// time. What does not fit is counted in one entry for all of it.
	refusalSources = 32
	// refusalQueue is how many entries may wait for the audit log.
	refusalQueue = 256
	// refusalTargetMax bounds the route text of an entry: the path of a
	// request that was let in nowhere is the sender's to write.
	refusalTargetMax = 160
)

// refusalLog writes refused management requests to the audit log.
//
// A refusal costs its sender nothing, so the log must not grow with them.
// The first refusal of a source for a reason is written at once, with its
// route. The ones that follow within refusalWindow are counted, and one
// entry says how many there were when the window ends. At most
// refusalSources pairs are followed at a time; the refusals of any further
// source are counted together in one entry. That bounds what refusals can
// add at 2*refusalSources+1 entries per window however many arrive, and at
// two per window for one source and reason, while the first attempt of a
// source is still on record with what it asked for.
//
// The request does not wait for the audit log. An entry is queued, and one
// goroutine that lives only while the queue has entries appends them, so a
// slow store delays the entries and not the answers. A queue that is full
// drops the entry and says so in the process log.
type refusalLog struct {
	audit  *incidents.AuditRing
	source func(*http.Request) string
	now    func() time.Time
	// after runs f once d has passed. It is time.AfterFunc outside tests.
	after  func(d time.Duration, f func())
	window time.Duration
	limit  int

	mu       sync.Mutex
	open     map[refusalKey]*refusalRun
	queue    []incidents.AuditEntry
	draining bool
	// pending counts queued entries not yet appended; tests wait on it.
	pending sync.WaitGroup
}

type refusalKey struct{ source, reason string }

// refusalRun is one source and reason within one window.
type refusalRun struct {
	actor   string
	repeats int
}

// unlisted is the run of everything that had no room of its own.
var unlisted = refusalKey{}

func newRefusalLog(audit *incidents.AuditRing, source func(*http.Request) string) *refusalLog {
	if source == nil {
		source = peerAddr
	}
	return &refusalLog{
		audit:  audit,
		source: source,
		now:    time.Now,
		after:  func(d time.Duration, f func()) { time.AfterFunc(d, f) },
		window: refusalWindow,
		limit:  refusalSources,
		open:   map[refusalKey]*refusalRun{},
	}
}

// peerAddr is the address the connection came from.
func peerAddr(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// refusal is what is recorded of one refused request. It has no field for
// the credential that was presented.
type refusal struct {
	reason string
	status int
	// actor is who the request was verified as, when it was: a session
	// token or a key that is valid and opens nothing. Empty otherwise.
	actor string
	// target is the route when one matched; the method and path otherwise.
	target string
	// role and scope are set for a permission denial.
	role, scope string
}

type refusalProbeKey struct{}

// asProbe marks a request that asks adminAuth a question on behalf of a
// public route (see healthFull). Its refusal is that route's ordinary
// answer and is not recorded.
func asProbe(r *http.Request) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), refusalProbeKey{}, true))
}

// note records a refusal. It returns without waiting for the audit log.
func (l *refusalLog) note(r *http.Request, f refusal) {
	if l == nil || l.audit == nil || r.Context().Value(refusalProbeKey{}) != nil {
		return
	}
	if f.actor == "" {
		f.actor = actorUnknown
	}
	if f.target == "" {
		f.target = r.Method + " " + clipTarget(r.URL.Path)
	}
	key := refusalKey{source: l.source(r), reason: f.reason}

	l.mu.Lock()
	defer l.mu.Unlock()
	if run, ok := l.open[key]; ok {
		run.repeats++
		return
	}
	if len(l.open) >= l.limit {
		// No room to follow this source. It is counted with the others.
		if _, ok := l.open[unlisted]; !ok {
			l.open[unlisted] = &refusalRun{actor: actorUnknown}
			l.closeLater(unlisted)
		}
		l.open[unlisted].repeats++
		return
	}
	l.open[key] = &refusalRun{actor: f.actor}
	l.closeLater(key)

	detail := map[string]interface{}{
		"reason": f.reason,
		"source": key.source,
		"status": f.status,
	}
	if f.scope != "" {
		detail["required_permission"] = f.scope
	}
	if f.role != "" {
		detail["role"] = f.role
	}
	l.enqueue(incidents.AuditEntry{
		Timestamp: l.now().UTC(),
		Kind:      auditRefused,
		Actor:     f.actor,
		Target:    f.target,
		Detail:    detail,
	})
}

// closeLater ends the run of key when its window has passed. The caller
// holds l.mu.
func (l *refusalLog) closeLater(key refusalKey) {
	l.after(l.window, func() { l.close(key) })
}

// close ends a run and, when refusals were counted in it, says how many.
func (l *refusalLog) close(key refusalKey) {
	l.mu.Lock()
	defer l.mu.Unlock()
	run, ok := l.open[key]
	if !ok {
		return
	}
	delete(l.open, key)
	if run.repeats == 0 {
		return
	}
	detail := map[string]interface{}{
		"repeats":        run.repeats,
		"window_seconds": int(l.window / time.Second),
	}
	if key == unlisted {
		// Many sources, or one that changes its address: too many to
		// follow one by one.
		detail["sources"] = "not followed individually"
	} else {
		detail["reason"] = key.reason
		detail["source"] = key.source
	}
	l.enqueue(incidents.AuditEntry{
		Timestamp: l.now().UTC(),
		Kind:      auditRefusedRepeated,
		Actor:     run.actor,
		Detail:    detail,
	})
}

// enqueue hands an entry to the goroutine that appends, starting it when
// none is running. The caller holds l.mu.
func (l *refusalLog) enqueue(e incidents.AuditEntry) {
	if len(l.queue) >= refusalQueue {
		log.Warn().Str("component", "audit").Str("kind", e.Kind).
			Msg("refusal not written to the audit log: the entries before it are still waiting for the store")
		return
	}
	l.queue = append(l.queue, e)
	l.pending.Add(1)
	if !l.draining {
		l.draining = true
		go l.drain()
	}
}

// drain appends the queued entries in order and returns when none is left.
func (l *refusalLog) drain() {
	for {
		l.mu.Lock()
		if len(l.queue) == 0 {
			l.queue = nil
			l.draining = false
			l.mu.Unlock()
			return
		}
		e := l.queue[0]
		l.queue = l.queue[1:]
		l.mu.Unlock()

		l.audit.Append(e)
		l.pending.Done()
	}
}

// clipTarget bounds a path and keeps only printable ASCII of it.
func clipTarget(path string) string {
	if len(path) > refusalTargetMax {
		path = path[:refusalTargetMax]
	}
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r > 0x7e {
			return '?'
		}
		return r
	}, path)
}

// credentialRefusal names why a request that matched no credential was
// answered 401, or "" when it presented none: a request without a
// credential was refused nothing, and is not recorded.
//
// Only the kind of credential and the reason are derived. The key and the
// token are not kept, hashed or measured.
func credentialRefusal(r *http.Request, jwtSecret string) string {
	if r.Header.Get("X-Tamga-Admin-Key") != "" {
		return refusedInvalidKey
	}
	token := bearerToken(r)
	if token == "" {
		return ""
	}
	if secret := strings.TrimSpace(jwtSecret); secret != "" {
		if _, err := auth.ParseToken(token, []byte(secret)); errors.Is(err, auth.ErrTokenExpired) {
			return refusedTokenExpired
		}
	}
	return refusedInvalidToken
}

// refuses asks what ServeHTTP is about to decide for r: whether a route
// matches and the caller's role does not carry what it asks for. It decides
// nothing itself; it is how the refusal gets written down.
func (g *guard) refuses(r *http.Request) (pattern, scope string, refused bool) {
	_, pattern = g.mux.Handler(r)
	if pattern == "" {
		return "", "", false
	}
	scope, declared := g.scopes[pattern]
	if !declared || !scopeFitsMethod(r.Method, scope) {
		return pattern, scope, true
	}
	return pattern, scope, !roleHasScope(RoleFrom(r.Context()), scope)
}

// watch records the permission denials of g and then lets g answer.
func (l *refusalLog) watch(g *guard) http.Handler {
	if l == nil || l.audit == nil {
		return g
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if pattern, scope, refused := g.refuses(r); refused {
			l.note(r, refusal{
				reason: codePermissionDenied,
				status: http.StatusForbidden,
				actor:  ActorFrom(r.Context()),
				target: pattern,
				role:   RoleFrom(r.Context()),
				scope:  scope,
			})
		}
		g.ServeHTTP(w, r)
	})
}
