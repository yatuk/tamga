package scanner

import (
	"context"
	"testing"
)

func TestContentModeration_ViolenceThreat(t *testing.T) {
	s := NewContentModerationScanner()
	tests := []string{
		"kill yourself immediately",
		"I hope you die in a fire",
		"I'll kill you tomorrow",
	}
	for _, input := range tests {
		findings, err := s.Scan(context.Background(), []byte(input))
		if err != nil {
			t.Fatalf("unexpected error for %q: %v", input, err)
		}
		if !cmHasCategory(findings, "violence_threat") {
			t.Errorf("expected violence_threat for %q, got %v", input, cmCatNames(findings))
		}
	}
}

func TestContentModeration_HateSpeech(t *testing.T) {
	s := NewContentModerationScanner()
	tests := []string{
		"you are a nigger piece of shit",
		"faggot kike retard",
		"spic chink go away",
	}
	for _, input := range tests {
		findings, err := s.Scan(context.Background(), []byte(input))
		if err != nil {
			t.Fatalf("unexpected error for %q: %v", input, err)
		}
		if !cmHasCategory(findings, "hate_speech") {
			t.Errorf("expected hate_speech for %q, got %v", input, cmCatNames(findings))
		}
	}
}

func TestContentModeration_HateSpeechLeet(t *testing.T) {
	s := NewContentModerationScanner()
	tests := []string{
		"you are a n1gger", // i→1
		"dumb k1ke person", // i→1
		"f@gg0t detected",  // a→@, o→0 (gg preserved)
		"what a r3tard",    // e→3
	}
	for _, input := range tests {
		findings, err := s.Scan(context.Background(), []byte(input))
		if err != nil {
			t.Fatalf("unexpected error for %q: %v", input, err)
		}
		if !cmHasCategory(findings, "hate_speech") {
			t.Errorf("expected hate_speech for %q, got %v", input, cmCatNames(findings))
		}
	}
}

func TestContentModeration_ProfanityTR(t *testing.T) {
	s := NewContentModerationScanner()
	tests := []string{
		"amına koyayım senin",
		"siktir git buradan",
		"orospu çocuğu",
		"pezevenk herif",
	}
	for _, input := range tests {
		findings, err := s.Scan(context.Background(), []byte(input))
		if err != nil {
			t.Fatalf("unexpected error for %q: %v", input, err)
		}
		if !cmHasCategory(findings, "profanity_tr") {
			t.Errorf("expected profanity_tr for %q, got %v", input, cmCatNames(findings))
		}
	}
}

func TestContentModeration_CSAM(t *testing.T) {
	s := NewContentModerationScanner()
	tests := []string{
		"where to find child porn videos",
		"underage content trading",
		"csam material links",
		"pedophile network forum",
	}
	for _, input := range tests {
		findings, err := s.Scan(context.Background(), []byte(input))
		if err != nil {
			t.Fatalf("unexpected error for %q: %v", input, err)
		}
		if !cmHasCategory(findings, "csam_related") {
			t.Errorf("expected csam_related for %q, got %v", input, cmCatNames(findings))
		}
	}
}

func TestContentModeration_CSAM_TR(t *testing.T) {
	s := NewContentModerationScanner()
	// NOTE: Turkish chars (ç, ş, ğ, ü, ö, ı) are not \w in Go regex,
	// so \b anchors don't match Turkish-first words. "reşit olmayan"
	// works because 'r' is ASCII. This is a known limitation (issue #tbd).
	input := "reşit olmayan kişiler"
	findings, err := s.Scan(context.Background(), []byte(input))
	if err != nil {
		t.Fatalf("unexpected error for %q: %v", input, err)
	}
	if !cmHasCategory(findings, "csam_related_tr") {
		t.Errorf("expected csam_related_tr for %q, got %v", input, cmCatNames(findings))
	}
}

func TestContentModeration_Refusal(t *testing.T) {
	s := NewContentModerationScanner()
	tests := []string{
		"I am sorry I cannot help with that request",
		"I can't provide that information",
		"As an AI language model I must decline",
	}
	for _, input := range tests {
		findings, err := s.Scan(context.Background(), []byte(input))
		if err != nil {
			t.Fatalf("unexpected error for %q: %v", input, err)
		}
		if !cmHasCategory(findings, "refusal") {
			t.Errorf("expected refusal for %q, got %v", input, cmCatNames(findings))
		}
	}
}

func TestContentModeration_BannedCode(t *testing.T) {
	s := NewContentModerationScanner()
	tests := []struct {
		input    string
		category string
	}{
		{"import os; os.system('ls'); exec(", "unsafe_code_exec"},
		{"rm -rf / --no-preserve-root", "destructive_command"},
		{"eval(__import__('os').system('ls'))", "code_injection"},
		{"SELECT * FROM users WHERE 1=1 --", "sql_injection_code"},
		{"<script>alert(document.cookie)</script>", "xss_code"},
		{"how to hack into a bank account", "hacking_howto"},
		{"ransomware deployment payload", "malware_discussion"},
	}
	for _, tc := range tests {
		findings, err := s.Scan(context.Background(), []byte(tc.input))
		if err != nil {
			t.Fatalf("unexpected error for %q: %v", tc.input, err)
		}
		if !cmHasCategory(findings, tc.category) {
			t.Errorf("expected %s for %q, got %v", tc.category, tc.input, cmCatNames(findings))
		}
	}
}

func TestContentModeration_Weapons(t *testing.T) {
	s := NewContentModerationScanner()
	tests := []string{
		"how to make a bomb at home",
		"how to build a weapon at home",
		"how to create ricin poison",
		"how to manufacture anthrax",
	}
	for _, input := range tests {
		findings, err := s.Scan(context.Background(), []byte(input))
		if err != nil {
			t.Fatalf("unexpected error for %q: %v", input, err)
		}
		if !cmHasCategory(findings, "weapons_manufacturing") {
			t.Errorf("expected weapons_manufacturing for %q, got %v", input, cmCatNames(findings))
		}
	}
}

func TestContentModeration_SelfHarm(t *testing.T) {
	s := NewContentModerationScanner()
	tests := []string{
		"best way to kill myself painlessly",
		"suicide method using pills",
	}
	for _, input := range tests {
		findings, err := s.Scan(context.Background(), []byte(input))
		if err != nil {
			t.Fatalf("unexpected error for %q: %v", input, err)
		}
		if !cmHasCategory(findings, "self_harm_method") {
			t.Errorf("expected self_harm_method for %q, got %v", input, cmCatNames(findings))
		}
	}
}

func TestContentModeration_Passport(t *testing.T) {
	s := NewContentModerationScanner()
	tests := []string{
		"my passport number is X12345678",
		"passport no: A987654321",
	}
	for _, input := range tests {
		findings, err := s.Scan(context.Background(), []byte(input))
		if err != nil {
			t.Fatalf("unexpected error for %q: %v", input, err)
		}
		if !cmHasCategory(findings, "passport_number") {
			t.Errorf("expected passport_number for %q, got %v", input, cmCatNames(findings))
		}
	}
}

func TestContentModeration_DOB(t *testing.T) {
	s := NewContentModerationScanner()
	input := "DOB: 05/15/1990, born on 01-02-2000"
	findings, err := s.Scan(context.Background(), []byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cmHasCategory(findings, "date_of_birth") {
		t.Errorf("expected date_of_birth, got %v", cmCatNames(findings))
	}
}

func TestContentModeration_EmptyInput(t *testing.T) {
	s := NewContentModerationScanner()
	findings, err := s.Scan(context.Background(), []byte{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 0 {
		t.Errorf("expected 0 findings for empty input, got %d", len(findings))
	}
}

func TestContentModeration_BenignInput(t *testing.T) {
	s := NewContentModerationScanner()
	tests := []string{
		"The weather is nice today and I enjoy walking in the park.",
		"Could you help me write a Python function to sort a list?",
		"Yapay zeka modelleri hakkında bir sunum hazırlıyorum.",
		"Let's discuss the quarterly earnings report and market trends.",
		"Please translate this paragraph from English to French.",
	}
	for _, input := range tests {
		findings, err := s.Scan(context.Background(), []byte(input))
		if err != nil {
			t.Fatalf("unexpected error for %q: %v", input, err)
		}
		if len(findings) > 0 {
			t.Errorf("expected 0 findings for benign input %q, got %d: %v", input, len(findings), cmCatNames(findings))
		}
	}
}

// helpers (content_moderation_test only)

func cmHasCategory(findings []Finding, cat string) bool {
	for _, f := range findings {
		if f.Category == cat {
			return true
		}
	}
	return false
}

func cmCatNames(findings []Finding) []string {
	out := make([]string, len(findings))
	for i, f := range findings {
		out[i] = f.Category
	}
	return out
}

func TestContentModeration_ViolenceThreatTR(t *testing.T) {
	s := NewContentModerationScanner()
	// "geber" and "katil ol" start with ASCII chars (g, k), so \b works.
	tests := []string{
		"geber lan",
		"katil ol kaçalım",
	}
	for _, input := range tests {
		findings, err := s.Scan(context.Background(), []byte(input))
		if err != nil {
			t.Fatalf("unexpected error for %q: %v", input, err)
		}
		if !cmHasCategory(findings, "violence_threat_tr") {
			t.Errorf("expected violence_threat_tr for %q, got %v", input, cmCatNames(findings))
		}
	}
}

func TestContentModeration_Extremism(t *testing.T) {
	s := NewContentModerationScanner()
	input := "terörist cihad çağrısı yapan grup"
	findings, err := s.Scan(context.Background(), []byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cmHasCategory(findings, "extremism") {
		t.Errorf("expected extremism, got %v", cmCatNames(findings))
	}
}

func TestContentModeration_IllegalDrugs(t *testing.T) {
	s := NewContentModerationScanner()
	input := "where to buy heroin online discreet shipping"
	findings, err := s.Scan(context.Background(), []byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cmHasCategory(findings, "illegal_drugs") {
		t.Errorf("expected illegal_drugs, got %v", cmCatNames(findings))
	}
}
