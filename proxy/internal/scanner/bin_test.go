package scanner

import (
	"path/filepath"
	"testing"
)

func TestLookupBIN_KnownPrefixes(t *testing.T) {
	globalBINLookup = nil
	path := filepath.Join("..", "..", "data", "bins.csv")
	if err := InitBINLookup(path); err != nil {
		t.Fatalf("InitBINLookup: %v", err)
	}

	visa := LookupBIN("4532015112830366")
	if visa == nil {
		t.Fatal("expected visa bin match")
	}
	if visa.Brand != "Visa" {
		t.Fatalf("visa brand mismatch: %q", visa.Brand)
	}

	mc := LookupBIN("5400030000000000")
	if mc == nil {
		t.Fatal("expected mastercard bin match")
	}
	if mc.Brand != "Mastercard" {
		t.Fatalf("mastercard brand mismatch: %q", mc.Brand)
	}
}

func TestLookupBIN_NoMatch(t *testing.T) {
	globalBINLookup = nil
	path := filepath.Join("..", "..", "data", "bins.csv")
	if err := InitBINLookup(path); err != nil {
		t.Fatalf("InitBINLookup: %v", err)
	}
	got := LookupBIN("0000001234567890")
	if got != nil {
		t.Fatalf("expected nil bin, got %+v", *got)
	}
}

func TestCreditCardConfidence_WithAndWithoutBIN(t *testing.T) {
	globalBINLookup = nil
	path := filepath.Join("..", "..", "data", "bins.csv")
	if err := InitBINLookup(path); err != nil {
		t.Fatalf("InitBINLookup: %v", err)
	}

	withBIN := calculatePIIConfidence("credit_card", "number 4532015112830366", 7, "4532015112830366")
	if withBIN.Total != 80 {
		t.Fatalf("with bin total: got=%d want=80", withBIN.Total)
	}

	withoutBIN := calculatePIIConfidence("credit_card", "number 4242424242424242", 7, "4242424242424242")
	if withoutBIN.Total != 60 {
		t.Fatalf("without bin total: got=%d want=60", withoutBIN.Total)
	}
}
