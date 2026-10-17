package classifier

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	pb "github.com/yatuk/tamga/proto/classifier/v1"
)

// TopicResult is the topic model's answer for one text.
type TopicResult struct {
	// Scores holds, per topic in the order asked, the probability that
	// the text is about it.
	Scores []float64
	// Truncated means the text was too long to read whole; a low score is
	// then not a verdict.
	Truncated bool
}

// TopicScorer scores texts against topics. The gRPC client is one; tests
// use their own.
type TopicScorer interface {
	ScoreTopics(ctx context.Context, requestID string, texts, topics []string) ([]TopicResult, error)
}

// ScoreTopics asks the service's topic model about texts.
func (c *Client) ScoreTopics(ctx context.Context, requestID string, texts, topics []string) ([]TopicResult, error) {
	resp, err := c.stub.ClassifyTopics(ctx, &pb.TopicsRequest{RequestId: requestID, Texts: texts, Topics: topics})
	if err != nil {
		return nil, err
	}
	if len(resp.Results) != len(texts) {
		return nil, fmt.Errorf("classifier: %d topic results for %d texts", len(resp.Results), len(texts))
	}
	out := make([]TopicResult, len(texts))
	for i, r := range resp.Results {
		if len(r.Scores) != len(topics) {
			return nil, fmt.Errorf("classifier: %d topic scores for %d topics", len(r.Scores), len(topics))
		}
		out[i].Scores = make([]float64, len(topics))
		for j, s := range r.Scores {
			out[i].Scores[j] = float64(s)
		}
		out[i].Truncated = r.Truncated
	}
	return out, nil
}

// TopicGuard puts a cache and a circuit breaker in front of a TopicScorer,
// as Guard does for injection scores. It has a breaker of its own: the
// topic model is optional, and a service without one answers every topic
// call with an error that must not stop the proxy asking about injections.
//
// A cached answer is for a text and the exact list of topics it was asked
// about, so a policy that changes the list asks again.
type TopicGuard struct {
	scorer TopicScorer
	now    func() time.Time

	mu    sync.Mutex
	cache map[[32]byte]topicEntry

	breaker breaker

	calls, cached, errorsN, shortCircuited atomic.Int64
}

type topicEntry struct {
	res  TopicResult
	seen time.Time
}

// NewTopicGuard wraps scorer. A nil scorer returns nil.
func NewTopicGuard(scorer TopicScorer) *TopicGuard {
	if scorer == nil {
		return nil
	}
	return &TopicGuard{scorer: scorer, now: time.Now, cache: make(map[[32]byte]topicEntry)}
}

// Stats is a snapshot of the guard's counters.
func (g *TopicGuard) Stats() Stats {
	if g == nil {
		return Stats{}
	}
	return Stats{Calls: g.calls.Load(), CachedTexts: g.cached.Load(), Errors: g.errorsN.Load(), ShortCircuited: g.shortCircuited.Load()}
}

// topicKey names a text asked about a list of topics. Lengths go in first
// so that no two lists, or a list and a text, run together into one key.
func topicKey(topics []string, text string) [32]byte {
	h := sha256.New()
	var n [8]byte
	binary.LittleEndian.PutUint64(n[:], uint64(len(topics)))
	h.Write(n[:])
	for _, s := range append(topics[:len(topics):len(topics)], text) {
		binary.LittleEndian.PutUint64(n[:], uint64(len(s)))
		h.Write(n[:])
		h.Write([]byte(s))
	}
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}

// Score returns a result per text, within timeout. Texts seen recently with
// the same topics are answered from the cache; the rest go to the service
// in one call.
func (g *TopicGuard) Score(ctx context.Context, requestID string, texts, topics []string, timeout time.Duration) ([]TopicResult, error) {
	out := make([]TopicResult, len(texts))
	keys := make([][32]byte, len(texts))
	var missIdx []int
	var missTexts []string
	now := g.now()

	g.mu.Lock()
	for i, t := range texts {
		keys[i] = topicKey(topics, t)
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

	if g.breaker.open(now) {
		g.shortCircuited.Add(1)
		return nil, ErrOpen
	}

	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	g.calls.Add(1)
	res, err := g.scorer.ScoreTopics(callCtx, requestID, missTexts, topics)
	if err == nil && len(res) != len(missTexts) {
		err = fmt.Errorf("classifier: %d topic results for %d texts", len(res), len(missTexts))
	}
	if err != nil {
		g.errorsN.Add(1)
		g.breaker.fail(g.now())
		return nil, err
	}
	g.breaker.succeed()

	g.mu.Lock()
	if len(g.cache)+len(missIdx) > cacheSize {
		g.cache = make(map[[32]byte]topicEntry)
	}
	for j, i := range missIdx {
		out[i] = res[j]
		if !res[j].Truncated {
			g.cache[keys[i]] = topicEntry{res: res[j], seen: now}
		}
	}
	g.mu.Unlock()
	return out, nil
}
