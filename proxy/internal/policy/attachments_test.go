package policy

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/yatuk/tamga/internal/scanner"
)

func parseAttachments(t *testing.T, section string) (*Policy, error) {
	t.Helper()
	return LoadFromBytes([]byte("version: \"1.0\"\nname: att\nattachments:\n" + section + "\nproviders:\n  allowed: [openai]\n"))
}

func attachmentFinding(category string) scanner.Finding {
	return scanner.Finding{Type: scanner.AttachmentFindingType, Category: category, Severity: "medium", Confidence: 1, Role: "user", Seg: -1}
}

func TestAttachmentSpec_AbsentIsOff(t *testing.T) {
	if (*Policy)(nil).AttachmentSpec().Enabled {
		t.Fatal("nil policy")
	}
	shipped, err := LoadFromFile(defaultPolicyPath)
	if err != nil {
		t.Fatal(err)
	}
	if shipped.AttachmentSpec() != (AttachmentSpec{}) {
		t.Fatalf("the shipped policy looks at attachments: %+v", shipped.AttachmentSpec())
	}
	if _, ok := shipped.attachmentAction(scanner.AttachmentEncrypted); ok {
		t.Fatal("an action without the section")
	}
	raw, err := json.Marshal(shipped)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"attachments"`) {
		t.Fatal("a policy without the section writes it in its JSON form, and changes its digest")
	}
}

func TestAttachmentSpec_Defaults(t *testing.T) {
	p, err := parseAttachments(t, "  unreadable: warn")
	if err != nil {
		t.Fatal(err)
	}
	want := AttachmentSpec{Enabled: true, MaxBytes: DefaultAttachmentMaxBytes, MaxPages: DefaultAttachmentMaxPages, Timeout: 10 * time.Second}
	if got := p.AttachmentSpec(); got != want {
		t.Fatalf("spec %+v, want %+v", got, want)
	}

	p, err = parseAttachments(t, "  extract:\n    enabled: true\n    max_bytes: 2048\n    max_pages: 5\n    timeout_ms: 1500\n  ocr:\n    enabled: true")
	if err != nil {
		t.Fatal(err)
	}
	want = AttachmentSpec{Enabled: true, Extract: true, OCR: true, MaxBytes: 2048, MaxPages: 5, Timeout: 1500 * time.Millisecond}
	if got := p.AttachmentSpec(); got != want {
		t.Fatalf("spec %+v, want %+v", got, want)
	}

	// OCR without extraction is off, and says so.
	p, err = parseAttachments(t, "  ocr:\n    enabled: true")
	if err != nil {
		t.Fatal(err)
	}
	if p.AttachmentSpec().OCR {
		t.Fatal("OCR on without extract")
	}
	if is, ok := issueFor(ValidateSemantics(p), "attachments.ocr.enabled"); !ok || is.Rule != "no_effect" {
		t.Fatalf("no warning for ocr without extract: %+v", is)
	}
}

func TestAttachments_RefusedValues(t *testing.T) {
	cases := map[string]string{
		"misspelled key":      "  unreadables: BLOCK",
		"redact":              "  unreadable: REDACT",
		"strip":               "  encrypted: STRIP",
		"route":               "  encrypted: ROUTE",
		"log":                 "  unreadable: LOG",
		"unknown action":      "  unreadable: DENY",
		"max_bytes too small": "  extract:\n    enabled: true\n    max_bytes: 10",
		"max_bytes too large": "  extract:\n    enabled: true\n    max_bytes: 67108864",
		"negative pages":      "  extract:\n    enabled: true\n    max_pages: -1",
		"too many pages":      "  extract:\n    enabled: true\n    max_pages: 5000",
		"timeout too short":   "  extract:\n    enabled: true\n    timeout_ms: 5",
		"timeout too long":    "  extract:\n    enabled: true\n    timeout_ms: 600000",
		"bounds when off":     "  extract:\n    enabled: false\n    max_bytes: 10",
	}
	for name, section := range cases {
		if _, err := parseAttachments(t, section); err == nil {
			t.Errorf("%s: loaded", name)
		}
	}
	_, err := parseAttachments(t, "  unreadable: REDACT")
	var ie *IssuesError
	if !errors.As(err, &ie) || ie.Issues[0].Field != "attachments.unreadable" || ie.Issues[0].Rule != "enum" {
		t.Fatalf("issue: %v", err)
	}
	// A rule keyed by the type cannot ask for REDACT either.
	if loadErr("version: \"1.0\"\nname: x\nrules:\n  attachment_unreadable:\n    action: REDACT\n") == "" {
		t.Error("rules.attachment_unreadable REDACT loaded")
	}
	// Case is not significant.
	if _, err := parseAttachments(t, "  unreadable: block\n  encrypted: Warn\n  extract:\n    enabled: true"); err != nil {
		t.Errorf("lower case: %v", err)
	}
}

func TestAttachments_BlockWithoutExtractIsWarnedAbout(t *testing.T) {
	p, err := parseAttachments(t, "  unreadable: BLOCK")
	if err != nil {
		t.Fatal(err)
	}
	if is, ok := issueFor(ValidateSemantics(p), "attachments.unreadable"); !ok || is.Severity != "warning" {
		t.Fatalf("no warning: %+v", is)
	}
}

func TestAttachments_SectionActionDecides(t *testing.T) {
	p, err := LoadFromBytes([]byte(`version: "1.0"
name: att
scan:
  default_action: BLOCK
attachments:
  unreadable: WARN
  encrypted: BLOCK
exceptions:
  - rule: attachment_unreadable
    roles: [admin]
    reason: test
`))
	if err != nil {
		t.Fatal(err)
	}
	enc, other := attachmentFinding(scanner.AttachmentEncrypted), attachmentFinding(scanner.AttachmentExtractFailed)
	if got := p.Evaluate([]scanner.Finding{enc}); got != ActionBlock {
		t.Errorf("encrypted: %s, want BLOCK", got)
	}
	// The section's action, and not scan.default_action.
	if got := p.Evaluate([]scanner.Finding{other}); got != ActionWarn {
		t.Errorf("extract_failed: %s, want WARN", got)
	}
	if got := p.FindingAction(other, "", false); got != ActionWarn {
		t.Errorf("FindingAction %s", got)
	}
	// An exception waives a rule, not the section's own action.
	if got, _ := p.EvaluateWithRole([]scanner.Finding{enc}, "admin", false); got != ActionBlock {
		t.Errorf("exception: %s, want BLOCK", got)
	}
	// A rule keyed by the type applies as well; the stronger wins.
	p.Rules = map[string]Rule{"attachment_unreadable": {Action: ActionBlock, Sensitivity: "low"}}
	if got := p.Evaluate([]scanner.Finding{other}); got != ActionBlock {
		t.Errorf("with a BLOCK rule: %s", got)
	}

	// Empty means PASS: recorded, nothing stopped.
	p, err = parseAttachments(t, "  extract:\n    enabled: true")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range scanner.AttachmentCategories {
		if got := p.Evaluate([]scanner.Finding{attachmentFinding(c)}); got != ActionPass {
			t.Errorf("%s: %s, want PASS", c, got)
		}
	}
}
