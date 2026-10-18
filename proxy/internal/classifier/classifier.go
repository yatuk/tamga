// Package classifier is the proxy's side of the inline prompt-injection
// classifier: a gRPC client for the classifier service, a cache so text the
// proxy has already seen is not scored again, and a breaker so a service
// that is down costs nothing after the first few failures.
//
// The classifier is on the decision path. Everything here is built so a
// request waits for it at most once, for at most the deadline it is given.
package classifier

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"

	"github.com/yatuk/tamga/internal/errkind"
	pb "github.com/yatuk/tamga/proto/classifier/v1"
)

// Result is the classifier's answer for one text.
type Result struct {
	// Score is the probability that the text is an injection attempt.
	Score float64
	// Truncated means the text was too long to score whole; a low Score is
	// then not a verdict.
	Truncated bool
	// Model names the model that gave Score.
	Model string
	// Cached says the result came from the Guard's cache: the text was
	// scored for an earlier request.
	Cached bool

	// The shadow model's answer, when the service runs one: a candidate
	// scored next to the model in service, recorded and never acted on.
	// ShadowModel is empty when the service runs none. HasShadow says
	// ShadowScore was given; when it was not, ShadowSkipped says why
	// ("late", "busy", "failed"). A text answered from the cache carries
	// what the shadow said when the text was first scored.
	ShadowModel     string
	HasShadow       bool
	ShadowScore     float64
	ShadowTruncated bool
	ShadowSkipped   string
}

// Scorer scores texts. The gRPC client is one; tests use their own.
type Scorer interface {
	Score(ctx context.Context, requestID string, texts []string) ([]Result, error)
}

// ErrOpen is returned while the breaker is open: the service failed
// repeatedly and is not being asked again yet.
var ErrOpen = errkind.New("circuit_open", "classifier: unavailable (circuit open)")

// ── gRPC client ──────────────────────────────────────────────────────────

// Client calls the classifier service.
type Client struct {
	conn *grpc.ClientConn
	stub pb.ClassifierServiceClient
}

// NewClient connects to addr (e.g. "classifier:50052"). An empty addr
// returns nil: no classifier is configured. extra is applied after the
// client's own options: the service token and TLS from
// grpcauth.DialOptions go there.
func NewClient(addr string, extra ...grpc.DialOption) (*Client, error) {
	if addr == "" {
		return nil, nil
	}
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                30 * time.Second,
			Timeout:             5 * time.Second,
			PermitWithoutStream: true,
		}),
	}
	conn, err := grpc.NewClient(addr, append(opts, extra...)...)
	if err != nil {
		return nil, fmt.Errorf("classifier: dial %s: %w", addr, err)
	}
	return &Client{conn: conn, stub: pb.NewClassifierServiceClient(conn)}, nil
}

func (c *Client) Score(ctx context.Context, requestID string, texts []string) ([]Result, error) {
	resp, err := c.stub.Classify(ctx, &pb.ClassifyRequest{RequestId: requestID, Texts: texts})
	if err != nil {
		return nil, err
	}
	if len(resp.Scores) != len(texts) {
		return nil, fmt.Errorf("classifier: %d scores for %d texts", len(resp.Scores), len(texts))
	}
	out := make([]Result, len(texts))
	for i, s := range resp.Scores {
		out[i].Score = float64(s)
		if i < len(resp.Truncated) {
			out[i].Truncated = resp.Truncated[i]
		}
		out[i].Model = resp.Model
	}
	if resp.ShadowModel != "" {
		// The shadow never fails the call: scores that do not fit the texts
		// are dropped and reported as a failed shadow.
		skipped := resp.ShadowSkipped
		ok := len(resp.ShadowScores) == len(texts)
		if !ok && skipped == "" {
			skipped = "failed"
		}
		for i := range out {
			out[i].ShadowModel = resp.ShadowModel
			if !ok {
				out[i].ShadowSkipped = skipped
				continue
			}
			out[i].HasShadow = true
			out[i].ShadowScore = float64(resp.ShadowScores[i])
			if i < len(resp.ShadowTruncated) {
				out[i].ShadowTruncated = resp.ShadowTruncated[i]
			}
		}
	}
	return out, nil
}

// Health reports whether the service has a model loaded.
func (c *Client) Health(ctx context.Context) (available bool, model, reason string, err error) {
	h, err := c.HealthDetail(ctx)
	if err != nil {
		return false, "", "", err
	}
	return h.Available, h.Model, h.Reason, nil
}

// HealthStatus is what the service says about each of its models.
type HealthStatus struct {
	Available bool
	Model     string
	Reason    string
	// The topic model, which is optional and loaded on its own.
	TopicsAvailable bool
	TopicsModel     string
	TopicsReason    string
	// The shadow injection model, optional and off unless configured.
	ShadowAvailable bool
	ShadowModel     string
	ShadowReason    string
}

// HealthDetail reports both models.
func (c *Client) HealthDetail(ctx context.Context) (HealthStatus, error) {
	resp, err := c.stub.Health(ctx, &pb.HealthRequest{})
	if err != nil {
		return HealthStatus{}, err
	}
	return HealthStatus{
		Available: resp.Available, Model: resp.Model, Reason: resp.Reason,
		TopicsAvailable: resp.TopicsAvailable, TopicsModel: resp.TopicsModel, TopicsReason: resp.TopicsReason,
		ShadowAvailable: resp.ShadowAvailable, ShadowModel: resp.ShadowModel, ShadowReason: resp.ShadowReason,
	}, nil
}

func (c *Client) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

// ── Guard: cache + breaker ───────────────────────────────────────────────

const (
	cacheTTL  = 10 * time.Minute
	cacheSize = 20000

	breakerThreshold = 5
	breakerCooldown  = 10 * time.Second
)

// breaker stops calls to a service that failed breakerThreshold times in a
// row, for breakerCooldown. After the cooldown one call is let through; a
// success closes it again.
type breaker struct {
	failures  atomic.Int32
	openUntil atomic.Int64 // unix nanos; 0 when closed
}

func (b *breaker) open(now time.Time) bool {
	until := b.openUntil.Load()
	return until != 0 && now.UnixNano() < until
}

func (b *breaker) fail(now time.Time) {
	if b.failures.Add(1) >= breakerThreshold {
		b.openUntil.Store(now.Add(breakerCooldown).UnixNano())
	}
}

func (b *breaker) succeed() {
	b.failures.Store(0)
	b.openUntil.Store(0)
}

type cacheEntry struct {
	res  Result
	seen time.Time
}

// Guard puts a cache and a circuit breaker in front of a Scorer.
//
// A conversation resends every earlier message with each turn. The cache
// means only the new text is scored, which is what keeps the classifier
// inside its deadline on a long conversation.
type Guard struct {
	scorer Scorer
	now    func() time.Time

	mu    sync.Mutex
	cache map[[32]byte]cacheEntry

	breaker breaker

	calls, cached, errorsN, shortCircuited atomic.Int64
}

// NewGuard wraps scorer. A nil scorer returns nil.
func NewGuard(scorer Scorer) *Guard {
	if scorer == nil {
		return nil
	}
	return &Guard{scorer: scorer, now: time.Now, cache: make(map[[32]byte]cacheEntry)}
}

// Stats is a snapshot of the guard's counters.
type Stats struct {
	Calls          int64 // calls made to the service
	CachedTexts    int64 // texts answered from the cache
	Errors         int64 // calls that failed or timed out
	ShortCircuited int64 // requests refused while the breaker was open
}

func (g *Guard) Stats() Stats {
	if g == nil {
		return Stats{}
	}
	return Stats{Calls: g.calls.Load(), CachedTexts: g.cached.Load(), Errors: g.errorsN.Load(), ShortCircuited: g.shortCircuited.Load()}
}

// Score returns a result per text, within timeout. Texts seen recently are
// answered from the cache; the rest go to the service in one call.
func (g *Guard) Score(ctx context.Context, requestID string, texts []string, timeout time.Duration) ([]Result, error) {
	out := make([]Result, len(texts))
	keys := make([][32]byte, len(texts))
	var missIdx []int
	var missTexts []string
	now := g.now()

	g.mu.Lock()
	for i, t := range texts {
		keys[i] = sha256.Sum256([]byte(t))
		if e, ok := g.cache[keys[i]]; ok && now.Sub(e.seen) < cacheTTL {
			out[i] = e.res
			out[i].Cached = true
			continue
		}
		missIdx = append(missIdx, i)
		missTexts = append(missTexts, t)
	}
	g.mu.Unlock()
	g.cached.Add(int64(len(texts) - len(missIdx)))
	if len(missIdx) == 0 {
		return out, nil
	}

	if g.breaker.open(now) {
		g.shortCircuited.Add(1)
		return nil, ErrOpen
	}

	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	g.calls.Add(1)
	res, err := g.scorer.Score(callCtx, requestID, missTexts)
	if err != nil {
		g.errorsN.Add(1)
		g.breaker.fail(g.now())
		return nil, err
	}
	g.breaker.succeed()

	g.mu.Lock()
	if len(g.cache)+len(missIdx) > cacheSize {
		g.cache = make(map[[32]byte]cacheEntry)
	}
	for j, i := range missIdx {
		out[i] = res[j]
		// A truncated result is not a verdict on the whole text and is not
		// worth remembering as one.
		if !res[j].Truncated {
			g.cache[keys[i]] = cacheEntry{res: res[j], seen: now}
		}
	}
	g.mu.Unlock()
	return out, nil
}
