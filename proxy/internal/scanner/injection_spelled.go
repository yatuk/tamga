package scanner

import (
	"strings"
	"sync"

	"github.com/yatuk/tamga/internal/scanner/normalize"
)

// An instruction written one letter at a time ("i g n o r e   a l l
// p r e v i o u s", "i.g.n.o.r.e", "i-g-n-o-r-e") is read by no phrase and no
// rule as it stands. The normalize package joins the letters of such a run
// (normalize.Result.Spelled); this file turns the result into text the
// rules can read and hands it to the two scanners that look for
// instructions, as one more view next to the decoded ones.
//
// Only those two get it. The PII and secret scanners do not: a run is made
// of letters, so it holds no number to find, and a finding from a view has
// no position (placeFinding), which for a maskable type means the request
// can only be blocked, never redacted. A view that could only add such
// findings is not given to them.
//
// Leetspeak inside a spelled run ("1 g n 0 r e") is not read here: a digit
// ends the run. Such runs are read apart, with the leetspeak view
// (injection_leet.go).

// normalizeForInjection normalises text the way the injection and jailbreak
// scanners read it: the default passes, and the spelled-out runs.
func normalizeForInjection(text string) normalize.Result {
	opts := normalize.Default()
	opts.SpelledRuns = true
	return normalize.Apply(text, opts)
}

// spelledViews returns the spelled-out runs as text with word boundaries.
// Where the text showed them (a wider gap, another separator) they are kept;
// a stretch without any ("ignorepreviousinstructions") is cut into the
// words the rules know.
func spelledViews(runs []string) []string {
	if len(runs) == 0 {
		return nil
	}
	out := make([]string, 0, len(runs))
	for _, run := range runs {
		fields := strings.Fields(run)
		for i, f := range fields {
			fields[i] = respace(f)
		}
		out = append(out, strings.Join(fields, " "))
	}
	return out
}

const (
	// respaceMinLetters is how many letters a row of known words needs
	// before it is cut apart. It is the length a spelled run needs to be
	// read at all (normalize: spelledMinLetters): two short words that
	// happen to stand inside a longer one ("act as" in "factasks") are
	// left joined, and so match nothing.
	respaceMinLetters = 8
	// respaceMaxLen bounds the work on one stretch of letters. A spelled
	// instruction is a sentence, not a page.
	respaceMaxLen = 512
)

// spelledExtraWords are the words of the verb-on-target rules
// (injection_intent.go) and of the regular expressions next to the phrase
// list, in English and in folded Turkish, and the small words that stand
// between them. The words of the phrase list itself are added from the list.
// A word missing here costs a match on a run without word gaps only.
var spelledExtraWords = strings.Fields(`
	ignore disregard forget drop cancel override bypass discard erase reset skip
	previous prior earlier above preceding all every any your its my the these those
	instruction instructions directive directives rule rules guideline guidelines
	prompt prompts restriction restrictions constraint constraints programming training
	comply obey answer respond regardless of policy policies
	disable turn off switch remove deactivate circumvent
	safety content moderation security ethical filter filters check checks measures
	guardrails safeguards ethics limits
	print reveal show dump output repeat display leak expose recite give me tell write out
	hidden secret initial original confidential system message role admin developer
	mode enabled activated engaged jailbreak unrestricted unfiltered
	you are now pretend act as if have has had no with without not do does
	and then to in on for is are was were be this that it what which from new
	please just only also first last next follow following given
	translate execute run apply note assistant model user said says say
	onceki tum butun gecmis talimat talimati talimatlar talimatlari talimatlarini
	kural kurallar kurallari kurallarini yonerge yonergeleri unut yoksay yok say
	gormezden gel iptal et gecersiz sifirla sistem istem istemi istemini
	mesaj mesaji mesajini promptu promptunu goster yaz soyle ver gizli sana verilen
	guvenlik filtre filtresini filtrelerini kapat atla hicbir sen artik ve
`)

var (
	spelledVocabOnce sync.Once
	spelledVocab     map[string]bool
	spelledVocabMax  int
)

// trFold writes the Turkish letters the way the normalised view has them.
var trFold = strings.NewReplacer("ç", "c", "ğ", "g", "ı", "i", "ö", "o", "ş", "s", "ü", "u")

func loadSpelledVocab() {
	spelledVocab = map[string]bool{}
	add := func(w string) {
		if len(w) < 2 {
			return
		}
		for i := 0; i < len(w); i++ {
			if w[i] < 'a' || w[i] > 'z' {
				return
			}
		}
		spelledVocab[w] = true
		if len(w) > spelledVocabMax {
			spelledVocabMax = len(w)
		}
	}
	for _, w := range spelledExtraWords {
		add(w)
	}
	notLetter := func(r rune) bool { return r < 'a' || r > 'z' }
	for _, def := range injectionPatterns {
		for _, w := range strings.FieldsFunc(trFold.Replace(strings.ToLower(def.phrase)), notLetter) {
			add(w)
		}
	}
}

// respace cuts a stretch of joined letters into the words the rules know:
// "ignorepreviousinstructions" becomes "ignore previous instructions". It
// takes the reading with the fewest letters outside a known word, and of
// those the one with the fewest words. Known words are only cut apart where
// a row of them is at least respaceMinLetters long; everything else stays
// joined, as it was.
func respace(s string) string {
	if len(s) < respaceMinLetters || len(s) > respaceMaxLen {
		return s
	}
	for i := 0; i < len(s); i++ {
		if s[i] < 'a' || s[i] > 'z' {
			return s
		}
	}
	spelledVocabOnce.Do(loadSpelledVocab)

	// best[i] is the cheapest reading of s[:i]; from[i] is where its last
	// piece starts, and known[i] whether that piece is a word.
	type cost struct{ unknown, words int }
	n := len(s)
	best := make([]cost, n+1)
	from := make([]int, n+1)
	known := make([]bool, n+1)
	for i := 1; i <= n; i++ {
		best[i] = cost{best[i-1].unknown + 1, best[i-1].words}
		from[i] = i - 1
		for l := 2; l <= spelledVocabMax && l <= i; l++ {
			if !spelledVocab[s[i-l:i]] {
				continue
			}
			c := cost{best[i-l].unknown, best[i-l].words + 1}
			if c.unknown < best[i].unknown || (c.unknown == best[i].unknown && c.words < best[i].words) {
				best[i], from[i], known[i] = c, i-l, true
			}
		}
	}
	if best[n].unknown == n {
		return s
	}

	// The pieces, last first.
	type piece struct {
		start, end int
		known      bool
	}
	var pieces []piece
	for i := n; i > 0; i = from[i] {
		pieces = append(pieces, piece{from[i], i, known[i]})
	}

	var b strings.Builder
	b.Grow(n + len(pieces))
	for i := len(pieces) - 1; i >= 0; {
		// A row of known words, and how many letters it has.
		j, letters := i, 0
		for j >= 0 && pieces[j].known {
			letters += pieces[j].end - pieces[j].start
			j--
		}
		switch {
		case letters >= respaceMinLetters:
			for k := i; k > j; k-- {
				if b.Len() > 0 {
					b.WriteByte(' ')
				}
				b.WriteString(s[pieces[k].start:pieces[k].end])
			}
			if j >= 0 {
				b.WriteByte(' ')
			}
			i = j
		case j < i:
			// Too short to be a phrase: left as part of what surrounds it.
			b.WriteString(s[pieces[i].start:pieces[j+1].end])
			i = j
		default:
			b.WriteString(s[pieces[i].start:pieces[i].end])
			i--
		}
	}
	return strings.TrimRight(b.String(), " ")
}
