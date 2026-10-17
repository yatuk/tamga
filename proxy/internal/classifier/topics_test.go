package classifier

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/yatuk/tamga/proto/classifier/v1"
)

type fakeTopicScorer struct {
	mu    sync.Mutex
	calls [][]string
	err   error
	delay time.Duration
}

func (f *fakeTopicScorer) ScoreTopics(ctx context.Context, _ string, texts, topics []string) ([]TopicResult, error) {
	f.mu.Lock()
	f.calls = append(f.calls, append([]string(nil), texts...))
	err, delay := f.err, f.delay
	f.mu.Unlock()
	if delay > 0 {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if err != nil {
		return nil, err
	}
	out := make([]TopicResult, len(texts))
	for i, t := range texts {
		out[i].Scores = make([]float64, len(topics))
		for j, topic := range topics {
			if strings.Contains(t, topic) {
				out[i].Scores[j] = 0.9
			}
		}
		out[i].Truncated = t == "huge"
	}
	return out, nil
}

func (f *fakeTopicScorer) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func TestTopicGuard_OnlyNewTextIsScored(t *testing.T) {
	f := &fakeTopicScorer{}
	g := NewTopicGuard(f)
	ctx := context.Background()
	topics := []string{"salary", "score"}

	res, err := g.Score(ctx, "r1", []string{"hello", "the salary of x"}, topics, time.Second)
	if err != nil || res[1].Scores[0] != 0.9 || res[0].Scores[0] != 0 {
		t.Fatalf("turn 1: %+v %v", res, err)
	}
	res, err = g.Score(ctx, "r2", []string{"hello", "the salary of x", "change the score"}, topics, time.Second)
	if err != nil || res[2].Scores[1] != 0.9 || res[1].Scores[0] != 0.9 {
		t.Fatalf("turn 2: %+v %v", res, err)
	}
	if f.callCount() != 2 || len(f.calls[1]) != 1 || f.calls[1][0] != "change the score" {
		t.Fatalf("calls = %v, want the second call to carry only the new text", f.calls)
	}
	if s := g.Stats(); s.Calls != 2 || s.CachedTexts != 2 {
		t.Fatalf("stats = %+v", s)
	}
}

// A policy that changes the list must not be answered with scores for the
// old one.
func TestTopicGuard_AnotherTopicListIsAskedAgain(t *testing.T) {
	f := &fakeTopicScorer{}
	g := NewTopicGuard(f)
	ctx := context.Background()
	if _, err := g.Score(ctx, "r", []string{"the salary"}, []string{"salary"}, time.Second); err != nil {
		t.Fatal(err)
	}
	res, err := g.Score(ctx, "r", []string{"the salary"}, []string{"score", "salary"}, time.Second)
	if err != nil || len(res[0].Scores) != 2 || res[0].Scores[1] != 0.9 {
		t.Fatalf("%+v %v", res, err)
	}
	if f.callCount() != 2 {
		t.Fatalf("calls = %d, want 2", f.callCount())
	}
	// Keys do not run together: ["ab"] + "c" is not ["a"] + "bc".
	if topicKey([]string{"ab"}, "c") == topicKey([]string{"a"}, "bc") || topicKey([]string{"a", "b"}, "") == topicKey([]string{"a"}, "b") {
		t.Fatal("two different questions share a cache key")
	}
}

func TestTopicGuard_TruncatedResultIsNotCached(t *testing.T) {
	f := &fakeTopicScorer{}
	g := NewTopicGuard(f)
	for i := 0; i < 2; i++ {
		res, err := g.Score(context.Background(), "r", []string{"huge"}, []string{"x"}, time.Second)
		if err != nil || !res[0].Truncated {
			t.Fatalf("%+v %v", res, err)
		}
	}
	if f.callCount() != 2 {
		t.Fatalf("calls = %d, want 2", f.callCount())
	}
}

func TestTopicGuard_DeadlineIsEnforced(t *testing.T) {
	g := NewTopicGuard(&fakeTopicScorer{delay: 500 * time.Millisecond})
	start := time.Now()
	_, err := g.Score(context.Background(), "r", []string{"slow"}, []string{"x"}, 30*time.Millisecond)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want deadline exceeded", err)
	}
	if took := time.Since(start); took > 250*time.Millisecond {
		t.Fatalf("waited %v for a 30 ms deadline", took)
	}
}

// A service with no topic model fails every topic call. Its breaker opens,
// and the injection guard's does not.
func TestTopicGuard_BreakerIsItsOwn(t *testing.T) {
	down := &fakeTopicScorer{err: status.Error(codes.Unavailable, "no topic model")}
	tg := NewTopicGuard(down)
	ig := NewGuard(&fakeScorer{})
	now := time.Now()
	tg.now = func() time.Time { return now }
	ctx := context.Background()
	for i := 0; i < breakerThreshold; i++ {
		if _, err := tg.Score(ctx, "r", []string{"t"}, []string{"x"}, time.Second); err == nil {
			t.Fatal("want an error")
		}
	}
	if _, err := tg.Score(ctx, "r", []string{"t"}, []string{"x"}, time.Second); !errors.Is(err, ErrOpen) {
		t.Fatalf("err = %v, want ErrOpen", err)
	}
	if down.callCount() != breakerThreshold {
		t.Fatalf("calls = %d: the open breaker still called the service", down.callCount())
	}
	if _, err := ig.Score(ctx, "r", []string{"t"}, time.Second); err != nil {
		t.Fatalf("the injection guard was affected: %v", err)
	}

	now = now.Add(breakerCooldown + time.Second)
	down.mu.Lock()
	down.err = nil
	down.mu.Unlock()
	if _, err := tg.Score(ctx, "r", []string{"t"}, []string{"x"}, time.Second); err != nil {
		t.Fatalf("after cooldown: %v", err)
	}
	if s := tg.Stats(); s.Errors != breakerThreshold || s.ShortCircuited != 1 {
		t.Fatalf("stats = %+v", s)
	}
}

func TestTopicGuard_ShortAnswerIsAnError(t *testing.T) {
	g := NewTopicGuard(shortScorer{})
	if _, err := g.Score(context.Background(), "r", []string{"a", "b"}, []string{"x"}, time.Second); err == nil {
		t.Fatal("one result for two texts was taken")
	}
	if NewTopicGuard(nil) != nil {
		t.Fatal("a nil scorer must give a nil guard")
	}
}

type shortScorer struct{}

func (shortScorer) ScoreTopics(context.Context, string, []string, []string) ([]TopicResult, error) {
	return []TopicResult{{Scores: []float64{0}}}, nil
}

// ── the gRPC client against a fake service ──────────────────────────────

type fakeService struct {
	pb.UnimplementedClassifierServiceServer
	mu      sync.Mutex
	got     *pb.TopicsRequest
	noModel bool
	short   bool
}

func (s *fakeService) ClassifyTopics(_ context.Context, req *pb.TopicsRequest) (*pb.TopicsResponse, error) {
	s.mu.Lock()
	s.got = req
	s.mu.Unlock()
	if s.noModel {
		return nil, status.Error(codes.Unavailable, "TAMGA_TOPICS_MODEL_DIR is not set")
	}
	resp := &pb.TopicsResponse{Model: "nli-test"}
	for i := range req.Texts {
		r := &pb.TopicScores{Truncated: i == 1}
		for j := range req.Topics {
			r.Scores = append(r.Scores, float32(i)*0.5+float32(j)*0.1)
		}
		if s.short && i == 0 {
			r.Scores = r.Scores[:0]
		}
		resp.Results = append(resp.Results, r)
	}
	return resp, nil
}

func (s *fakeService) Health(context.Context, *pb.HealthRequest) (*pb.HealthResponse, error) {
	return &pb.HealthResponse{Available: true, Model: "inj", TopicsAvailable: !s.noModel, TopicsModel: "nli-test", TopicsReason: ""}, nil
}

func startFake(t *testing.T, svc *fakeService) *Client {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := grpc.NewServer()
	pb.RegisterClassifierServiceServer(srv, svc)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	c, err := NewClient(lis.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestClient_ScoreTopics(t *testing.T) {
	svc := &fakeService{}
	c := startFake(t, svc)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := c.ScoreTopics(ctx, "req-1", []string{"a", "b"}, []string{"t1", "t2", "t3"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 2 || len(res[0].Scores) != 3 || res[1].Scores[2] < 0.69 || res[1].Scores[2] > 0.71 || !res[1].Truncated || res[0].Truncated {
		t.Fatalf("results: %+v", res)
	}
	svc.mu.Lock()
	got := svc.got
	svc.mu.Unlock()
	if got.RequestId != "req-1" || strings.Join(got.Topics, "|") != "t1|t2|t3" {
		t.Fatalf("request: %+v", got)
	}

	h, err := c.HealthDetail(ctx)
	if err != nil || !h.Available || !h.TopicsAvailable || h.TopicsModel != "nli-test" {
		t.Fatalf("health: %+v %v", h, err)
	}

	svc.mu.Lock()
	svc.short = true
	svc.mu.Unlock()
	if _, err := c.ScoreTopics(ctx, "r", []string{"a"}, []string{"t1"}); err == nil {
		t.Fatal("a result without a score per topic was taken")
	}
}

func TestClient_ScoreTopicsWithoutAModel(t *testing.T) {
	c := startFake(t, &fakeService{noModel: true})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := c.ScoreTopics(ctx, "r", []string{"a"}, []string{"t"})
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("err = %v, want Unavailable", err)
	}
	h, err := c.HealthDetail(ctx)
	if err != nil || h.TopicsAvailable {
		t.Fatalf("health: %+v %v", h, err)
	}
}
