package linkscan

import (
	"strings"
	"testing"
)

func TestFoldLatin(t *testing.T) {
	for in, want := range map[string]string{
		"github.com":     "github.com",
		"gıthub.com":     "github.com",
		"GİTHUB.com":     "github.com",
		"gi̇thub.com":    "github.com", // i and a combining dot: what a browser makes of İ
		"paypał.com":     "paypal.com",
		"gøøgle.com":     "google.com",
		"đocker.example": "docker.example",
		"mícrosoft.com":  "microsoft.com",
		"ÀÉÎÕÜ.example":  "aeiou.example",
		"çağrı.example":  "cagri.example",
		"şekerbank":      "sekerbank",
		"аpple.com":      "аpple.com", // Cyrillic а is not Latin and is not folded
	} {
		if got := foldLatin(in); got != want {
			t.Errorf("foldLatin(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestImitated_BuiltinNames(t *testing.T) {
	var none *Lists
	for host, want := range map[string]string{
		// The protected hosts themselves, and what is under them.
		"github.com":                "",
		"GitHub.COM":                "",
		"docs.github.com":           "",
		"türkçe.github.com":         "", // a host under github.com, whatever its label
		"xn--trke-2oa7j.github.com": "",
		// Look-alikes.
		"gıthub.com":          "github.com",
		"www.gıthub.com":      "github.com",
		"gİthub.com":          "github.com",
		"GİTHUB.COM":          "github.com",
		"gíthub.com":          "github.com",
		"xn--gthub-n4a.com":   "github.com",
		"XN--GTHUB-N4A.com":   "github.com",
		"paypał.com":          "paypal.com",
		"xn--paypa-o7a.com":   "paypal.com",
		"accounts.gøøgle.com": "google.com",
		"akbánk.com":          "akbank.com",
		"zıraatbank.com.tr":   "ziraatbank.com.tr",
		"garantıbbva.com.tr":  "garantibbva.com.tr",
		"pypı.org":            "pypi.org",
		// Not a protected host: another ending, another name, a longer name.
		"gıthub.example":         "",
		"gıthub.com.example.net": "",
		"gıthubb.com":            "",
		"mygıthub.com":           "",
		"ışık.com.tr":            "",
		"türkiye.gov.tr":         "",
		"işbank.com.tr":          "",
		"yapıkredi.com.tr":       "",
		"münchen.de":             "",
		"café.fr":                "",
		"äpple.se":               "",
		"example.com":            "",
		"":                       "",
	} {
		if got := none.imitated(host); got != want {
			t.Errorf("imitated(%q) = %q, want %q", host, got, want)
		}
	}
}

// The operator's names are added to the built-in ones, and a protected name
// that is itself written with Turkish letters is imitated by the plain one.
func TestImitated_OperatorNames(t *testing.T) {
	lists, err := NewLists(nil, []string{"sekerbank-eski.example.com.tr"})
	if err != nil {
		t.Fatal(err)
	}
	if err := lists.Protect([]string{"şekerbank.example.com.tr", "Portal.Example.ORG.", "xn--ekerbank-mwb.example.net"}); err != nil {
		t.Fatal(err)
	}
	for host, want := range map[string]string{
		"şekerbank.example.com.tr":        "",
		"xn--ekerbank-mwb.example.com.tr": "", // the same host in punycode
		"ŞEKERBANK.example.com.tr":        "",
		"giriş.şekerbank.example.com.tr":  "",
		"sekerbank.example.com.tr":        "şekerbank.example.com.tr",
		"www.sekerbank.example.com.tr":    "şekerbank.example.com.tr",
		"sékerbank.example.com.tr":        "şekerbank.example.com.tr",
		"sekerbank.example.net":           "şekerbank.example.net",
		"portal.example.org":              "",
		"pörtal.example.org":              "portal.example.org",
		"portał.example.org":              "portal.example.org",
		"gıthub.com":                      "github.com", // the built-in names stay
		"sekerbank.example.org":           "",
	} {
		if got := lists.imitated(host); got != want {
			t.Errorf("imitated(%q) = %q, want %q", host, got, want)
		}
	}

	// Both spellings protected: neither is the look-alike of the other.
	both, _ := NewLists(nil, nil)
	if err := both.Protect([]string{"şekerbank.example.com.tr", "sekerbank.example.com.tr"}); err != nil {
		t.Fatal(err)
	}
	for _, host := range []string{"şekerbank.example.com.tr", "sekerbank.example.com.tr", "www.sekerbank.example.com.tr"} {
		if got := both.imitated(host); got != "" {
			t.Errorf("both spellings protected: imitated(%q) = %q", host, got)
		}
	}
	if got := both.imitated("sékerbank.example.com.tr"); got == "" {
		t.Error("a third spelling is not reported")
	}
}

func TestScan_ProtectedLookalike(t *testing.T) {
	hits := Scan("Clone https://gıthub.com/acme/x.git now", nil)
	if len(hits) != 1 || hits[0].Category != Homoglyph || hits[0].Imitates != "github.com" || hits[0].Host != "gıthub.com" {
		t.Fatalf("hits = %+v", hits)
	}
	// One hit for a host that is a look-alike on both counts.
	if hits := Scan("https://gіthub.com/x", nil); len(hits) != 1 || hits[0].Imitates != "" { // Cyrillic і
		t.Fatalf("mixed-script host: %+v", hits)
	}
	// Not rendered as a link inside code, like every form check.
	if hits := Scan("Run `curl https://gıthub.com/x`", nil); len(hits) != 0 {
		t.Fatalf("in code: %+v", hits)
	}
	// The allow list vouches for a host, this one too.
	lists, _ := NewLists(nil, []string{"gıthub.com"})
	if hits := Scan("https://gıthub.com/x", lists); len(hits) != 0 {
		t.Fatalf("allowed host: %+v", hits)
	}
	// The real hosts.
	for _, text := range []string{"https://github.com/golang/go", "https://GITHUB.com/golang/go", "[pypi](https://pypi.org/project/requests/)"} {
		if hits := Scan(text, nil); len(hits) != 0 {
			t.Errorf("%q: %+v", text, hits)
		}
	}
}

func TestCheckProtectedHost(t *testing.T) {
	for _, ok := range []string{"example.com", "bank.example.com.tr", "şekerbank.example.com.tr", "Example.COM."} {
		if err := CheckProtectedHost(ok); err != nil {
			t.Errorf("%q refused: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "*.example.com", "intranet", "203.0.113.5", "[2001:db8::1]", "https://example.com", "example.com/x"} {
		if err := CheckProtectedHost(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	if len(BuiltinProtectedHosts()) > 20 {
		t.Error("the built-in list is meant to be short")
	}
	for _, h := range BuiltinProtectedHosts() {
		if !isASCII(h) || h != strings.ToLower(h) {
			t.Errorf("built-in name %q is not plain lower-case ASCII", h)
		}
	}
}

// The measurement for the look-alike check alone: the benign set under the
// built-in names and under a policy that protects more.
func TestCorpus_ProtectedNamesReportNoBenignAnswer(t *testing.T) {
	lists, _ := NewLists(nil, nil)
	if err := lists.Protect([]string{"isbank.com.tr", "yapikredi.com.tr", "vakifbank.com.tr", "turkiye.gov.tr", "example.com.tr"}); err != nil {
		t.Fatal(err)
	}
	idn := 0
	for i, a := range benignAnswers {
		if !isASCII(a) || strings.Contains(a, "xn--") {
			idn++
		}
		for _, l := range []*Lists{nil, lists} {
			for _, h := range Scan(a, l) {
				t.Errorf("benign answer %d: %s (imitates %q) for %q", i, h.Category, h.Imitates, a[h.Start:h.End])
			}
		}
	}
	caught, want := 0, 0
	for _, m := range maliciousAnswers {
		for _, h := range Scan(m.Text, nil) {
			if h.Imitates != "" {
				caught++
			}
		}
		if m.Category == Homoglyph && isLatinOnly(m.Text) {
			want++
		}
	}
	t.Logf("%d benign answers (%d with a letter outside ASCII or a punycode label): 0 reported; %d of %d Latin-only look-alikes reported", len(benignAnswers), idn, caught, want)
	if caught != want || want < 5 {
		t.Errorf("Latin-only look-alikes: %d of %d reported", caught, want)
	}
}

// isLatinOnly reports whether no letter of the text is Cyrillic or Greek.
func isLatinOnly(s string) bool {
	for _, r := range s {
		if (r >= 0x0370 && r <= 0x03FF) || (r >= 0x0400 && r <= 0x052F) {
			return false
		}
	}
	return !strings.Contains(s, "xn--pypal") // punycode of a Cyrillic letter
}
