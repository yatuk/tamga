package main

import (
	"encoding/json"
	"os"
	"testing"
)

// TestWriteBenignDevTexts writes the benign developer texts, one JSON
// object per line ({"id", "text"}), to the file TAMGA_BENIGN_DEV_OUT names.
// classifier/finetune/sweep.py runs it to add them to its benign pool; the
// texts live here, and some are built by code, so they are not kept twice.
// Without the variable it is skipped.
func TestWriteBenignDevTexts(t *testing.T) {
	path := os.Getenv("TAMGA_BENIGN_DEV_OUT")
	if path == "" {
		t.Skip("set TAMGA_BENIGN_DEV_OUT to write the texts")
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	enc := json.NewEncoder(f)
	for _, b := range benignDevTexts {
		if err := enc.Encode(map[string]string{"id": b.id, "text": b.text}); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}
