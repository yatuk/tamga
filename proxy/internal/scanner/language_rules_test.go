package scanner

import (
	"context"
	"strings"
	"testing"

	"github.com/yatuk/tamga/internal/extract"
	"github.com/yatuk/tamga/internal/scanner/langid"
)

// fakeDetector names a piece by the first marker word it holds: "zzde" and
// so on. A piece without one cannot be told.
type fakeDetector struct{ calls int }

func (d *fakeDetector) Detect(text string) []langid.Guess {
	d.calls++
	for _, code := range []string{"tr", "en", "de", "fr"} {
		if strings.Contains(text, "zz"+code) {
			return []langid.Guess{{Code: code, Confidence: 1}}
		}
	}
	return nil
}

// sentence returns a sentence of about fifty letters marked as code.
func sentence(code string) string {
	return "zz" + code + " lorem ipsum dolor sit amet consectetur adipiscing elit sed do."
}

func TestLanguage_ShareThreshold(t *testing.T) {
	spec := trEnSpec()
	// Four Turkish sentences and one German: a fifth of the letters.
	mostly := strings.Repeat(sentence("tr")+" ", 4) + sentence("de")
	if found := languageFindings(t, spec, &fakeDetector{}, mostly); len(found) != 0 {
		t.Fatalf("a fifth in German is under 0.3: got %s", found[0].Match)
	}
	// Two and two: half.
	half := strings.Repeat(sentence("tr")+" ", 2) + strings.Repeat(sentence("de")+" ", 2)
	found := languageFindings(t, spec, &fakeDetector{}, half)
	if len(found) != 1 {
		t.Fatalf("half in German: %d findings, want 1", len(found))
	}
	if got := found[0].Match; got != "de 50%, tr 50%" {
		t.Errorf("match %q", got)
	}
	if got := found[0].Metadata["foreign_share"]; got != "0.50" {
		t.Errorf("foreign_share %q", got)
	}
	if got := found[0].Metadata["languages"]; got != "de=0.50,tr=0.50" {
		t.Errorf("languages %q", got)
	}
	// The share is configurable: at 0.1 the fifth is too much.
	spec.MaxForeignShare = 0.1
	if found := languageFindings(t, spec, &fakeDetector{}, mostly); len(found) != 1 {
		t.Errorf("max_foreign_share 0.1: %d findings, want 1", len(found))
	}
	// At 0 any foreign piece is.
	spec.MaxForeignShare = 0
	if found := languageFindings(t, spec, &fakeDetector{}, strings.Repeat(sentence("tr")+" ", 20)+sentence("de")); len(found) != 1 {
		t.Errorf("max_foreign_share 0: %d findings, want 1", len(found))
	}
}

func TestLanguage_ShortTextIsNotJudged(t *testing.T) {
	det := langid.Default()
	for _, text := range []string{"Danke schön, bis morgen!", "¿Qué tal?", "Привет, как дела?", "ok"} {
		if found := languageFindings(t, trEnSpec(), det, text); len(found) != 0 {
			t.Errorf("%q: %d letters is under min_letters, got %s", text, countLetters(text), found[0].Match)
		}
	}
	// The same German at more length is judged.
	if found := languageFindings(t, trEnSpec(), det, langGerman[0]); len(found) != 1 {
		t.Errorf("German at %d letters: %d findings", countLetters(langGerman[0]), len(found))
	}
	// min_letters is configurable.
	spec := trEnSpec()
	spec.MinLetters = 500
	if found := languageFindings(t, spec, det, langGerman[0]); len(found) != 0 {
		t.Errorf("min_letters 500: got a finding")
	}
}

func TestLanguage_TextNobodyCanTellIsNotJudged(t *testing.T) {
	text := strings.Repeat("lorem ipsum dolor sit amet. ", 10)
	if found := languageFindings(t, trEnSpec(), &fakeDetector{}, text); len(found) != 0 {
		t.Errorf("no piece was named, got %s", found[0].Match)
	}
}

func TestLanguage_ScopeAndSwitch(t *testing.T) {
	det := langid.Default()
	segs := []extract.Segment{{Role: extract.RoleSystem, Text: langGerman[0], Path: "messages.0.content"}}
	run := func(spec LanguageSpec) []Finding {
		found, err := NewLanguageScannerWith(func() LanguageSpec { return spec }, det).scanSegments(context.Background(), segs)
		if err != nil {
			t.Fatal(err)
		}
		return found
	}
	if found := run(trEnSpec()); len(found) != 0 {
		t.Errorf("system text with scope [user, tool]: %d findings", len(found))
	}
	spec := trEnSpec()
	spec.Roles = []string{"system"}
	if found := run(spec); len(found) != 1 || found[0].Role != "system" {
		t.Errorf("scope [system]: %+v", found)
	}
	spec.Enabled = false
	if found := run(spec); len(found) != 0 {
		t.Errorf("disabled: %d findings", len(found))
	}
}

func TestLanguage_RawScanFindsNothing(t *testing.T) {
	s := NewLanguageScannerWith(func() LanguageSpec { return trEnSpec() }, langid.Default())
	found, err := s.Scan(context.Background(), []byte(langGerman[0]))
	if err != nil || len(found) != 0 {
		t.Errorf("Scan: %v, %v", found, err)
	}
}

func TestLanguage_ThroughScanSegments(t *testing.T) {
	r := NewRegistry()
	r.Register(NewPIIScanner())
	r.Register(NewLanguageScannerWith(func() LanguageSpec { return trEnSpec() }, langid.Default()))
	segs := []extract.Segment{
		{Role: extract.RoleUser, Text: langTurkish[0], Path: "messages.0.content"},
		{Role: extract.RoleUser, Text: langGerman[1] + " Meine Adresse ist max@example.invalid.", Path: "messages.1.content"},
	}
	found, err := r.ScanSegments(context.Background(), segs, PipelineConfig{Mode: ModeSync})
	if err != nil {
		t.Fatal(err)
	}
	var lang []Finding
	for _, f := range found {
		if f.Type == LanguageFindingType {
			lang = append(lang, f)
		}
	}
	if len(lang) != 1 || lang[0].Seg != 1 || lang[0].Path != "messages.1.content" {
		t.Fatalf("language findings %+v", lang)
	}
	if lang[0].ScannerVersion == "" {
		t.Error("no scanner version")
	}
}

func TestLanguage_RequestBudgetTakesNewestFirst(t *testing.T) {
	det := &fakeDetector{}
	// Each text is about 6000 letters of German, sampled to at most 3000;
	// the request's budget of 6000 covers a few such texts, not ten.
	long := strings.Repeat(sentence("de")+" ", 110)
	segs := make([]extract.Segment, 10)
	for i := range segs {
		segs[i] = extract.Segment{Role: extract.RoleUser, Text: long + strings.Repeat(" x", i), Path: "messages.n.content"}
	}
	found, err := NewLanguageScannerWith(func() LanguageSpec { return trEnSpec() }, det).scanSegments(context.Background(), segs)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) < 3 || len(found) >= len(segs) {
		t.Fatalf("%d findings for %d texts over the budget", len(found), len(segs))
	}
	for _, f := range found {
		if f.Seg < len(segs)-len(found) {
			t.Errorf("finding in text %d: the newest are judged first", f.Seg)
		}
	}
}

func TestLanguage_LongTextIsCached(t *testing.T) {
	ResetScanCache()
	det := &fakeDetector{}
	long := strings.Repeat(sentence("de")+" ", 50)
	first := languageFindings(t, trEnSpec(), det, long)
	calls := det.calls
	second := languageFindings(t, trEnSpec(), det, long)
	if len(first) != 1 || len(second) != 1 {
		t.Fatalf("findings %d and %d", len(first), len(second))
	}
	if det.calls != calls {
		t.Errorf("the second request asked the detector again (%d calls, then %d)", calls, det.calls)
	}
	// Another allow list is another key.
	spec := trEnSpec()
	spec.Allow = []string{"de"}
	if found := languageFindings(t, spec, det, long); len(found) != 0 {
		t.Errorf("German allowed: got a finding from the cache")
	}
}

func TestLanguageProse(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"fenced code", "Şu kodu incele:\n```go\nfunc main() {\n\tfmt.Println(\"merhaba\")\n}\n```\nNe yapıyor?", "Şu kodu incele:\nNe yapıyor?"},
		{"fenced code with comments", "Bak:\n```python\n# Benutzer laden und prüfen\nuser = load(id)\nif user is None:\n    raise KeyError(id)\n```", "Bak:"},
		{"fenced prose", "```\nDas ist kein Code, sondern ein ganz normaler Satz.\n```", "Das ist kein Code, sondern ein ganz normaler Satz."},
		{"indented code", "Hata şurada:\n\n    for i in range(10):\n        total += values[i]\n\nNeden?", "Hata şurada:\nNeden?"},
		{"java trace", "Exception in thread \"main\" java.lang.NullPointerException: name is null\n\tat com.example.App.run(App.java:42)\n\tat com.example.App.main(App.java:10)\nBu hatayı nasıl düzeltirim?", "Bu hatayı nasıl düzeltirim?"},
		{"python trace", "Traceback (most recent call last):\n  File \"app.py\", line 3, in <module>\n    main()\nValueError: invalid literal for int()\nNeden oluyor?", "Neden oluyor?"},
		{"go trace", "panic: runtime error: index out of range\n\ngoroutine 1 [running]:\nmain.main()\n\t/home/x/main.go:8 +0x1d\nNe demek bu?", "Ne demek bu?"},
		{"log lines", "2026-10-17T10:00:00Z INFO server started on port 8080\n[ERROR] connection refused by upstream\nlevel=warn msg=\"slow request\" took=3s\nLoglara göre sorun ne?", "Loglara göre sorun ne?"},
		{"tokens", "Dosya /var/log/app.log ve C:\\temp\\x.txt içinde, ayrıca https://example.invalid/a?b=1 ile www.example.invalid adresinde, yaz dev@example.invalid user_id getUserName config.yaml v1.2.3 2026 ve 42.", "Dosya ve içinde, ayrıca ile adresinde, yaz ve"},
		{"inline code", "`SELECT * FROM users` sorgusu yavaş.", "sorgusu yavaş."},
		{"apostrophes stay", "Türkiye'nin başkenti Ankara'dır, e-posta adresini gönder.", "Türkiye'nin başkenti Ankara'dır, e-posta adresini gönder."},
		{"invisible characters", "Pass\u200bwort ver\u200bgessen", "Passwort vergessen"},
	}
	for _, c := range cases {
		got := LanguageProse(c.in)
		if got != c.want {
			t.Errorf("%s:\n got %q\nwant %q", c.name, got, c.want)
		}
	}
}

func TestLanguage_TraceInTurkishPasses(t *testing.T) {
	det := langid.Default()
	text := langTurkish[0] + "\n\nAldığım hata şu:\n\n" +
		"Exception in thread \"main\" java.lang.IllegalStateException: Could not open the connection to the database server\n" +
		"\tat com.example.db.Pool.open(Pool.java:118)\n\tat com.example.db.Pool.get(Pool.java:64)\n" +
		"\tat com.example.service.OrderService.load(OrderService.java:212)\n" +
		"Caused by: java.net.ConnectException: Connection refused: the remote host did not answer in time\n" +
		"\t... 12 more\n"
	spec := trEnSpec()
	spec.Allow = []string{"tr"}
	if found := languageFindings(t, spec, det, text); len(found) != 0 {
		t.Errorf("an English trace in a Turkish message, only tr allowed: got %s", found[0].Match)
	}
	// English prose next to it counts.
	english := langTurkish[0] + "\n" + langEnglish[0] + " " + langEnglish[1]
	if found := languageFindings(t, spec, det, english); len(found) != 1 {
		t.Errorf("English prose next to Turkish, only tr allowed: %d findings, want 1", len(found))
	}
}

func TestLanguage_GermanInAFenceIsStillGerman(t *testing.T) {
	det := langid.Default()
	text := "```\n" + langGerman[0] + "\n" + langGerman[1] + "\n```"
	if found := languageFindings(t, trEnSpec(), det, text); len(found) != 1 {
		t.Errorf("German prose in a fence: %d findings, want 1", len(found))
	}
	// German split by zero-width spaces is still German.
	split := strings.ReplaceAll(langGerman[0], "e", "e\u200b")
	if found := languageFindings(t, trEnSpec(), det, split); len(found) != 1 {
		t.Errorf("German with zero-width spaces: %d findings, want 1", len(found))
	}
}

func TestLanguage_CodeWithGermanCommentsPasses(t *testing.T) {
	det := langid.Default()
	text := "Bu fonksiyon neden yavaş çalışıyor, nasıl hızlandırabilirim?\n```js\n" +
		"// Lädt alle Bestellungen des Kunden und berechnet die Summe\n" +
		"async function summe(kundeId) {\n" +
		"  const bestellungen = await db.query('SELECT * FROM bestellungen WHERE kunde = ?', [kundeId]);\n" +
		"  // Für jede Bestellung den Preis addieren, ohne Rabatt\n" +
		"  return bestellungen.reduce((s, b) => s + b.preis, 0);\n" +
		"}\n```"
	if found := languageFindings(t, trEnSpec(), det, text); len(found) != 0 {
		t.Errorf("German comments in a code block: got %s", found[0].Match)
	}
}
