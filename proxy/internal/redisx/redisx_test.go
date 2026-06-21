package redisx

import (
	"context"
	"testing"
	"time"
)

func TestNewFromURL_Empty(t *testing.T) {
	c := NewFromURL("")
	if c == nil {
		t.Fatal("expected non-nil client")
	}
	if c.Enabled() {
		t.Error("empty URL should produce memClient (not enabled)")
	}
	if err := c.Ping(context.Background()); err != nil {
		t.Errorf("memClient ping should be nil, got %v", err)
	}
	if err := c.Close(); err != nil {
		t.Errorf("memClient close should be nil, got %v", err)
	}
}

func TestNewFromURL_Invalid(t *testing.T) {
	c := NewFromURL("://bad url!")
	if c == nil {
		t.Fatal("expected non-nil client for invalid URL")
	}
	if c.Enabled() {
		t.Error("invalid URL should fall back to memClient")
	}
}

func TestMemClient_SetAndGet(t *testing.T) {
	c := NewFromURL("")
	ctx := context.Background()

	// Set a value
	err := c.Set(ctx, "mykey", []byte("hello"), 10*time.Minute)
	if err != nil {
		t.Fatalf("Set: %v", err)
	}

	// Get it back
	val, ok, err := c.Get(ctx, "mykey")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !ok {
		t.Fatal("expected key to exist")
	}
	if string(val) != "hello" {
		t.Errorf("expected 'hello', got %q", val)
	}
}

func TestMemClient_GetNotFound(t *testing.T) {
	c := NewFromURL("")
	ctx := context.Background()

	_, ok, err := c.Get(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if ok {
		t.Error("expected key to not exist")
	}
}

func TestMemClient_GetExpired(t *testing.T) {
	c := NewFromURL("")
	ctx := context.Background()

	// Set with very short TTL
	err := c.Set(ctx, "ephemeral", []byte("temp"), 1*time.Millisecond)
	if err != nil {
		t.Fatalf("Set: %v", err)
	}

	// Wait for expiry
	time.Sleep(5 * time.Millisecond)

	_, ok, err := c.Get(ctx, "ephemeral")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if ok {
		t.Error("expected expired key to not exist")
	}
}

func TestMemClient_SetEmptyKey(t *testing.T) {
	c := NewFromURL("")
	ctx := context.Background()

	err := c.Set(ctx, "", []byte("value"), time.Minute)
	if err == nil {
		t.Error("expected error for empty key")
	}
}

func TestMemClient_Incr(t *testing.T) {
	c := NewFromURL("")
	ctx := context.Background()

	v1, err := c.Incr(ctx, "counter", 1, 10*time.Minute)
	if err != nil {
		t.Fatalf("Incr: %v", err)
	}
	if v1 != 1 {
		t.Errorf("expected 1, got %d", v1)
	}

	v2, err := c.Incr(ctx, "counter", 5, 10*time.Minute)
	if err != nil {
		t.Fatalf("Incr: %v", err)
	}
	if v2 != 6 {
		t.Errorf("expected 6, got %d", v2)
	}

	// Negative delta
	v3, err := c.Incr(ctx, "counter", -2, 10*time.Minute)
	if err != nil {
		t.Fatalf("Incr negative: %v", err)
	}
	if v3 != 4 {
		t.Errorf("expected 4, got %d", v3)
	}
}

func TestMemClient_IncrFloat(t *testing.T) {
	c := NewFromURL("")
	ctx := context.Background()

	v1, err := c.IncrFloat(ctx, "budget", 0.5, 10*time.Minute)
	if err != nil {
		t.Fatalf("IncrFloat: %v", err)
	}
	if v1 != 0.5 {
		t.Errorf("expected 0.5, got %f", v1)
	}

	v2, err := c.IncrFloat(ctx, "budget", 1.25, 10*time.Minute)
	if err != nil {
		t.Fatalf("IncrFloat: %v", err)
	}
	if v2 != 1.75 {
		t.Errorf("expected 1.75, got %f", v2)
	}
}

func TestMemClient_Del(t *testing.T) {
	c := NewFromURL("")
	ctx := context.Background()

	_ = c.Set(ctx, "todelete", []byte("bye"), time.Minute)
	err := c.Del(ctx, "todelete")
	if err != nil {
		t.Fatalf("Del: %v", err)
	}
	_, ok, _ := c.Get(ctx, "todelete")
	if ok {
		t.Error("expected key to be deleted")
	}
}

func TestMemClient_DelNonExistent(t *testing.T) {
	c := NewFromURL("")
	ctx := context.Background()

	// Should not error on non-existent key
	err := c.Del(ctx, "nonexistent")
	if err != nil {
		t.Errorf("Del non-existent: %v", err)
	}
}

func TestMemClient_IncrTTLExpiry(t *testing.T) {
	c := NewFromURL("")
	ctx := context.Background()

	_, err := c.Incr(ctx, "short-lived", 1, 1*time.Millisecond)
	if err != nil {
		t.Fatalf("Incr: %v", err)
	}

	time.Sleep(5 * time.Millisecond)

	_, ok, _ := c.Get(ctx, "short-lived")
	if ok {
		t.Error("expected counter to expire")
	}
}

func TestAppendInt(t *testing.T) {
	tests := []struct {
		name string
		v    int64
		want string
	}{
		{"zero", 0, "0"},
		{"positive", 42, "42"},
		{"large", 1234567890, "1234567890"},
		{"negative", -5, "-5"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := appendInt(nil, tt.v)
			if string(got) != tt.want {
				t.Errorf("appendInt(%d) = %q; want %q", tt.v, got, tt.want)
			}
		})
	}
}

func TestParseFloat(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want float64
	}{
		{"integer", "42", 42},
		{"decimal", "3.14", 3.14},
		{"negative", "-10.5", -10.5},
		{"zero", "0", 0},
		{"zero dot zero", "0.0", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out float64
			n, err := parseFloat([]byte(tt.in), &out)
			if err != nil {
				t.Fatalf("parseFloat(%q): %v", tt.in, err)
			}
			if n == 0 {
				t.Error("expected non-zero bytes consumed")
			}
			if out != tt.want {
				t.Errorf("parseFloat(%q) = %f; want %f", tt.in, out, tt.want)
			}
		})
	}
}

func TestFormatFloat_Roundtrip(t *testing.T) {
	// Use values that survive float → string → float cleanly (avoiding IEEE 754
	// rounding artifacts in the 6th decimal place).
	tests := []float64{0, 0.5, 1.0, 1.25, -42.0, 3.5, 100.0, 0.125}
	for _, v := range tests {
		got := formatFloat(v)
		var parsed float64
		_, _ = parseFloat(got, &parsed)
		if parsed != v {
			t.Errorf("roundtrip failed: %f → %q → %f", v, got, parsed)
		}
	}
}

func TestMemClient_Close(t *testing.T) {
	c := NewFromURL("")
	if err := c.Close(); err != nil {
		t.Errorf("memClient close: %v", err)
	}
	// Close should be idempotent
	if err := c.Close(); err != nil {
		t.Errorf("memClient second close: %v", err)
	}
}

func TestMemClient_IncrFloat_Expiry(t *testing.T) {
	c := NewFromURL("")
	ctx := context.Background()

	_, err := c.IncrFloat(ctx, "temp-float", 0.5, 1*time.Millisecond)
	if err != nil {
		t.Fatalf("IncrFloat: %v", err)
	}

	time.Sleep(5 * time.Millisecond)

	_, ok, _ := c.Get(ctx, "temp-float")
	if ok {
		t.Error("expected float to expire")
	}
}

func TestMemClient_SetNoTTL(t *testing.T) {
	c := NewFromURL("")
	ctx := context.Background()

	// TTL <= 0 means no expiry
	err := c.Set(ctx, "forever", []byte("value"), 0)
	if err != nil {
		t.Fatalf("Set: %v", err)
	}

	val, ok, _ := c.Get(ctx, "forever")
	if !ok {
		t.Fatal("expected key to exist (no TTL)")
	}
	if string(val) != "value" {
		t.Errorf("expected 'value', got %q", val)
	}
}

func TestMemClient_IncrNoTTL(t *testing.T) {
	c := NewFromURL("")
	ctx := context.Background()

	_, err := c.Incr(ctx, "forever-counter", 1, 0)
	if err != nil {
		t.Fatalf("Incr: %v", err)
	}

	_, ok, _ := c.Get(ctx, "forever-counter")
	if !ok {
		t.Error("expected counter to exist with no TTL")
	}
}
