package proxy

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/yatuk/tamga/internal/extract"
	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/scanner"
)

// maxOutputScanBytes is the default cap on non-stream response bodies that
// will be buffered and scanned. It sits above what current models can emit in
// one completion, so in practice only binary payloads (e.g. base64 images)
// exceed it; those are forwarded intact and unscanned.
const maxOutputScanBytes = 1024 * 1024

// vaultMaxRestoreBytes caps how much of a response the vault will buffer to
// restore placeholders. Chat responses are far smaller; a response larger than
// this is forwarded intact without restore (see wrapResponseForOutputScan).
const vaultMaxRestoreBytes = 8 * 1024 * 1024

// canaryMaxScanBytes caps how much of a response the canary detector will
// buffer. Responses larger than this are forwarded intact without a leak scan.
const canaryMaxScanBytes = 4 * 1024 * 1024

// outputScanResult is what scanResponseBody hands back to ModifyResponse.
type outputScanResult struct {
	findings []scanner.Finding
	action   policy.Action
	text     string
	elapsed  time.Duration
	// redacted is the response with the redact_on findings masked, set when
	// action is REDACT and the masking was carried out and verified.
	redacted      []byte
	redactedCount int
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
	scanCtx := ctx
	if windowMs > 0 {
		var cancel context.CancelFunc
		scanCtx, cancel = context.WithTimeout(ctx, time.Duration(windowMs)*time.Millisecond)
		defer cancel()
	}

	// A response shape the extractor knows is scanned by segment, which is
	// what makes redaction possible: findings then have a place in the body.
	if res, ok := extract.ExtractResponse(provider.Name(), raw); ok && len(res.Segments) > 0 {
		findings, err := reg.ScanSegments(scanCtx, res.Segments, pipeCfg)
		if outputReg != nil {
			extra, extraErr := outputReg.ScanSegments(scanCtx, res.Segments, pipeCfg)
			findings = append(findings, extra...)
			if err == nil {
				err = extraErr
			}
		}
		out := outputScanResult{findings: findings, action: pol.EvaluateOutput(findings)}
		if out.action == policy.ActionRedact {
			out.redacted, out.redactedCount = redactResponse(provider.Name(), raw, res.Segments, findings, pol)
			if out.redacted == nil {
				// A value that cannot be located, or a rewrite that did not
				// read back as intended: the response is not sent as it is.
				scanner.RecordDegraded("redact_output_failed")
				out.action = policy.ActionBlock
			}
		}
		out.elapsed = time.Since(start)
		return out, err
	}

	text := provider.ExtractOutputText(raw)
	if text == "" {
		return outputScanResult{action: policy.ActionPass, elapsed: time.Since(start)}, nil
	}

	// A scanner that fails or times out degrades the scan but does not void
	// it: whatever the other scanners found is still evaluated, and the error
	// is handed back so the caller can flag the response.
	findings, err := reg.ScanAllWithConfig(scanCtx, []byte(text), pipeCfg)

	// Run output-only scanners (e.g. code_leak) if configured.
	if outputReg != nil {
		extraFindings, extraErr := outputReg.ScanAllWithConfig(scanCtx, []byte(text), pipeCfg)
		findings = append(findings, extraFindings...)
		if err == nil {
			err = extraErr
		}
	}

	act := pol.EvaluateOutput(findings)
	if act == policy.ActionRedact {
		// The shape is unknown, so there is no place to redact at. Passing
		// the response on unredacted is what REDACT exists to prevent.
		scanner.RecordDegraded("redact_output_failed")
		act = policy.ActionBlock
	}
	return outputScanResult{
		findings: findings,
		action:   act,
		text:     text,
		elapsed:  time.Since(start),
	}, err
}

// redactResponse masks the findings output_rules.redact_on names. It returns
// nil when any of them has no position, or when the rewritten response
// cannot be confirmed.
func redactResponse(provider string, raw []byte, segs []extract.Segment, findings []scanner.Finding, pol *policy.Policy) ([]byte, int) {
	var edits []extract.Edit
	for _, f := range findings {
		if !pol.OutputRedacts(f) {
			continue
		}
		if !f.Placed() || f.Seg < 0 || f.Seg >= len(segs) {
			return nil, 0
		}
		edits = append(edits, extract.Edit{Seg: f.Seg, From: f.StartPos, To: f.EndPos, Replacement: "[" + f.Category + "_REDACTED]"})
	}
	if len(edits) == 0 {
		return nil, 0
	}
	out, err := extract.RewriteResponseChecked(provider, raw, segs, edits)
	if err != nil {
		return nil, 0
	}
	return out, len(edits)
}

// wrapResponseForOutputScan replaces the upstream response body with a buffered
// copy that can be scanned in ModifyResponse. For streaming responses
// (text/event-stream or application/x-ndjson) we fall through and let the
// streaming transport flush as normal; a future revision can implement a
// tee-reader with sliding-window scanning.
// When forceBuffer is true (vault restore needs the full body), buffering
// happens regardless of OutputRules/stream content type, and the body is never
// truncated — if it exceeds vaultMaxRestoreBytes the original stream is
// reconstructed (buffered prefix + remaining) and (false) is returned so the
// caller forwards it intact without restore.
func wrapResponseForOutputScan(resp *http.Response, pol *policy.Policy, forceBuffer bool) ([]byte, bool, error) {
	if resp == nil || resp.Body == nil {
		return nil, false, nil
	}
	ct := resp.Header.Get("Content-Type")

	if forceBuffer {
		body, err := io.ReadAll(io.LimitReader(resp.Body, int64(vaultMaxRestoreBytes)+1))
		if err != nil {
			return nil, false, err
		}
		if len(body) > vaultMaxRestoreBytes {
			// Too large to restore safely — preserve the full body and skip.
			resp.Body = struct {
				io.Reader
				io.Closer
			}{io.MultiReader(bytes.NewReader(body), resp.Body), resp.Body}
			return nil, false, nil
		}
		_ = resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(body))
		return body, true, nil
	}

	if pol == nil || pol.OutputRules == nil || !pol.OutputRules.Enabled {
		return nil, false, nil
	}
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
	if err != nil {
		_ = resp.Body.Close()
		return nil, false, err
	}
	if len(body) > limit {
		// Larger than the scan buffer. The client must still receive the
		// whole response, so stitch the bytes already read back in front of
		// the rest of the stream and forward it unscanned — visibly, not
		// silently: a truncated JSON body cannot be parsed for scanning and
		// would reach the caller corrupted.
		resp.Body = struct {
			io.Reader
			io.Closer
		}{io.MultiReader(bytes.NewReader(body), resp.Body), resp.Body}
		if resp.Header == nil {
			resp.Header = http.Header{}
		}
		resp.Header.Set("X-Tamga-Output-Scan", "skipped-too-large")
		scanner.RecordDegraded(scanner.DegradedOutputTooLarge)
		log.Warn().
			Str("component", "proxy").
			Int("buffer_bytes", limit).
			Msg("response exceeds output_rules.buffer_bytes; forwarded without output scan")
		return nil, false, nil
	}
	_ = resp.Body.Close()
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
