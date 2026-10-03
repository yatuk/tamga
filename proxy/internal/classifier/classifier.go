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
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"

	pb "github.com/yatuk/tamga/proto/classifier/v1"
)

// Result is the classifier's answer for one text.
type Result struct {
	// Score is the probability that the text is an injection attempt.
	Score float64
	// Truncated means the text was too long to score whole; a low Score is
	// then not a verdict.
	Truncated bool
}

// Scorer scores texts. The gRPC client is one; tests use their own.
type Scorer interface {
	Score(ctx context.Context, requestID string, texts []string) ([]Result, error)
}

// ErrOpen is returned while the breaker is open: the service failed
// repeatedly and is not being asked again yet.
var ErrOpen = errors.New("classifier: unavailable (circuit open)")

// ── gRPC client ──────────────────────────────────────────────────────────

// Client calls the classifier service.
type Client struct {
	conn *grpc.ClientConn
	stub pb.ClassifierServiceClient
}

// NewClient connects to addr (e.g. "classifier:50052"). An empty addr
// returns nil: no classifier is configured.
func NewClient(addr string) (*Client, error) {
	if addr == "" {
		return nil, nil
	}
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                30 * time.Second,
			Timeout:             5 * time.Second,
			PermitWithoutStream: true,
		}),
	)
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
	}
	return out, nil
}

// Health reports whether the service has a model loaded.
func (c *Client) Health(ctx context.Context) (available bool, model, reason string, err error) {
	resp, err := c.stub.Health(ctx, &pb.HealthRequest{})
	if err != nil {
		return false, "", "", err
	}
	return resp.Available, resp.Model, resp.Reason, nil
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

	failures  atomic.Int32
	openUntil atomic.Int64 // unix nanos; 0 when closed

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

	if until := g.openUntil.Load(); until != 0 && now.UnixNano() < until {
		g.shortCircuited.Add(1)
		return nil, ErrOpen
	}

	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	g.calls.Add(1)
	res, err := g.scorer.Score(callCtx, requestID, missTexts)
	if err != nil {
		g.errorsN.Add(1)
		if g.failures.Add(1) >= breakerThreshold {
			g.openUntil.Store(g.now().Add(breakerCooldown).UnixNano())
		}
		return nil, err
	}
	g.failures.Store(0)
	g.openUntil.Store(0)

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
