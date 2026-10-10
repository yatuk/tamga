package config

import "testing"

// An address for the analyzer is set in every Compose deployment, for the
// PDF reports. Copying request text to it is a separate choice, off unless
// made.
func TestLoad_AnalyzerSendContentIsOffUnlessSet(t *testing.T) {
	t.Setenv("TAMGA_ANALYZER_ADDR", "analyzer:50051")
	t.Setenv("TAMGA_ANALYZER_SEND_CONTENT", "")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.AnalyzerSendContent {
		t.Fatal("content goes to the analyzer with only its address set")
	}

	t.Setenv("TAMGA_ANALYZER_SEND_CONTENT", "true")
	cfg, err = Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.AnalyzerSendContent {
		t.Fatal("TAMGA_ANALYZER_SEND_CONTENT=true was not read")
	}
}
