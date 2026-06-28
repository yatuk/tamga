package tenant

import (
	"fmt"
	"strings"
	"testing"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name  string
		orgID string
	}{
		{name: "normal org", orgID: "acme"},
		{name: "empty org", orgID: ""},
		{name: "org with special chars", orgID: "org:1/slash"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ns := New(tt.orgID)
			if ns == nil {
				t.Fatal("New() returned nil")
			}
			if ns.OrgID != tt.orgID {
				t.Fatalf("OrgID mismatch: got=%q want=%q", ns.OrgID, tt.orgID)
			}
		})
	}
}

func TestNamespace_RateLimit(t *testing.T) {
	tests := []struct {
		name           string
		orgID          string
		apiKey         string
		minute         int64
		wantPrefix     string
		skipColonCheck bool
	}{
		{name: "normal org key and minute", orgID: "acme", apiKey: "sk-abc123", minute: 1718140000, wantPrefix: "tamga:rl:"},
		{name: "empty orgID", orgID: "", apiKey: "sk-abc", minute: 0, wantPrefix: "tamga:rl:"},
		{name: "empty apiKey", orgID: "acme", apiKey: "", minute: 999, wantPrefix: "tamga:rl:"},
		{name: "negative minute", orgID: "acme", apiKey: "sk-x", minute: -1, wantPrefix: "tamga:rl:"},
		{name: "orgID with colon", orgID: "org:1", apiKey: "sk-a", minute: 1, wantPrefix: "tamga:rl:", skipColonCheck: true},
		{name: "orgID with slash", orgID: "org/1", apiKey: "sk-a", minute: 1, wantPrefix: "tamga:rl:"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ns := New(tt.orgID)
			got := ns.RateLimit(tt.apiKey, tt.minute)

			// Structural assertions: prefix, orgID presence, colon count, field presence.
			if !strings.HasPrefix(got, tt.wantPrefix) {
				t.Fatalf("prefix mismatch: got=%q want prefix=%q", got, tt.wantPrefix)
			}
			if !strings.Contains(got, tt.orgID) {
				t.Fatalf("key missing orgID: got=%q orgID=%q", got, tt.orgID)
			}
			if !tt.skipColonCheck && strings.Count(got, ":") != 4 {
				t.Fatalf("colon count mismatch: got=%d want=4 (key=%q)", strings.Count(got, ":"), got)
			}

			minuteStr := fmt.Sprintf("%d", tt.minute)
			if !strings.Contains(got, minuteStr) {
				t.Fatalf("key missing minute: got=%q minute=%q", got, minuteStr)
			}
			if !strings.Contains(got, tt.apiKey) {
				t.Fatalf("key missing apiKey: got=%q apiKey=%q", got, tt.apiKey)
			}
		})
	}
}

func TestNamespace_DailyTokenQuota(t *testing.T) {
	tests := []struct {
		name           string
		orgID          string
		apiKey         string
		date           string
		skipColonCheck bool
	}{
		{name: "normal", orgID: "acme", apiKey: "sk-abc123", date: "2024-06-17"},
		{name: "empty orgID", orgID: "", apiKey: "sk-abc", date: "2024-01-01"},
		{name: "empty apiKey", orgID: "acme", apiKey: "", date: "2024-12-31"},
		{name: "empty date", orgID: "acme", apiKey: "sk-x", date: ""},
		{name: "orgID with colon", orgID: "org:1", apiKey: "sk-a", date: "2024-06-17", skipColonCheck: true},
		{name: "orgID with slash", orgID: "org/1", apiKey: "sk-a", date: "2024-06-17"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ns := New(tt.orgID)
			got := ns.DailyTokenQuota(tt.apiKey, tt.date)

			if !strings.HasPrefix(got, "tamga:dtq:") {
				t.Fatalf("prefix mismatch: got=%q want prefix=tamga:dtq:", got)
			}
			if !strings.Contains(got, tt.orgID) {
				t.Fatalf("key missing orgID: got=%q orgID=%q", got, tt.orgID)
			}
			if !tt.skipColonCheck && strings.Count(got, ":") != 4 {
				t.Fatalf("colon count mismatch: got=%d want=4 (key=%q)", strings.Count(got, ":"), got)
			}
			if !strings.Contains(got, tt.apiKey) {
				t.Fatalf("key missing apiKey: got=%q apiKey=%q", got, tt.apiKey)
			}
			if !strings.Contains(got, tt.date) {
				t.Fatalf("key missing date: got=%q date=%q", got, tt.date)
			}
		})
	}
}

func TestNamespace_Budget(t *testing.T) {
	tests := []struct {
		name           string
		orgID          string
		date           string
		counter        string
		skipColonCheck bool
	}{
		{name: "normal", orgID: "acme", date: "2024-06-17", counter: "token"},
		{name: "empty orgID", orgID: "", date: "2024-01-01", counter: "req"},
		{name: "empty date", orgID: "acme", date: "", counter: "token"},
		{name: "empty counter", orgID: "acme", date: "2024-06-17", counter: ""},
		{name: "orgID with colon", orgID: "org:1", date: "2024-06-17", counter: "token", skipColonCheck: true},
		{name: "orgID with slash", orgID: "org/1", date: "2024-06-17", counter: "token"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ns := New(tt.orgID)
			got := ns.Budget(tt.date, tt.counter)

			if !strings.HasPrefix(got, "tamga:budget:") {
				t.Fatalf("prefix mismatch: got=%q want prefix=tamga:budget:", got)
			}
			if !strings.Contains(got, tt.orgID) {
				t.Fatalf("key missing orgID: got=%q orgID=%q", got, tt.orgID)
			}
			if !tt.skipColonCheck && strings.Count(got, ":") != 4 {
				t.Fatalf("colon count mismatch: got=%d want=4 (key=%q)", strings.Count(got, ":"), got)
			}
			if !strings.Contains(got, tt.date) {
				t.Fatalf("key missing date: got=%q date=%q", got, tt.date)
			}
			if !strings.Contains(got, tt.counter) {
				t.Fatalf("key missing counter: got=%q counter=%q", got, tt.counter)
			}
		})
	}
}

func TestNamespace_Cache(t *testing.T) {
	tests := []struct {
		name           string
		orgID          string
		hash           string
		skipColonCheck bool
	}{
		{name: "normal", orgID: "acme", hash: "abc123def456"},
		{name: "empty orgID", orgID: "", hash: "deadbeef"},
		{name: "empty hash", orgID: "acme", hash: ""},
		{name: "orgID with colon", orgID: "org:1", hash: "abcdef", skipColonCheck: true},
		{name: "orgID with slash", orgID: "org/1", hash: "abcdef"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ns := New(tt.orgID)
			got := ns.Cache(tt.hash)

			if !strings.HasPrefix(got, "tamga:cache:") {
				t.Fatalf("prefix mismatch: got=%q want prefix=tamga:cache:", got)
			}
			if !strings.Contains(got, tt.orgID) {
				t.Fatalf("key missing orgID: got=%q orgID=%q", got, tt.orgID)
			}
			if !tt.skipColonCheck && strings.Count(got, ":") != 3 {
				t.Fatalf("colon count mismatch: got=%d want=3 (key=%q)", strings.Count(got, ":"), got)
			}
			if !strings.Contains(got, tt.hash) {
				t.Fatalf("key missing hash: got=%q hash=%q", got, tt.hash)
			}
		})
	}
}
