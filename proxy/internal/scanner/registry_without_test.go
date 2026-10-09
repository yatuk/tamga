package scanner

import (
	"context"
	"testing"
)

func TestRegistryWithout(t *testing.T) {
	kept := &countingScanner{name: "kept", findings: []Finding{{Type: "pii", Category: "email"}}}
	dropped := &countingScanner{name: "dropped", findings: []Finding{{Type: "secret", Category: "aws_key"}}}
	r := NewRegistry()
	r.Register(kept)
	r.Register(dropped)
	r.SetSpeed("kept", SpeedSlow)
	r.SetSpeed("dropped", SpeedSlow)

	w := r.Without("dropped", "not_registered")
	if w.Count() != 1 {
		t.Fatalf("Count = %d, want 1", w.Count())
	}
	found, err := w.ScanAll(context.Background(), []byte("text"))
	if err != nil || len(found) != 1 || found[0].Type != "pii" {
		t.Fatalf("findings = %+v, err = %v", found, err)
	}
	if w.speeds["kept"] != SpeedSlow {
		t.Fatalf("speed class of the kept scanner was lost: %v", w.speeds)
	}
	if _, ok := w.speeds["dropped"]; ok {
		t.Fatal("speed class of the dropped scanner was kept")
	}
	// The registry it came from is as it was.
	if r.Count() != 2 {
		t.Fatalf("source Count = %d, want 2", r.Count())
	}
}
