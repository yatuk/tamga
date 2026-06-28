package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/yatuk/tamga/internal/events"
)

func TestLiveEvents_NoBroker(t *testing.T) {
	cfg := Config{
		AdminKey:     "test-key",
		DefaultOrgID: "org-1",
		Started:      time.Now(),
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/live/events", nil)
	adminHeaders(cfg.AdminKey)(req)
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp.StatusCode)
	}
}

func TestLiveEvents_BrokerAvailable(t *testing.T) {
	broker := events.NewBroker(8)

	cfg := Config{
		AdminKey:     "test-key",
		Broker:       broker,
		DefaultOrgID: "org-1",
		Started:      time.Now(),
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/live/events?key=test-key", nil)
	// Use query param auth because EventSource can't send custom headers
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}
	ct := resp.Header.Get("Content-Type")
	if ct != "text/event-stream" {
		t.Errorf("expected 'text/event-stream', got %q", ct)
	}
	if resp.Header.Get("Cache-Control") != "no-cache" {
		t.Error("expected Cache-Control: no-cache")
	}
	if resp.Header.Get("Connection") != "keep-alive" {
		t.Error("expected Connection: keep-alive")
	}
}

func TestLiveEvents_Unauthorized(t *testing.T) {
	cfg := Config{
		AdminKey:     "secret-key",
		DefaultOrgID: "org-1",
		Started:      time.Now(),
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/live/events")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}
