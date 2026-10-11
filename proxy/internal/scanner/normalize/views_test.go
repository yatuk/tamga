package normalize

import (
	"encoding/csv"
	"os"
	"sort"
	"strings"
	"testing"
)

var viewSamples = []string{
	"",
	"plain ASCII text with 10000000146 in it",
	"MiXeD Case And İstanbul IŞIK ığdır ÇÖĞÜŞ",
	"fullwidth １０００００００１４６ and ａｙｓｅ＠ｅｘａｍｐｌｅ．ｃｏｍ",
	"zero​width 1​0​0​0​0​0​0​0​1​4​6 and soft­hyphen",
	"entities &#49;&#48;&#48; &amp; &lt;b&gt; &semi; &ampx &notin; &not &#x41;&#65 &unknown; & alone &",
	"math \U0001D7CF\U0001D7CE\U0001D7CE bold and Cyrillic іgnоrе аll",
	"combining é ạ̈ q̃ and precomposed é ü ñ ǆ ﬁ ½ ①",
	"numbers bir sıfır sıfır sıfır sıfır sıfır sıfır sıfır bir dört altı and one two three",
	"tel +dokuz sıfır beş üç iki bir iki üç dört beş altı yedi",
	"leet 1gn0r3 4ll pr3v10us 1nstruct10ns",
	"i g n o r e   a l l   p r e v i o u s",
	"base64 MTAwMDAwMDAxNDY= hex 3130303030303030313436 rot13 nlfr@rknzcyr.pbz",
	"AKIA IOSF ODNN 7EXA MPLE and ghp_ abcd",
	"note\U000E0031\U000E0030\U000E0030\U000E0030 between \U000E0031\U000E0034\U000E0036\U000E0036 tags",
	"line separator nbsp\vtab\x00nul\x7fdel",
	"한글 조합 가 and ᄀ́ᅡ",
	"bad utf8 \xff\xfe İ\xc3 end",
	"emoji 👩‍💻 flag 🏴\U000E0067\U000E0062\U000E0073\U000E0063\U000E0074\U000E007F ok",
	"Ⅻ ㎏ ﷺ ｶﾞ",
}

func sortedCopy(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

func checkViewsMatchApply(t *testing.T, s string, opts Options) {
	t.Helper()
	want := Apply(s, opts)
	got := ApplyViews(s, opts)
	if got.Canonical.Text != want.Canonical {
		t.Errorf("%q\n canonical view = %q\n Apply          = %q", s, got.Canonical.Text, want.Canonical)
	}
	all := append([]string(nil), got.Decoded...)
	if got.Rot13.Text != "" {
		all = append(all, got.Rot13.Text)
	}
	a, b := sortedCopy(all), sortedCopy(want.Decoded)
	if strings.Join(a, "\x00") != strings.Join(b, "\x00") {
		t.Errorf("%q\n decoded views = %q\n Apply         = %q", s, a, b)
	}
}

// The views are the readings Apply makes; only the offsets are new.
func TestApplyViews_SameTextAsApply(t *testing.T) {
	plain := Default()
	plain.Deleet = false
	for _, s := range viewSamples {
		checkViewsMatchApply(t, s, Default())
		checkViewsMatchApply(t, s, plain)
	}
}

func TestApplyViews_SameTextAsApply_Corpus(t *testing.T) {
	n := 0
	for _, name := range []string{"prompts.csv", "holdout.csv"} {
		f, err := os.Open("../../../testdata/redteam/" + name)
		if err != nil {
			t.Skipf("corpus not found: %v", err)
		}
		r := csv.NewReader(f)
		r.FieldsPerRecord = -1
		r.LazyQuotes = true
		rows, err := r.ReadAll()
		_ = f.Close()
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range rows {
			for _, cell := range row {
				if len(cell) > 12 {
					checkViewsMatchApply(t, cell, Default())
					n++
				}
			}
		}
	}
	if n < 100 {
		t.Fatalf("only %d corpus texts were read", n)
	}
}

// A match in a view is put back on the bytes it was read from.
func TestApplyViews_SpanIsTheSource(t *testing.T) {
	tests := []struct {
		name, text, find, source string
	}{
		{"as written", "id 10000000146 here", "10000000146", "10000000146"},
		{"fullwidth", "id １０００００００１４６ here", "10000000146", "１０００００００１４６"},
		{"zero width", "id 1​0​0​0​0​0​0​0​1​4​6 here", "10000000146", "1​0​0​0​0​0​0​0​1​4​6"},
		{"entities", "id &#49;&#48;&#48;&#48;&#48;&#48;&#48;&#48;&#49;&#52;&#54; here", "10000000146", "&#49;&#48;&#48;&#48;&#48;&#48;&#48;&#48;&#49;&#52;&#54;"},
		{"math bold", "id \U0001D7CF\U0001D7CE\U0001D7CE here", "100", "\U0001D7CF\U0001D7CE\U0001D7CE"},
		{"number words", "id bir sıfır sıfır dört here", "1004", "bir sıfır sıfır dört"},
		{"look-alikes", "write to аysе@example.com now", "ayse@example.com", "аysе@example.com"},
		{"upper case with a dotted İ", "ŞİFRE İSTANBUL x", "istanbul", "İSTANBUL"},
		{"entity then fullwidth", "a &amp; １２ b", "& 12", "&amp; １２"},
		{"combining mark", "café 99", "cafe 99", "café 99"},
	}
	opts := Default()
	opts.Deleet = false
	for _, tt := range tests {
		v := ApplyViews(tt.text, opts).Canonical
		at := strings.Index(v.Text, tt.find)
		if at < 0 {
			t.Errorf("%s: %q not in the view %q", tt.name, tt.find, v.Text)
			continue
		}
		from, to, ok := v.Span(at, at+len(tt.find))
		if !ok {
			t.Errorf("%s: the view has no offsets", tt.name)
			continue
		}
		if got := tt.text[from:to]; got != tt.source {
			t.Errorf("%s: span = %q, want %q", tt.name, got, tt.source)
		}
	}
}

func TestViews_DigitReadings(t *testing.T) {
	text := "no 1.0-0 0.0.0.0.0.1.4.6 and 64100000001 end"
	v := StripDigitSeparators(SourceView(text))
	if want := stripSeparatorsBetweenDigits(text); v.Text != want {
		t.Fatalf("stripped = %q, want %q", v.Text, want)
	}
	at := strings.Index(v.Text, "10000000146")
	from, to, ok := v.Span(at, at+11)
	if !ok || text[from:to] != "1.0-0 0.0.0.0.0.1.4.6" {
		t.Fatalf("span = %q", text[from:to])
	}
	r := ReverseDigitRuns(v)
	if want := reverseDigitSequences(v.Text); r.Text != want {
		t.Fatalf("reversed = %q, want %q", r.Text, want)
	}
	at = strings.LastIndex(r.Text, "10000000146")
	from, to, ok = r.Span(at, at+11)
	if !ok || text[from:to] != "64100000001" {
		t.Fatalf("reversed span = %q", text[from:to])
	}
	// A match on part of a reversed run is the whole run: its digits are
	// not where they were.
	from, to, _ = r.Span(at+2, at+5)
	if text[from:to] != "64100000001" {
		t.Fatalf("partial reversed span = %q", text[from:to])
	}
}

// A decoded reading is another text: nothing in it has a place in the source.
func TestViews_DecodedHasNoSpan(t *testing.T) {
	if _, _, ok := (View{Text: "10000000146"}).Span(0, 11); ok {
		t.Fatal("a view without offsets gave a span")
	}
	v := ApplyViews("payload MTAwMDAwMDAxNDY= end", Default())
	found := false
	for _, d := range v.Decoded {
		if strings.Contains(d, "10000000146") {
			found = true
		}
	}
	if !found {
		t.Fatalf("the base64 payload was not decoded: %q", v.Decoded)
	}
	if strings.Contains(v.Canonical.Text, "10000000146") {
		t.Fatal("the decoded value is in the located view")
	}
}

// Two values hidden in tag characters in two places are two values.
func TestHiddenViews_SeparateRuns(t *testing.T) {
	tag := func(s string) string {
		var b strings.Builder
		for _, r := range s {
			b.WriteRune(0xE0000 + r)
		}
		return b.String()
	}
	text := "first" + tag("10000000146") + " and second" + tag("10000000146") + " done"
	views := hiddenViews(text)
	found := false
	for _, v := range views {
		if strings.Contains(v, "10000000146 10000000146") {
			found = true
		}
	}
	if !found {
		t.Fatalf("the two runs are not read apart: %q", views)
	}
	if h := FindHidden("x" + tag("hello world")); h.TagRuns != "" {
		t.Fatalf("one run was cut: %q", h.TagRuns)
	}
}

func FuzzApplyViews(f *testing.F) {
	for _, s := range viewSamples {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		opts := Default()
		v := ApplyViews(s, opts)
		if want := Apply(s, opts).Canonical; v.Canonical.Text != want {
			t.Fatalf("%q: canonical view %q, Apply %q", s, v.Canonical.Text, want)
		}
		for _, view := range []View{v.Folded, v.Canonical, v.Rot13} {
			if !view.Located() {
				continue
			}
			if view.start != nil && (len(view.start) != len(view.Text) || len(view.end) != len(view.Text)) {
				t.Fatalf("%q: offset tables of %d and %d for a text of %d", s, len(view.start), len(view.end), len(view.Text))
			}
			prev := 0
			for i := 0; i < len(view.Text); i++ {
				from, to, ok := view.Span(i, i+1)
				if !ok || from < 0 || to > len(s) || from >= to {
					t.Fatalf("%q: byte %d of %q has span [%d,%d) ok=%v", s, i, view.Text, from, to, ok)
				}
				if from < prev {
					t.Fatalf("%q: spans go backwards at byte %d of %q", s, i, view.Text)
				}
				prev = from
			}
		}
	})
}

// ApplyViews next to Apply on the same text: what keeping the offsets costs.
func BenchmarkApplyViews(b *testing.B) {
	prose := strings.Repeat("The quick brown fox jumps over the lazy dog, and I have a plan for a day at a farm. ", 60)
	turkish := strings.Repeat("Merhaba, geçen ayın satış raporunu özetler misin? Müşteri sayısı arttı ve gelir yükseldi. ", 60)
	for _, bm := range []struct{ name, text string }{{"prose_5KB", prose}, {"turkish_5KB", turkish}} {
		opts := Default()
		b.Run("apply/"+bm.name, func(b *testing.B) {
			b.SetBytes(int64(len(bm.text)))
			b.ReportAllocs()
			for b.Loop() {
				_ = Apply(bm.text, opts)
			}
		})
		b.Run("views/"+bm.name, func(b *testing.B) {
			b.SetBytes(int64(len(bm.text)))
			b.ReportAllocs()
			for b.Loop() {
				_ = ApplyViews(bm.text, opts)
			}
		})
	}
}

// The test in front of the number-word pass lets through every text the
// pass would rewrite: skipping it must change nothing.
func TestMayHoldNumberRun(t *testing.T) {
	expand := expandReference
	samples := append([]string{
		"one two", "ONE  Two", "one, two", "one-two", "on the one hand", "bir iki üç", "BİR İKİ", "sıfır\tbir",
		"one\ntwo", "one\u00a0two", "one1 two", "one two2", "x one two y", "twenty one", "on on", "one", "", "onetwo",
		"yüz kırk beş", "seventeen eighteen", "one \xff two", "one\u200btwo",
	}, viewSamples...)
	for _, s := range samples {
		changed := expand(s) != s
		if changed && !mayHoldNumberRun(s) {
			t.Errorf("%q is rewritten by the pass and the test in front of it says no", s)
		}
		if got := expandWordsToNumbers(s); got != expand(s) {
			t.Errorf("expandWordsToNumbers(%q) = %q, want %q", s, got, expand(s))
		}
	}
	if mayHoldNumberRun("The quick brown fox is on the roof, one of them at least.") {
		t.Error("prose without two number words in a row is taken for a number run")
	}
}

// expandReference is the number-word pass without the test in front of it.
func expandReference(s string) string {
	tokens := tokenize(s)
	if len(tokens) < 2 {
		return s
	}
	var out strings.Builder
	for i := 0; i < len(tokens); {
		run := 0
		if isNumberWord(tokens[i]) {
			run = collectNumberRun(tokens, i)
		}
		if run > 1 {
			if val, ok := parseNumberRun(tokens[i : i+run]); ok {
				out.WriteString(val)
				i += run
				continue
			}
		}
		out.WriteString(tokens[i])
		i++
	}
	return out.String()
}

func FuzzExpandWordsToNumbers(f *testing.F) {
	for _, s := range viewSamples {
		f.Add(s)
	}
	f.Add("one two three")
	f.Add("on bir iki")
	f.Fuzz(func(t *testing.T, s string) {
		if got, want := expandWordsToNumbers(s), expandReference(s); got != want {
			t.Fatalf("expandWordsToNumbers(%q) = %q, want %q", s, got, want)
		}
	})
}
