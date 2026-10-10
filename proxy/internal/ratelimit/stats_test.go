package ratelimit

import (
	"fmt"
	"strings"
	"testing"

	"github.com/yatuk/tamga/internal/policy"
)

func statsLimiter(t *testing.T) *Limiter {
	t.Helper()
	p, err := policy.LoadFromBytes([]byte(`version: "1.0"
rate_limit:
  max_requests_per_minute: 100000
  max_tokens_per_day: 100000000
`))
	if err != nil {
		t.Fatal(err)
	}
	l := NewLimiter(func() *policy.Policy { return p })
	t.Cleanup(func() { _ = l.Close() })
	return l
}

// TopKeys is the keys with the most requests, not every key the limiter has
// seen: the limiter counts one per client address for callers without a key.
func TestGetStats_TopKeysIsBounded(t *testing.T) {
	l := statsLimiter(t)
	const keys = TopKeysLimit + 15
	// Key i makes i+1 requests, so the busiest are the last ones.
	for i := 0; i < keys; i++ {
		for n := 0; n <= i; n++ {
			l.Check(fmt.Sprintf("ip:10.0.0.%d", i))
		}
	}
	s := l.GetStats()
	if len(s.TopKeys) != TopKeysLimit {
		t.Fatalf("%d keys in the snapshot, want %d", len(s.TopKeys), TopKeysLimit)
	}
	for i := keys - TopKeysLimit; i < keys; i++ {
		key := fmt.Sprintf("ip:10.0.0.%d", i)
		if got := s.TopKeys[key]; got != int64(i+1) {
			t.Errorf("%s: %d requests in the snapshot, want %d", key, got, i+1)
		}
	}
	if _, ok := s.TopKeys["ip:10.0.0.0"]; ok {
		t.Error("the quietest key is in the snapshot")
	}
	if want := int64(keys * (keys + 1) / 2); s.TotalRequests != want {
		t.Errorf("total requests = %d, want %d: the total counts every key", s.TotalRequests, want)
	}
}

// The daily token totals are kept apart from the request counts: a snapshot
// used to list "dtq:<key>:<date>" entries whose value was a number of
// tokens, next to keys whose value was a number of requests.
func TestGetStats_TopKeysHoldsRequestCountsOnly(t *testing.T) {
	l := statsLimiter(t)
	l.Check("tk:key-1")
	l.Check("tk:key-1")
	if res := l.CheckDailyTokenQuota("tk:key-1", 4321); !res.Allowed || res.TokensUsed != 4321 {
		t.Fatalf("daily quota: %+v", res)
	}
	if res := l.CheckDailyTokenQuota("tk:key-1", 1000); res.TokensUsed != 5321 {
		t.Fatalf("the daily total is not kept: %+v", res)
	}
	s := l.GetStats()
	if len(s.TopKeys) != 1 || s.TopKeys["tk:key-1"] != 2 {
		t.Fatalf("top keys = %v, want tk:key-1 with 2 requests", s.TopKeys)
	}
	for key := range s.TopKeys {
		if strings.HasPrefix(key, "dtq:") {
			t.Errorf("a token total is listed as a key: %s", key)
		}
	}
}
