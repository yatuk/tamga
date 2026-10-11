package classifier

import "testing"

const (
	english  = "Could you help me plan the release notes for next week, please?"
	turkish  = "Yarınki toplantı için sunumu güncelledim, gözden geçirir misiniz?"
	russian  = "Добрый день, подскажите, пожалуйста, как пройти к вокзалу?"
	arabic   = "مرحبا، هل يمكنك مساعدتي في كتابة رسالة إلى المدير؟"
	japanese = "来週の会議の議事録を要約してください。特に予算の部分をお願いします。"
)

func scriptOfText(text string) Script {
	var c ScriptCounts
	c.Add(text)
	return c.Dominant()
}

func TestDominantScript(t *testing.T) {
	tests := map[string]Script{
		english:              ScriptLatin,
		turkish:              ScriptLatin,
		russian:              ScriptCyrillic,
		arabic:               ScriptArabic,
		japanese:             ScriptCJK,
		"ok":                 ScriptNone, // too short to say
		"1234567890 !!! ???": ScriptNone,
		"":                   ScriptNone,
	}
	for text, want := range tests {
		if got := scriptOfText(text); got != want {
			t.Errorf("%q: %v, want %v", text, got, want)
		}
	}
}

func TestScriptSwitch(t *testing.T) {
	tests := []struct {
		name   string
		before Script
		text   string
		want   bool
		to     Script
	}{
		{"same script", ScriptLatin, english, false, ScriptNone},
		{"turkish after english", ScriptLatin, turkish, false, ScriptNone},
		{"turkish and english in one message", ScriptLatin, turkish + " " + english, false, ScriptNone},
		{"russian after english", ScriptLatin, russian, true, ScriptCyrillic},
		{"arabic after russian", ScriptCyrillic, arabic, true, ScriptArabic},
		{"english after russian", ScriptCyrillic, english, false, ScriptNone},
		{"russian in a russian conversation", ScriptCyrillic, russian, false, ScriptNone},
		{"first message in russian", ScriptNone, russian, false, ScriptNone},
		{"english, then russian, in one message", ScriptNone, english + "\n" + russian, true, ScriptCyrillic},
		{"russian, then english, in one message", ScriptLatin, russian + "\n" + english, true, ScriptCyrillic},
		{"russian with an english log, russian conversation", ScriptCyrillic, russian + "\n" + english, false, ScriptNone},
		{"english with a few russian words", ScriptLatin, english + " " + english + " вокзал", false, ScriptNone},
		{"japanese with english terms throughout", ScriptCJK, "API の設定を確認して deploy を実行し、ログを確認してください。エラーが出たら rollback してください。", false, ScriptNone},
		{"too short", ScriptLatin, "да", false, ScriptNone},
	}
	for _, tt := range tests {
		to, got := ScriptSwitch(tt.before, tt.text)
		if got != tt.want || to != tt.to {
			t.Errorf("%s: (%v, %v), want (%v, %v)", tt.name, to, got, tt.to, tt.want)
		}
	}
}

// A long text is sampled, not read whole.
func TestScriptCountsSample(t *testing.T) {
	var c ScriptCounts
	long := make([]byte, 0, 3*scriptSample)
	for len(long) < 3*scriptSample {
		long = append(long, "word "...)
	}
	c.Add(string(long))
	if c[ScriptLatin] > scriptSample {
		t.Fatalf("%d letters counted from a sample of %d bytes", c[ScriptLatin], scriptSample)
	}
}
