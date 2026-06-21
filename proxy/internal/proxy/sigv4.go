package proxy

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// SigV4Credentials carries the short-lived key material needed to sign
// AWS Bedrock requests. SessionToken is optional (only populated when
// the host runs on an IAM role / AssumeRole / SSO profile).
type SigV4Credentials struct {
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
	Region          string
	Service         string
}

// bedrockCredentials derives SigV4 credentials from the environment.
// Returned zero-value + ok=false means no creds configured and the
// proxy should forward untouched (useful in smoke tests / local-only
// deployments).
func bedrockCredentials() (SigV4Credentials, bool) {
	ak := strings.TrimSpace(envLookup("AWS_ACCESS_KEY_ID"))
	sk := strings.TrimSpace(envLookup("AWS_SECRET_ACCESS_KEY"))
	if ak == "" || sk == "" {
		return SigV4Credentials{}, false
	}
	region := strings.TrimSpace(envLookup("AWS_REGION"))
	if region == "" {
		region = strings.TrimSpace(envLookup("AWS_DEFAULT_REGION"))
	}
	if region == "" {
		region = "us-east-1"
	}
	return SigV4Credentials{
		AccessKeyID:     ak,
		SecretAccessKey: sk,
		SessionToken:    strings.TrimSpace(envLookup("AWS_SESSION_TOKEN")),
		Region:          region,
		Service:         "bedrock",
	}, true
}

// signBedrockRequest attaches AWS SigV4 headers to req for use against
// bedrock-runtime. It mirrors the minimum fields Bedrock's InvokeModel
// / ConverseStream APIs require: Host, X-Amz-Date, X-Amz-Content-Sha256,
// optional X-Amz-Security-Token, and Authorization.
//
// The body must already be set on req (the caller's Director wires
// bytes.NewReader), since SigV4 hashes the payload and commits to a
// Content-Length up-front.
// BedrockRequestSigner returns a hook for upstream provider pools: signs requests when
// the target host is an AWS Bedrock runtime endpoint.
func BedrockRequestSigner() func(*http.Request, []byte, *url.URL) {
	return func(req *http.Request, body []byte, u *url.URL) {
		if req == nil || u == nil {
			return
		}
		if !strings.Contains(strings.ToLower(u.Host), "bedrock") {
			return
		}
		if creds, ok := bedrockCredentials(); ok {
			signBedrockRequest(req, body, creds, time.Now().UTC())
		}
	}
}

func signBedrockRequest(req *http.Request, body []byte, creds SigV4Credentials, now time.Time) {
	if req == nil || creds.AccessKeyID == "" || creds.SecretAccessKey == "" {
		return
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	now = now.UTC()
	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")

	sum := sha256.Sum256(body)
	payloadHash := hex.EncodeToString(sum[:])

	// Drop any inbound Authorization from the caller — we are signing
	// a fresh request against Bedrock, the client's key is not valid
	// here.
	req.Header.Del("Authorization")
	req.Header.Set("Host", req.Host)
	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)
	if creds.SessionToken != "" {
		req.Header.Set("X-Amz-Security-Token", creds.SessionToken)
	}

	canonicalURI := req.URL.EscapedPath()
	if canonicalURI == "" {
		canonicalURI = "/"
	}
	canonicalQuery := canonicalQueryString(req.URL.RawQuery)

	signedHeaderNames, canonicalHeaders := canonicalHeaderBlock(req)
	canonicalRequest := strings.Join([]string{
		req.Method,
		canonicalURI,
		canonicalQuery,
		canonicalHeaders,
		signedHeaderNames,
		payloadHash,
	}, "\n")

	credScope := fmt.Sprintf("%s/%s/%s/aws4_request", dateStamp, creds.Region, creds.Service)
	crSum := sha256.Sum256([]byte(canonicalRequest))
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		credScope,
		hex.EncodeToString(crSum[:]),
	}, "\n")

	signingKey := deriveSigningKey(creds.SecretAccessKey, dateStamp, creds.Region, creds.Service)
	signature := hex.EncodeToString(hmacSHA256(signingKey, []byte(stringToSign)))

	auth := fmt.Sprintf(
		"AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		creds.AccessKeyID, credScope, signedHeaderNames, signature,
	)
	req.Header.Set("Authorization", auth)
}

func canonicalHeaderBlock(req *http.Request) (string, string) {
	// We sign Host + any x-amz-* header so Bedrock's verifier accepts
	// the request. Keep the set minimal to reduce surprise across
	// proxies that re-order headers (Envoy, ALB).
	lower := map[string]string{}
	lower["host"] = req.Host
	for k, v := range req.Header {
		lk := strings.ToLower(k)
		if lk == "host" || strings.HasPrefix(lk, "x-amz-") || lk == "content-type" {
			lower[lk] = strings.Join(v, ",")
		}
	}
	names := make([]string, 0, len(lower))
	for k := range lower {
		names = append(names, k)
	}
	sort.Strings(names)
	var b strings.Builder
	for _, n := range names {
		b.WriteString(n)
		b.WriteByte(':')
		b.WriteString(strings.TrimSpace(collapseWS(lower[n])))
		b.WriteByte('\n')
	}
	return strings.Join(names, ";"), b.String()
}

func canonicalQueryString(raw string) string {
	if raw == "" {
		return ""
	}
	pairs := strings.Split(raw, "&")
	sort.Strings(pairs)
	return strings.Join(pairs, "&")
}

func collapseWS(s string) string {
	fields := strings.Fields(s)
	return strings.Join(fields, " ")
}

func hmacSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

func deriveSigningKey(secret, dateStamp, region, service string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secret), []byte(dateStamp))
	kRegion := hmacSHA256(kDate, []byte(region))
	kService := hmacSHA256(kRegion, []byte(service))
	return hmacSHA256(kService, []byte("aws4_request"))
}
