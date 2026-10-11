package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/yatuk/tamga/internal/events"
	"github.com/yatuk/tamga/internal/providers"
)

// The event list and the export, answered from the events in memory, call
// the same requests shadow AI: those providers.IsShadow does. The export
// had a list of its own, without the LiteLLM bridge.
func TestShadowFilter_ListAndExportAgree(t *testing.T) {
	recorded := []string{
		"deepseek", "DeepSeek", " groq ", "openai2",
		"openai", "OpenAI", "anthropic", "google", "azure", "azure_openai", "google_vertex",
		"litellm", "",
	}
	now := time.Now().UTC()
	rb := events.NewRecentBuffer(50)
	var want []string
	for i, p := range recorded {
		id := fmt.Sprintf("evt-%02d", i)
		rb.Add(events.Event{RequestID: id, EventType: "request_scanned", Provider: p, Timestamp: now.Add(-time.Hour)})
		if providers.IsShadow(p) {
			want = append(want, id)
		}
	}
	if len(want) != 4 {
		t.Fatalf("%d shadow providers among the recorded ones, want 4", len(want))
	}
	ts := httptest.NewServer(testMux(Config{AdminKey: "test-key", Recent: rb}))
	defer ts.Close()

	for _, path := range []string{
		"/api/v1/events?shadow=true&limit=100",
		"/api/v1/events?provider=shadow&limit=100",
		"/api/v1/events/export?format=json&provider=shadow",
	} {
		req, _ := http.NewRequest(http.MethodGet, ts.URL+path, nil)
		req.Header.Set("X-Tamga-Admin-Key", "test-key")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		var body struct {
			Events []struct {
				RequestID string `json:"request_id"`
			} `json:"events"`
		}
		err = json.NewDecoder(resp.Body).Decode(&body)
		_ = resp.Body.Close()
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("%s: status %d, %v", path, resp.StatusCode, err)
		}
		var got []string
		for _, e := range body.Events {
			got = append(got, e.RequestID)
		}
		sort.Strings(got)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: shadow = %v, want %v", path, got, want)
		}
	}
}
