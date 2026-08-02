package vault

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/yatuk/tamga/internal/scanner"
)

func TestCipherRoundTrip(t *testing.T) {
	keyB64, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	key, err := KeyFromBase64(keyB64)
	if err != nil {
		t.Fatalf("KeyFromBase64: %v", err)
	}
	c, err := NewCipher(key)
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}
	for _, pt := range []string{"", "Ahmet Yılmaz", "12345678950", "TC: 12345678950, IBAN TR33..."} {
		blob, err := c.Encrypt(pt)
		if err != nil {
			t.Fatalf("Encrypt(%q): %v", pt, err)
		}
		got, err := c.Decrypt(blob)
		if err != nil {
			t.Fatalf("Decrypt: %v", err)
		}
		if got != pt {
			t.Errorf("round-trip: got %q want %q", got, pt)
		}
	}
}

func TestCipher_WrongKeyFails(t *testing.T) {
	k1, _ := GenerateKey()
	k2, _ := GenerateKey()
	key1, _ := KeyFromBase64(k1)
	key2, _ := KeyFromBase64(k2)
	c1, _ := NewCipher(key1)
	c2, _ := NewCipher(key2)
	blob, _ := c1.Encrypt("secret")
	if _, err := c2.Decrypt(blob); err == nil {
		t.Error("decrypt with wrong key should fail (GCM auth)")
	}
}

func TestNewCipher_BadKeyLength(t *testing.T) {
	if _, err := NewCipher([]byte("short")); err == nil {
		t.Error("expected error for non-32-byte key")
	}
}

func TestTokenize_NumbersAndMaps(t *testing.T) {
	content := []byte("Ahmet icin ozet, TC 12345678950 ve TC 10000000146")
	span := func(sub string) (int, int) {
		i := strings.Index(string(content), sub)
		return i, i + len(sub)
	}
	p0, p1 := span("Ahmet")
	t0, t1 := span("12345678950")
	u0, u1 := span("10000000146")
	findings := []scanner.Finding{
		{Type: "pii", Category: "person", StartPos: p0, EndPos: p1},
		{Type: "pii", Category: "tc_kimlik", StartPos: t0, EndPos: t1},
		{Type: "pii", Category: "tc_kimlik", StartPos: u0, EndPos: u1},
	}
	out, mapping := Tokenize(content, findings)
	s := string(out)

	if !strings.Contains(s, "[TAMGA_PERSON_1]") {
		t.Errorf("missing person placeholder: %q", s)
	}
	if !strings.Contains(s, "[TAMGA_TC_KIMLIK_1]") || !strings.Contains(s, "[TAMGA_TC_KIMLIK_2]") {
		t.Errorf("TC placeholders not numbered per category: %q", s)
	}
	if mapping["[TAMGA_PERSON_1]"] != "Ahmet" {
		t.Errorf("person mapping = %q", mapping["[TAMGA_PERSON_1]"])
	}
	if mapping["[TAMGA_TC_KIMLIK_1]"] != "12345678950" || mapping["[TAMGA_TC_KIMLIK_2]"] != "10000000146" {
		t.Errorf("TC mapping wrong: %+v", mapping)
	}

	// Restore must reproduce the original exactly.
	restored := Restore(out, mapping)
	if string(restored) != string(content) {
		t.Errorf("restore mismatch:\n got %q\nwant %q", restored, content)
	}
}

func TestTokenize_IgnoresNonPII(t *testing.T) {
	content := []byte("hello world")
	findings := []scanner.Finding{{Type: "injection", Category: "x", StartPos: 0, EndPos: 5}}
	out, mapping := Tokenize(content, findings)
	if string(out) != "hello world" || len(mapping) != 0 {
		t.Errorf("non-pii finding should not tokenize: out=%q map=%v", out, mapping)
	}
}

func TestRestore_NilMapping(t *testing.T) {
	body := []byte("nothing to do")
	if got := Restore(body, nil); string(got) != "nothing to do" {
		t.Errorf("nil mapping should pass through, got %q", got)
	}
}

// mockRedis is a map-backed RedisClient for store tests.
type mockRedis struct{ data map[string][]byte }

func newMockRedis() *mockRedis { return &mockRedis{data: map[string][]byte{}} }
func (m *mockRedis) Get(_ context.Context, k string) ([]byte, bool, error) {
	v, ok := m.data[k]
	return v, ok, nil
}
func (m *mockRedis) Set(_ context.Context, k string, v []byte, _ time.Duration) error {
	m.data[k] = v
	return nil
}
func (m *mockRedis) Del(_ context.Context, k string) error { delete(m.data, k); return nil }

func TestStore_SaveLoadDelete(t *testing.T) {
	keyB64, _ := GenerateKey()
	key, _ := KeyFromBase64(keyB64)
	c, _ := NewCipher(key)
	s := NewStore(newMockRedis(), c, time.Minute)
	if !s.Enabled() {
		t.Fatal("store should be enabled with client+cipher")
	}
	mapping := map[string]string{"[TAMGA_TC_KIMLIK_1]": "12345678950", "[TAMGA_PERSON_1]": "Ahmet"}
	ctx := context.Background()
	if err := s.Save(ctx, "req-1", mapping); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := s.Load(ctx, "req-1")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got["[TAMGA_TC_KIMLIK_1]"] != "12345678950" || got["[TAMGA_PERSON_1]"] != "Ahmet" {
		t.Errorf("loaded mapping wrong: %+v", got)
	}
	s.Delete(ctx, "req-1")
	after, _ := s.Load(ctx, "req-1")
	if len(after) != 0 {
		t.Errorf("after delete expected empty, got %+v", after)
	}
}

func TestStore_DisabledNoop(t *testing.T) {
	s := NewStore(nil, nil, time.Minute)
	if s.Enabled() {
		t.Error("nil client/cipher store should be disabled")
	}
	if err := s.Save(context.Background(), "r", map[string]string{"a": "b"}); err != nil {
		t.Errorf("disabled Save should no-op, got %v", err)
	}
	got, _ := s.Load(context.Background(), "r")
	if len(got) != 0 {
		t.Errorf("disabled Load should be empty, got %v", got)
	}
}

// TestStore_EncryptsAtRest verifies the raw stored bytes do not contain the
// plaintext (it is AES-GCM encrypted).
func TestStore_EncryptsAtRest(t *testing.T) {
	keyB64, _ := GenerateKey()
	key, _ := KeyFromBase64(keyB64)
	c, _ := NewCipher(key)
	m := newMockRedis()
	s := NewStore(m, c, time.Minute)
	_ = s.Save(context.Background(), "req-x", map[string]string{"[TAMGA_TC_KIMLIK_1]": "12345678950"})
	for _, raw := range m.data {
		if strings.Contains(string(raw), "12345678950") {
			t.Error("plaintext TCKN found in stored bytes — not encrypted at rest")
		}
	}
}
