package scanner

import (
	"context"
	"testing"
	"time"
)

// stubPlainScanner records that plain Scan was called.
type stubPlainScanner struct {
	called int
}

func (s *stubPlainScanner) Name() string { return "stub_plain" }
func (s *stubPlainScanner) Scan(ctx context.Context, content []byte) ([]Finding, error) {
	s.called++
	return nil, nil
}

// stubContextualScanner records which entry point ran and the reqCtx it saw.
type stubContextualScanner struct {
	scanCalled    int
	ctxCalled     int
	lastReqCtx    *RequestContext
	resultFinding []Finding
}

func (s *stubContextualScanner) Name() string { return "stub_contextual" }
func (s *stubContextualScanner) Scan(ctx context.Context, content []byte) ([]Finding, error) {
	s.scanCalled++
	return s.resultFinding, nil
}
func (s *stubContextualScanner) ScanWithContext(ctx context.Context, content []byte, reqCtx *RequestContext) ([]Finding, error) {
	s.ctxCalled++
	s.lastReqCtx = reqCtx
	return s.resultFinding, nil
}

func TestPipeline_ContextualScannerDispatch(t *testing.T) {
	reqCtx := &RequestContext{
		RequestId:         "req-1",
		OperatorId:        "op-1",
		ActiveDecisionIds: []string{"D-2026-06-23-001"},
	}

	modes := []PipelineMode{ModeSync, ModeAsync, ModeAdaptive}
	for _, mode := range modes {
		t.Run(string(mode), func(t *testing.T) {
			plain := &stubPlainScanner{}
			contextual := &stubContextualScanner{}
			entries := []ScannerEntry{
				{Scanner: plain, Speed: SpeedFast},
				{Scanner: contextual, Speed: SpeedSlow},
			}
			p := NewPipelineWithConfig(entries, PipelineConfig{Mode: mode, RequestCtx: reqCtx})
			if _, err := p.Scan(context.Background(), []byte("hello")); err != nil {
				t.Fatalf("Scan error: %v", err)
			}
			if plain.called != 1 {
				t.Errorf("plain scanner Scan called %d times, want 1", plain.called)
			}
			if contextual.ctxCalled != 1 || contextual.scanCalled != 0 {
				t.Errorf("contextual scanner: ScanWithContext=%d Scan=%d, want 1/0", contextual.ctxCalled, contextual.scanCalled)
			}
			if contextual.lastReqCtx == nil || contextual.lastReqCtx.OperatorId != "op-1" {
				t.Errorf("RequestContext not threaded: %+v", contextual.lastReqCtx)
			}
		})
	}
}

func TestPipeline_ContextualScannerWorkerPool(t *testing.T) {
	pool := NewWorkerPool(2, 8)
	defer func() { _ = pool.Shutdown(5 * time.Second) }()

	reqCtx := &RequestContext{RequestId: "req-wp", OperatorId: "op-wp"}
	plain := &stubPlainScanner{}
	contextual := &stubContextualScanner{}
	entries := []ScannerEntry{
		{Scanner: plain, Speed: SpeedFast},
		{Scanner: contextual, Speed: SpeedSlow},
	}
	p := NewPipelineWithConfig(entries, PipelineConfig{Mode: ModeWorkerPool, Pool: pool, RequestCtx: reqCtx})
	if _, err := p.Scan(context.Background(), []byte("hello")); err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if contextual.ctxCalled != 1 || contextual.scanCalled != 0 {
		t.Errorf("worker pool: ScanWithContext=%d Scan=%d, want 1/0", contextual.ctxCalled, contextual.scanCalled)
	}
	if contextual.lastReqCtx == nil || contextual.lastReqCtx.OperatorId != "op-wp" {
		t.Errorf("RequestContext not threaded through pool: %+v", contextual.lastReqCtx)
	}
}

func TestPipeline_NilRequestCtxUsesPlainScan(t *testing.T) {
	contextual := &stubContextualScanner{}
	entries := []ScannerEntry{{Scanner: contextual, Speed: SpeedFast}}
	p := NewPipelineWithConfig(entries, PipelineConfig{Mode: ModeSync})
	if _, err := p.Scan(context.Background(), []byte("hello")); err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if contextual.scanCalled != 1 || contextual.ctxCalled != 0 {
		t.Errorf("nil reqCtx: Scan=%d ScanWithContext=%d, want 1/0", contextual.scanCalled, contextual.ctxCalled)
	}
}
