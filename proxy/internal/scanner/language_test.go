package scanner

import (
	"context"
	"strings"
	"testing"

	"github.com/yatuk/tamga/internal/extract"
	"github.com/yatuk/tamga/internal/scanner/langid"
)

// The texts below were written for these tests. Each class has texts of
// the length of a chat message; the Turkish ones are also written without
// Turkish letters, as on a keyboard that has none.
var (
	langTurkish = []string{
		"Merhaba, geçen hafta sipariş ettiğim ürün hasarlı geldi. Kutunun bir köşesi ezilmişti ve içindeki cihaz çalışmıyor. İade sürecini nasıl başlatabilirim?",
		"Şirketimizin yıllık bütçe planlamasında personel giderlerinin payı geçen yıla göre arttı. Bu artışın nedenlerini madde madde açıklar mısın?",
		"Kira sözleşmem önümüzdeki ay bitiyor. Ev sahibi kiraya yüzde elli zam yapmak istiyor. Yasal olarak uygulanabilecek en yüksek artış oranı nedir?",
		"Bu raporu yarına kadar bitirebilir misin? Toplantı saat üçte başlayacak ve müdür sonuçları görmek istiyor.",
	}
	langTurkishASCII = []string{
		"Merhaba, gecen hafta siparis ettigim urun hasarli geldi. Kutunun bir kosesi ezilmisti ve icindeki cihaz calismiyor. Iade surecini nasil baslatabilirim?",
		"Sirketimizin yillik butce planlamasinda personel giderlerinin payi gecen yila gore artti. Bu artisin nedenlerini madde madde aciklar misin?",
		"Kira sozlesmem onumuzdeki ay bitiyor. Ev sahibi kiraya yuzde elli zam yapmak istiyor. Yasal olarak uygulanabilecek en yuksek artis orani nedir?",
		"Bu raporu yarina kadar bitirebilir misin? Toplanti saat ucte baslayacak ve mudur sonuclari gormek istiyor.",
	}
	langEnglish = []string{
		"Hello, the product I ordered last week arrived damaged. One corner of the box was crushed and the device inside does not work. How can I start the return?",
		"We need to postpone the project delivery by two weeks because the test team found critical bugs. Could you draft a short email to the customer?",
	}
	langMixed = []string{
		"Deploy sırasında pipeline fail oldu, log'larda connection refused hatası var. Staging ortamında rollback yapmamız gerekiyor mu yoksa hotfix ile mi ilerleyelim?",
		"Bu sprint'te backlog'daki user story'leri önceliklendirmemiz lazım. Product owner ile bir meeting ayarlayıp acceptance criteria'ları netleştirelim.",
	}
	langGerman = []string{
		"Hallo, das Produkt, das ich letzte Woche bestellt habe, kam beschädigt an. Eine Ecke des Kartons war eingedrückt und das Gerät funktioniert nicht.",
		"Wir müssen die Projektabgabe um zwei Wochen verschieben, weil das Testteam kritische Fehler gefunden hat. Kannst du eine kurze E-Mail entwerfen?",
	}
	langSpanish = []string{
		"Hola, el producto que pedí la semana pasada llegó dañado. Una esquina de la caja estaba aplastada y el aparato no funciona. ¿Cómo inicio la devolución?",
	}
	langRussian = []string{
		"Здравствуйте, товар, который я заказал на прошлой неделе, пришёл повреждённым. Угол коробки был смят, и устройство внутри не работает.",
	}
	langArabic = []string{
		"مرحبا، المنتج الذي طلبته الأسبوع الماضي وصل تالفا. كانت إحدى زوايا الصندوق مسحوقة والجهاز لا يعمل. كيف يمكنني بدء عملية الإرجاع؟",
	}
)

func trEnSpec() LanguageSpec {
	return LanguageSpec{Enabled: true, Allow: []string{"tr", "en"}, MinLetters: DefaultLanguageMinLetters, MaxForeignShare: DefaultLanguageMaxForeignShare, Roles: []string{"user", "tool"}}
}

// languageFindings runs the language check over texts given as user
// messages.
func languageFindings(t *testing.T, spec LanguageSpec, det langid.Detector, texts ...string) []Finding {
	t.Helper()
	segs := make([]extract.Segment, len(texts))
	for i, text := range texts {
		segs[i] = extract.Segment{Role: extract.RoleUser, Text: text, Path: "messages." + string(rune('0'+i)) + ".content"}
	}
	s := NewLanguageScannerWith(func() LanguageSpec { return spec }, det)
	found, err := s.scanSegments(context.Background(), segs)
	if err != nil {
		t.Fatal(err)
	}
	return found
}

func TestLanguage_AllowedLanguagesPass(t *testing.T) {
	det := langid.Default()
	var texts []string
	texts = append(texts, langTurkish...)
	texts = append(texts, langTurkishASCII...)
	texts = append(texts, langEnglish...)
	texts = append(texts, langMixed...)
	for _, text := range texts {
		if found := languageFindings(t, trEnSpec(), det, text); len(found) != 0 {
			t.Errorf("%q: got %s, want no finding", text[:40], found[0].Match)
		}
	}
}

func TestLanguage_OtherLanguagesAreFound(t *testing.T) {
	det := langid.Default()
	cases := map[string][]string{"de": langGerman, "es": langSpanish, "ru": langRussian, "ar": langArabic}
	for want, texts := range cases {
		for _, text := range texts {
			found := languageFindings(t, trEnSpec(), det, text)
			if len(found) != 1 {
				t.Errorf("%s %q: %d findings, want 1", want, text[:30], len(found))
				continue
			}
			f := found[0]
			if f.Type != LanguageFindingType || f.Category != LanguageNotAllowed || f.Role != "user" || f.Path != "messages.0.content" {
				t.Errorf("%s: finding %+v", want, f)
			}
			if !strings.HasPrefix(f.Match, want+" ") || !strings.HasPrefix(f.Metadata["languages"], want+"=") {
				t.Errorf("%s: match %q, languages %q", want, f.Match, f.Metadata["languages"])
			}
			if f.Placed() {
				t.Errorf("%s: a verdict on a message has no span, got %d-%d", want, f.StartPos, f.EndPos)
			}
			// The finding names languages, never the text.
			if strings.Contains(f.Match, strings.Fields(text)[1]) {
				t.Errorf("%s: match %q holds the text", want, f.Match)
			}
		}
	}
}
