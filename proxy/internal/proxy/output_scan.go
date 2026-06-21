package proxy

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"time"

	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/scanner"
)

// maxOutputScanBytes caps non-stream response bodies that will be scanned.
const maxOutputScanBytes = 256 * 1024

// outputScanResult is what scanResponseBody hands back to ModifyResponse.
type outputScanResult struct {
	findings []scanner.Finding
	action   policy.Action
	text     string
	elapsed  time.Duration
}

// scanResponseBody reads up to `limit` bytes from `body`, runs both the main
// scanner registry and the optional output-only registry on the extracted text,
// and returns the combined outcome. The caller is responsible for reconstructing
// the response stream.
//
// outputReg is optional (may be nil) and contains scanners that should only
// run on response bodies (e.g. code_leak).
func scanResponseBody(ctx context.Context, reg *scanner.Registry, outputReg *scanner.Registry, pol *policy.Policy, provider Provider, raw []byte, windowMs int, pipeCfg scanner.PipelineConfig) (outputScanResult, error) {
	start := time.Now()
	text := provider.ExtractOutputText(raw)
	if text == "" {
		return outputScanResult{action: policy.ActionPass, elapsed: time.Since(start)}, nil
	}

	scanCtx := ctx
	if windowMs > 0 {
		var cancel context.CancelFunc
		scanCtx, cancel = context.WithTimeout(ctx, time.Duration(windowMs)*time.Millisecond)
		defer cancel()
	}
	findings, err := reg.ScanAllWithConfig(scanCtx, []byte(text), pipeCfg)
	if err != nil {
		return outputScanResult{action: policy.ActionPass, elapsed: time.Since(start)}, err
	}

	// Run output-only scanners (e.g. code_leak) if configured.
	if outputReg != nil {
		extraFindings, extraErr := outputReg.ScanAllWithConfig(scanCtx, []byte(text), pipeCfg)
		if extraErr == nil {
			findings = append(findings, extraFindings...)
		}
	}

	act := pol.EvaluateOutput(findings)
	return outputScanResult{
		findings: findings,
		action:   act,
		text:     text,
		elapsed:  time.Since(start),
	}, nil
}

// wrapResponseForOutputScan replaces the upstream response body with a buffered
// copy that can be scanned in ModifyResponse. For streaming responses
// (text/event-stream or application/x-ndjson) we fall through and let the
// streaming transport flush as normal; a future revision can implement a
// tee-reader with sliding-window scanning.
func wrapResponseForOutputScan(resp *http.Response, pol *policy.Policy) ([]byte, bool, error) {
	if resp == nil || resp.Body == nil {
		return nil, false, nil
	}
	if pol == nil || pol.OutputRules == nil || !pol.OutputRules.Enabled {
		return nil, false, nil
	}
	ct := resp.Header.Get("Content-Type")
	if isStreamContentType(ct) {
		// Stream scanning is a best-effort "hint" for now; the body is still
		// forwarded unchanged via FlushInterval.
		return nil, false, nil
	}
	limit := pol.OutputRules.BufferBytes
	if limit <= 0 {
		limit = maxOutputScanBytes
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, int64(limit)+1))
	_ = resp.Body.Close()
	if err != nil {
		return nil, false, err
	}
	truncated := len(body) > limit
	if truncated {
		body = body[:limit]
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))
	return body, true, nil
}

func isStreamContentType(ct string) bool {
	if ct == "" {
		return false
	}
	switch {
	case containsCI(ct, "text/event-stream"):
		return true
	case containsCI(ct, "application/x-ndjson"):
		return true
	}
	return false
}

func containsCI(haystack, needle string) bool {
	if len(needle) == 0 {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		ok := true
		for j := 0; j < len(needle); j++ {
			a := haystack[i+j]
			b := needle[j]
			if a >= 'A' && a <= 'Z' {
				a += 32
			}
			if b >= 'A' && b <= 'Z' {
				b += 32
			}
			if a != b {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}
