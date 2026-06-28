package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/scanner"
)

// attachStreamingOutputScanner wraps a streaming response body (SSE / NDJSON) with a
// sliding-buffer scanner. On output policy BLOCK, the stream is fail-closed with a
// terminal error chunk (FAZ1 Prompt 2).
func attachStreamingOutputScanner(
	resp *http.Response,
	pol *policy.Policy,
	reg *scanner.Registry,
	cfg HandlerConfig,
	provider, requestID string,
	ctx context.Context,
) {
	if resp == nil || resp.Body == nil || pol == nil || pol.OutputRules == nil || pol.OutputRules.Streaming == nil {
		return
	}
	st := pol.OutputRules.Streaming
	if !st.Enabled {
		return
	}
	maxBuf := st.MaxBufferBytes
	if maxBuf <= 0 {
		maxBuf = 8192
	}
	winMs := 200
	if pol.OutputRules.ScanWindowMs > 0 {
		winMs = pol.OutputRules.ScanWindowMs
	}

	orig := resp.Body
	pr, pw := io.Pipe()
	resp.Body = pr
	resp.ContentLength = -1
	resp.Header.Del("Content-Length")

	ct := resp.Header.Get("Content-Type")

	go func() {
		defer func() { _ = orig.Close() }()
		defer func() { _ = pw.Close() }()

		chunk := make([]byte, 4096)
		var scanBuf []byte
		var scanLatencySum time.Duration
		for {
			n, rerr := orig.Read(chunk)
			if n > 0 {
				scanBuf = append(scanBuf, chunk[:n]...)
				if len(scanBuf) > maxBuf {
					scanBuf = scanBuf[len(scanBuf)-maxBuf:]
				}

				scanCtx, cancel := context.WithTimeout(ctx, time.Duration(winMs)*time.Millisecond)
				t0 := time.Now()

				// Guard against nil Config — HandlerConfig may be constructed
				// without a Config field in certain proxy handler paths.
				var scanMode scanner.PipelineMode
				var scanTimeout time.Duration
				var scanLoadShed bool
				if cfg.Config != nil {
					scanMode = scanner.PipelineMode(cfg.Config.ScannerPipelineMode)
					scanTimeout = time.Duration(cfg.Config.ScannerPipelineTimeoutMs) * time.Millisecond
					scanLoadShed = cfg.Config.ScannerLoadShed
				} else {
					scanTimeout = 200 * time.Millisecond
				}

				pipeCfg := scanner.PipelineConfig{
					Mode:     scanMode,
					Timeout:  scanTimeout,
					Pool:     cfg.ScannerPool,
					LoadShed: scanLoadShed,
				}
				findings, scanErr := reg.ScanAllWithConfig(scanCtx, scanBuf, pipeCfg)

				// Run output-only scanners (e.g. code_leak) if configured.
				if cfg.OutputOnlyRegistry != nil && scanErr == nil {
					extraFindings, extraErr := cfg.OutputOnlyRegistry.ScanAllWithConfig(scanCtx, scanBuf, pipeCfg)
					if extraErr == nil {
						findings = append(findings, extraFindings...)
					}
				}

				cancel()
				scanLatencySum += time.Since(t0)

				if scanErr != nil {
					if pol.OutputRules != nil && pol.OutputRules.FailOpen {
						if _, werr := pw.Write(chunk[:n]); werr != nil {
							return
						}
					} else {
						// Fail-close: terminate the stream with an error event instead
						// of forwarding the unscanned chunk. This prevents PII/secrets
						// from leaking through when the scanner is unavailable.
						_, _ = pw.Write(buildStreamTermination(ct, "scanner_unavailable"))
						go publishOutputEvent(ctx, cfg, requestID, provider, nil, policy.ActionBlock, scanLatencySum)
						return
					}
				} else {
					act := pol.EvaluateOutput(findings)
					// REDACT cannot splice SSE/NDJSON safely mid-stream; fail-close like BLOCK.
					if act == policy.ActionBlock || act == policy.ActionRedact {
						_, _ = pw.Write(buildStreamTermination(ct, "content_blocked_by_policy"))
						go publishOutputEvent(ctx, cfg, requestID, provider, findings, act, scanLatencySum)
						return
					}
					if _, werr := pw.Write(chunk[:n]); werr != nil {
						return
					}
				}
			}
			if rerr == io.EOF {
				return
			}
			if rerr != nil {
				_ = pw.CloseWithError(rerr)
				return
			}
		}
	}()
}

func buildStreamTermination(ct string, reason string) []byte {
	if containsCI(ct, "text/event-stream") {
		return buildSSEErrorEvent(reason)
	}
	obj := map[string]interface{}{
		"type": "error",
		"error": map[string]string{
			"type":    "content_policy_violation",
			"message": "Response blocked by Tamga security policy",
			"reason":  reason,
		},
	}
	b, _ := json.Marshal(obj)
	return append(append(b, '\n'), '\n')
}

func buildSSEErrorEvent(reason string) []byte {
	data := map[string]interface{}{
		"type": "error",
		"error": map[string]string{
			"type":    "content_policy_violation",
			"message": "Response blocked by Tamga security policy",
			"reason":  reason,
		},
	}
	body, _ := json.Marshal(data)
	return []byte(fmt.Sprintf("event: error\ndata: %s\n\n", body))
}
