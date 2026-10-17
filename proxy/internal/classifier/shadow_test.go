package classifier

import (
	"context"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"

	pb "github.com/yatuk/tamga/proto/classifier/v1"
)

// shadowService answers Classify with a shadow model beside the primary.
type shadowService struct {
	pb.UnimplementedClassifierServiceServer
	shadow  string // shadow model name; "" runs none
	skipped string
	short   bool // fewer shadow scores than texts
}

func (s *shadowService) Classify(_ context.Context, req *pb.ClassifyRequest) (*pb.ClassifyResponse, error) {
	resp := &pb.ClassifyResponse{Model: "primary", ShadowModel: s.shadow, ShadowSkipped: s.skipped}
	for i := range req.Texts {
		resp.Scores = append(resp.Scores, 0.1)
		resp.Truncated = append(resp.Truncated, false)
		if s.shadow != "" && s.skipped == "" && !(s.short && i > 0) {
			resp.ShadowScores = append(resp.ShadowScores, 0.9)
			resp.ShadowTruncated = append(resp.ShadowTruncated, i == 1)
		}
	}
	return resp, nil
}

func (s *shadowService) Health(context.Context, *pb.HealthRequest) (*pb.HealthResponse, error) {
	if s.shadow == "" {
		return &pb.HealthResponse{Available: true, Model: "primary", ShadowReason: "TAMGA_CLASSIFIER_SHADOW_MODEL_DIR is not set"}, nil
	}
	return &pb.HealthResponse{Available: true, Model: "primary", ShadowAvailable: true, ShadowModel: s.shadow}, nil
}

func startShadow(t *testing.T, svc *shadowService) *Client {
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

func TestClient_ShadowScores(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := startShadow(t, &shadowService{shadow: "cand"}).Score(ctx, "r", []string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	for i, r := range res {
		if r.Score < 0.09 || r.Score > 0.11 || r.Model != "primary" || r.ShadowModel != "cand" || !r.HasShadow || r.ShadowScore < 0.89 || r.ShadowSkipped != "" {
			t.Fatalf("result %d: %+v", i, r)
		}
	}
	if res[0].ShadowTruncated || !res[1].ShadowTruncated {
		t.Fatalf("shadow truncation: %+v", res)
	}

	// No shadow: nothing about one.
	res, err = startShadow(t, &shadowService{}).Score(ctx, "r", []string{"a"})
	if err != nil || res[0].ShadowModel != "" || res[0].HasShadow {
		t.Fatalf("without a shadow: %+v %v", res, err)
	}

	// A late shadow: the primary's scores, and why the shadow's are missing.
	res, err = startShadow(t, &shadowService{shadow: "cand", skipped: "late"}).Score(ctx, "r", []string{"a"})
	if err != nil || res[0].Score < 0.09 || res[0].HasShadow || res[0].ShadowSkipped != "late" {
		t.Fatalf("late shadow: %+v %v", res, err)
	}

	// Shadow scores that do not fit the texts never fail the call.
	res, err = startShadow(t, &shadowService{shadow: "cand", short: true}).Score(ctx, "r", []string{"a", "b"})
	if err != nil || len(res) != 2 || res[0].HasShadow || res[1].HasShadow || res[0].ShadowSkipped != "failed" {
		t.Fatalf("short shadow: %+v %v", res, err)
	}
}

func TestClient_HealthReportsTheShadow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	h, err := startShadow(t, &shadowService{shadow: "cand"}).HealthDetail(ctx)
	if err != nil || !h.ShadowAvailable || h.ShadowModel != "cand" {
		t.Fatalf("health: %+v %v", h, err)
	}
	h, err = startShadow(t, &shadowService{}).HealthDetail(ctx)
	if err != nil || !h.Available || h.ShadowAvailable || h.ShadowReason == "" {
		t.Fatalf("health without a shadow: %+v %v", h, err)
	}
}

func TestGuard_CachedResultsKeepTheirShadowAndSayCached(t *testing.T) {
	g := NewGuard(scorerFunc(func(texts []string) []Result {
		out := make([]Result, len(texts))
		for i := range out {
			out[i] = Result{Score: 0.2, ShadowModel: "cand", HasShadow: true, ShadowScore: 0.7}
		}
		return out
	}))
	first, err := g.Score(context.Background(), "r1", []string{"hello"}, time.Second)
	if err != nil || first[0].Cached {
		t.Fatalf("first: %+v %v", first, err)
	}
	second, err := g.Score(context.Background(), "r2", []string{"hello"}, time.Second)
	if err != nil || !second[0].Cached || !second[0].HasShadow || second[0].ShadowScore != 0.7 {
		t.Fatalf("second: %+v %v", second, err)
	}
}

type scorerFunc func([]string) []Result

func (f scorerFunc) Score(_ context.Context, _ string, texts []string) ([]Result, error) {
	return f(texts), nil
}

func TestShadowOutcome(t *testing.T) {
	with := func(s float64) Result { return Result{HasShadow: true, ShadowScore: s} }
	cases := []struct {
		primary bool
		r       Result
		want    string
	}{
		{true, with(0.99), ShadowBoth},
		{false, with(0.10), ShadowNeither},
		{false, with(0.95), ShadowOnly},
		{true, with(0.94), ShadowPrimaryOnly},
		{true, Result{ShadowSkipped: "late"}, ShadowMissing},
	}
	for _, c := range cases {
		if got := ShadowOutcome(c.primary, c.r, 0.95); got != c.want {
			t.Errorf("ShadowOutcome(%v, %+v) = %s, want %s", c.primary, c.r, got, c.want)
		}
	}
}

func TestShadowStats(t *testing.T) {
	RecordShadow("stats-model-b", ShadowOnly)
	RecordShadow("stats-model-a", ShadowNeither)
	RecordShadow("stats-model-a", ShadowNeither)
	var mine []ShadowCount
	for _, c := range ShadowStats() {
		if c.Model == "stats-model-a" || c.Model == "stats-model-b" {
			mine = append(mine, c)
		}
	}
	if len(mine) != 2 || mine[0] != (ShadowCount{"stats-model-a", ShadowNeither, 2}) || mine[1] != (ShadowCount{"stats-model-b", ShadowOnly, 1}) {
		t.Fatalf("stats = %+v", mine)
	}
}
