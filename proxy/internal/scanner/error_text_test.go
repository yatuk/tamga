package scanner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// lockedBuffer is a log sink the pipeline's goroutines can write to.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// captureScanLog sends the process log to a buffer for the length of a test.
func captureScanLog(t *testing.T) *lockedBuffer {
	t.Helper()
	logs := &lockedBuffer{}
	old := log.Logger
	log.Logger = zerolog.New(logs).Level(zerolog.DebugLevel)
	t.Cleanup(func() { log.Logger = old })
	return logs
}

// echoError is a scanner's error that quotes what the scanner was given.
type echoError struct{ text string }

func (e *echoError) Error() string { return "cannot parse input: " + e.text }

// quotingScanner fails with its input in the error, or panics with it.
type quotingScanner struct {
	name   string
	panics bool
}

func (s quotingScanner) Name() string { return s.name }
func (s quotingScanner) Scan(_ context.Context, content []byte) ([]Finding, error) {
	if s.panics {
		panic("unexpected input: " + string(content))
	}
	return nil, fmt.Errorf("quoting scanner: %w", &echoError{text: string(content)})
}

// A scanner whose error (or panic) quotes its input must put none of that
// input in the log, nor in the message of the error the pipeline returns,
// which callers log and answer with.
func TestPipeline_ScannerErrorTextIsNotLogged(t *testing.T) {
	const secret = "my TC is 38461027540 and the card is 4111-1111-1111-1111"
	pool := NewWorkerPool(2, 8)
	defer func() { _ = pool.Shutdown(time.Second) }()

	for _, mode := range []PipelineMode{ModeAdaptive, ModeAsync, ModeWorkerPool, ModeSync} {
		for _, panics := range []bool{false, true} {
			if panics && (mode == ModeSync || mode == ModeWorkerPool) {
				// Sync mode lets a panic through by design, and the pool's
				// workers are not the pipeline's to recover.
				continue
			}
			for _, speed := range []ScannerSpeed{SpeedFast, SpeedSlow} {
				name := fmt.Sprintf("%s/panics=%v/speed=%d", mode, panics, speed)
				t.Run(name, func(t *testing.T) {
					logs := captureScanLog(t)
					p := NewPipelineWithConfig([]ScannerEntry{
						{Scanner: okScanner("healthy"), Speed: SpeedFast},
						{Scanner: quotingScanner{name: "quoting", panics: panics}, Speed: speed},
					}, PipelineConfig{Mode: mode, Pool: pool})

					_, err := p.Scan(context.Background(), []byte(secret))
					if !errors.Is(err, ErrScanDegraded) {
						t.Fatalf("err = %v, want ErrScanDegraded", err)
					}
					if strings.Contains(err.Error(), "38461027540") || strings.Contains(err.Error(), "4111") {
						t.Errorf("the returned error's message holds the input: %q", err.Error())
					}
					if !strings.Contains(err.Error(), "quoting") {
						t.Errorf("the returned error does not name the scanner: %q", err.Error())
					}

					out := logs.String()
					for _, part := range []string{"38461027540", "4111", "my TC is", "cannot parse input", "unexpected input"} {
						if strings.Contains(out, part) {
							t.Errorf("the log holds %q:\n%s", part, out)
						}
					}

					wantKind := `"error_kind":"*scanner.echoError"`
					if panics {
						wantKind = `"error_kind":"panic"`
					}
					if mode == ModeSync {
						// Sync mode logs nothing; the kind is in the error.
						if !strings.Contains(err.Error(), "*scanner.echoError") {
							t.Errorf("err = %q, want the kind in it", err.Error())
						}
						return
					}
					if !strings.Contains(out, wantKind) || !strings.Contains(out, `"scanner":"quoting"`) {
						t.Errorf("the log does not name the scanner and the kind (%s):\n%s", wantKind, out)
					}
					// The cause is still in the chain for a caller that asks.
					var echo *echoError
					if !panics && !errors.As(err, &echo) {
						t.Errorf("the cause is no longer in the chain: %v", err)
					}
				})
			}
		}
	}
}

// A scan cut short by its deadline is still told apart from a scanner that
// failed, now that the cause is out of the message.
func TestPipeline_IncompleteReasonSurvivesTheQuietError(t *testing.T) {
	captureScanLog(t)
	stopped := degStub{name: "slow", fn: func() ([]Finding, error) {
		return nil, fmt.Errorf("%w: gave up: %w", ErrScanDegraded, context.DeadlineExceeded)
	}}
	p := NewPipelineWithConfig([]ScannerEntry{{Scanner: stopped, Speed: SpeedFast}}, PipelineConfig{Mode: ModeAdaptive})
	_, err := p.Scan(context.Background(), []byte("x"))
	if got := IncompleteReason(err); got != IncompleteDeadline {
		t.Fatalf("IncompleteReason = %q, want %q (err %v)", got, IncompleteDeadline, err)
	}
	if !strings.HasSuffix(err.Error(), "slow (error): timeout") {
		t.Fatalf("err = %q", err.Error())
	}
}
