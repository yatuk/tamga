package api

import (
	"fmt"
	"testing"
	"time"

	"github.com/yatuk/tamga/internal/events"
	"github.com/yatuk/tamga/internal/pricing"
)

func catalogModels(t *testing.T, catalog []map[string]interface{}, provider string) []map[string]interface{} {
	t.Helper()
	for _, p := range catalog {
		if p["id"] == provider {
			return p["models"].([]map[string]interface{})
		}
	}
	t.Fatalf("provider %q is not in the catalog", provider)
	return nil
}

func TestProviderCatalog_NamesNoModelWithoutTraffic(t *testing.T) {
	catalog := providerCatalog(nil, pricing.Defaults())
	if len(catalog) != len(providerRoutes) {
		t.Fatalf("want one entry per route (%d), got %d", len(providerRoutes), len(catalog))
	}
	for _, p := range catalog {
		if models := p["models"].([]map[string]interface{}); len(models) != 0 {
			t.Fatalf("%v lists models nobody has used: %v", p["id"], models)
		}
	}
}

func TestProviderCatalog_ListsWhatPassedThrough(t *testing.T) {
	recent := events.NewRecentBuffer(50)
	add := func(provider, model, eventType string, n int) {
		for i := 0; i < n; i++ {
			recent.Add(events.Event{
				RequestID: fmt.Sprintf("%s-%s-%s-%d", provider, model, eventType, i),
				EventType: eventType,
				Provider:  provider,
				Model:     model,
				Timestamp: time.Now().UTC(),
			})
		}
	}
	add("openai", "gpt-4o-2024-08-06", "request_scanned", 2)
	add("openai", "gpt-99", "request_scanned", 3)
	add("openai", "gpt-99", "request_blocked", 1)
	add("local", "qwen3:32b", "request_scanned", 1)
	add("openai", "", "request_scanned", 4)        // no model in the body
	add("openai", "gpt-4.1", "policy_reloaded", 5) // not a request

	catalog := providerCatalog(recent, pricing.Defaults())

	openai := catalogModels(t, catalog, "openai")
	if len(openai) != 2 {
		t.Fatalf("want gpt-99 and gpt-4o-2024-08-06, got %v", openai)
	}
	// Most used first; a model without a price says so instead of showing zero.
	if openai[0]["id"] != "gpt-99" || openai[0]["requests"] != int64(4) || openai[0]["priced"] != false {
		t.Fatalf("first openai model: %v", openai[0])
	}
	if _, has := openai[0]["input_usd"]; has {
		t.Fatalf("an unpriced model must carry no price: %v", openai[0])
	}
	if openai[1]["id"] != "gpt-4o-2024-08-06" || openai[1]["priced"] != true || openai[1]["input_usd"] != 2.5 {
		t.Fatalf("second openai model: %v", openai[1])
	}

	local := catalogModels(t, catalog, "local")
	if len(local) != 1 || local[0]["id"] != "qwen3:32b" || local[0]["priced"] != true || local[0]["input_usd"] != 0.0 {
		t.Fatalf("self-hosted model: %v", local)
	}

	if anthropic := catalogModels(t, catalog, "anthropic"); len(anthropic) != 0 {
		t.Fatalf("anthropic had no traffic, got %v", anthropic)
	}
}
