package proxy

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestSignBedrockRequestAttachesHeaders(t *testing.T) {
	body := []byte(`{"prompt":"hello"}`)
	req, err := http.NewRequest(http.MethodPost,
		"https://bedrock-runtime.us-east-1.amazonaws.com/model/anthropic.claude-3-sonnet/invoke",
		io.NopCloser(bytes.NewReader(body)))
	if err != nil {
		t.Fatalf("new req: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	creds := SigV4Credentials{
		AccessKeyID:     "AKIDEXAMPLE",
		SecretAccessKey: "wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY",
		Region:          "us-east-1",
		Service:         "bedrock",
	}
	signBedrockRequest(req, body, creds, time.Date(2026, time.April, 17, 12, 0, 0, 0, time.UTC))

	auth := req.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "AWS4-HMAC-SHA256 Credential=AKIDEXAMPLE/20260417/us-east-1/bedrock/aws4_request") {
		t.Fatalf("auth header malformed: %q", auth)
	}
	if !strings.Contains(auth, "SignedHeaders=") {
		t.Fatalf("missing SignedHeaders: %q", auth)
	}
	if !strings.Contains(auth, "Signature=") {
		t.Fatalf("missing Signature: %q", auth)
	}
	if got := req.Header.Get("X-Amz-Date"); got != "20260417T120000Z" {
		t.Fatalf("x-amz-date = %q", got)
	}
	if got := req.Header.Get("X-Amz-Content-Sha256"); len(got) != 64 {
		t.Fatalf("x-amz-content-sha256 malformed: %q", got)
	}
}

func TestSignBedrockRequestNoCredsNoop(t *testing.T) {
	body := []byte(`{}`)
	req, _ := http.NewRequest(http.MethodPost, "https://example.com/", bytes.NewReader(body))
	signBedrockRequest(req, body, SigV4Credentials{}, time.Now())
	if req.Header.Get("Authorization") != "" {
		t.Fatalf("expected no Authorization when creds empty")
	}
}

func TestBedrockCredentialsFromEnv(t *testing.T) {
	orig := envLookup
	t.Cleanup(func() { envLookup = orig })
	envs := map[string]string{
		"AWS_ACCESS_KEY_ID":     "AKID",
		"AWS_SECRET_ACCESS_KEY": "sekret",
		"AWS_REGION":            "eu-west-1",
		"AWS_SESSION_TOKEN":     "token",
	}
	envLookup = func(k string) string { return envs[k] }

	c, ok := bedrockCredentials()
	if !ok {
		t.Fatalf("expected creds ok")
	}
	if c.AccessKeyID != "AKID" || c.Region != "eu-west-1" || c.SessionToken != "token" {
		t.Fatalf("unexpected creds: %+v", c)
	}
}
