# Prompt injection and jailbreaks in Turkish: what is public

Read on 2026-10-11 for sprint item S6-2. This page records what was found,
where, and under which licence as the source states it. Nothing listed here
was downloaded: no dataset, no model. Pages were read through a fetch tool
that returns a summary of the page, so a figure below is what that summary
reported from the page; the [could not verify](#could-not-verify) list says
which claims rest on a search snippet only or could not be opened.

No row of any dataset named here is in this repository. The Turkish set
under `proxy/testdata/redteam/` (`tr_tuning.csv`, `tr_locked.csv`) was
written for Tamga; this page is where its categories come from.

## Summary

- **Nobody has published an attack success rate for prompt injection in
  Turkish against a deployed application.** What exists is jailbreak work
  on harmful-content benchmarks, where Turkish is one language among ten in
  one paper, and small Turkish datasets from 2024 to 2026 without a paper.
- **The low-resource finding does not transfer to Turkish as is.** The
  paper that showed translation raising jailbreak success did not test
  Turkish, and the gain it measured was concentrated in low-resource
  languages. The one paper found that does test Turkish treats it as
  medium-resource and reports unsafe-response rates that depend more on the
  model and the attack than on the language.
- **Guard models mostly do not say what they do in Turkish.** Of the four
  model cards read, one lists Turkish among its training languages and none
  reports a Turkish metric. A small community dataset reports miss rates of
  four guards on 248 Turkish attacks, between 0% and 85.5%.
- **The Turkish-specific evasions people name are few and concrete**:
  dotted and dotless I under an ASCII `lower()`, diacritics dropped,
  suffix chains that defeat keyword lists, Turkish-English mixing, Turkish
  leetspeak, spaced or split words. All of them are about text matching,
  which is what the deterministic scanners do; that is where S6-2 can act
  without a model.

## Papers

### Low-resource languages and translated attacks

| Paper | What it measured | Turkish |
|---|---|---|
| Yong, Menghini, Bach, "Low-Resource Languages Jailbreak GPT-4", arXiv:2310.02446 (v1 2023-10-03, v2 2024-01-27). <https://arxiv.org/abs/2310.02446>, table read at <https://arxiv.org/html/2310.02446v1> | AdvBench prompts translated into 12 languages, sent to GPT-4. Share of answers that bypassed the safety filter: English 0.96%; the four low-resource languages combined 79.04% (Zulu 53.08, Scots Gaelic 43.08, Hmong 28.85, Guarani 15.96); the four mid-resource languages combined 21.92% (Ukrainian 2.31, Bengali 13.27, Thai 10.38, Hebrew 7.12); the four high-resource languages combined 10.96% (Mandarin 2.69, Arabic 3.65, Italian 0.58, Hindi 6.54) | **Not tested.** The twelve languages are the ones listed |
| Atil, Passonneau, Morstatter, "Do Methods to Jailbreak and Defend LLMs Generalize Across Languages?", arXiv:2511.00689 (v1 2025-11-01, v2 2025-11-04). <https://arxiv.org/abs/2511.00689v2>, table read at <https://arxiv.org/html/2511.00689v2>. Paper licence: CC BY 4.0 | Ten languages, six models, HarmBench (a 200-query subset) and AdvBench (520 strings), two jailbreak methods and defences | **Tested, as a medium-resource language** (with Arabic and Korean). Unsafe-response rate on HarmBench in Turkish, standard query / logic jailbreak / the Andriushchenko et al. jailbreak: GPT-4o 6.0 / 54.0 / 5.0; Sonnet 4 5.0 / 16.0 / 44.0; Qwen2.5-7b 26.5 / 58.0 / 91.0; Qwen2.5-14b 12.5 / 53.0 / 94.5; Llama3.1-8b 46.5 / 63.0 / 90.5; Llama3.1-70b 28.0 / 59.5 / 92.0. The authors note that the low GPT-4o figure in the third column comes partly from code-switching in the answers |
| Deng et al., "Multilingual Jailbreak Challenges in Large Language Models", ICLR 2024, arXiv:2310.06474. <https://arxiv.org/abs/2310.06474> | MultiJail: harmful queries translated by native speakers into nine languages (Chinese, Italian, Vietnamese, Arabic, Korean, Thai, Bengali, Swahili, Javanese) | Not in the nine (from a search summary; the paper was not opened) |
| Wang et al., "All Languages Matter: On the Multilingual Safety of Large Language Models", arXiv:2310.00905. <https://arxiv.org/abs/2310.00905> | XSafety: ten languages (English, Chinese, Hindi, Spanish, French, Arabic, Bengali, Russian, Japanese, German) | Not in the ten (from a search summary; the paper was not opened) |
| Ifebi, Kong, Mehrotra, "Minionese: Comprehensive Benchmark and Mechanistic Study of Multilingual LLM Safety", arXiv:2607.10112 (2026-07-11). <https://arxiv.org/abs/2607.10112> | 18 languages in four resource tiers; translation, code-switching, transliteration and "translationese" as perturbations. The abstract reports that code-switching stays effective down to the lowest tier | See [could not verify](#could-not-verify) |

What these say for Tamga:

- They measure whether a model **answers a harmful request**. Tamga's
  injection scanners look for something else: an instruction that tries to
  replace the application's instructions. A translated harmful request has
  no such instruction in it and is a matter for content moderation, not for
  these rules.
- "Translate the attack into another language" is still the cheapest way
  past a guard that reads one language. For a proxy deployed in Turkey the
  relevant direction is both: an English attack in Turkish traffic (the
  English rules cover it) and a Turkish attack past rules written for
  English.
- Code-switching is reported as robust across tiers. In Turkish it has a
  specific form, an English verb with "et-" ("ignore et", "bypass et") and
  an English noun with a Turkish ending ("instructions'ları").

### Turkish-specific academic work

None was found on arXiv. One journal article that appears to compare Turkish
open-source, localized and commercial models under adversarial prompts could
not be opened; see [could not verify](#could-not-verify).

## Datasets that hold Turkish injection text

Licence and size are as the dataset card states. Both were downloaded on
2026-10-17 with the owner's approval; the first is in the repository, the
second is not.

| Dataset | Host | Size | Licence | What it is |
|---|---|---|---|---|
| `beratcmn/turkish-prompt-injections` | <https://huggingface.co/datasets/beratcmn/turkish-prompt-injections> | 661 rows (train 546, test 115) | Apache-2.0 | A Turkish translation of `deepset/prompt-injections`; columns `text`, `label`, `original` (the English source). The card does not say how it was translated or define the labels. In the repository, corrected, as `proxy/testdata/redteam/trd_tuning.csv` and `trd_locked.csv` |
| `OnerAYTAS/Turkish_prompt_injection_jailbreak_dataset` | <https://huggingface.co/datasets/OnerAYTAS/Turkish_prompt_injection_jailbreak_dataset> | About 20,500 rows in eight files | CC BY-NC-SA 4.0 | Yıldız Technical University; the set of Aytaş et al., *Applied Sciences* 16(13), 6740 (2026), <https://doi.org/10.3390/app16136740>. Machine translations (translategemma, Google Translate, DeepL) of safe-guard-prompt-injection, Yash0728/toxicity_prompt-injection, jackhhao/jailbreak-classification, rogue-security/prompt-injections-benchmark and Injection-Attack-Detection-Dataset, each with its source's labels, plus 790 harmful requests. Non-commercial: measured on a local copy only (`docs/benchmarks/README.md`) |

Neither is used as training data for the fine-tune in this
round. `classifier/finetune/README.md` says what would have to be checked
before one is: the licence on the day of download, where the rows come from
(a translation of `deepset/prompt-injections` carries that set's licence
and its German and English origin), and that no row is also in a set the
current model was trained or evaluated on.

Two things matter for S6-2:

- Both sets are machine translations of English sets, so they carry those
  sets' phrasing, and the larger one may not be used commercially.
- A useful Turkish set pairs attacks with benign texts that share their
  vocabulary, and the Tamga set is written that way: the benign half is as
  important as the attack half, because in Turkish the trigger words
  ("talimat", "kural", "yok say") are everyday words in banking, law and
  teaching.

## Guard models and Turkish

| Model | Licence | Size | What the card says about Turkish |
|---|---|---|---|
| `Horizon-Labs/prompt-injection-guard-small`, the model Tamga's classifier numbers were measured with. Card read from the copy an operator placed in `models/` on 2026-10-03 (revision `3215a27`); <https://huggingface.co/Horizon-Labs/prompt-injection-guard-small> | Apache-2.0; backbone `jhu-clsp/mmBERT-small`, MIT | 141M parameters; 268 MB as the int8 ONNX export | Lists `tr` among 19 named languages. Training data includes about 80k synthetic examples "in 30 languages". **No per-language metric**: the only multilingual rows are the model's own held-out synthetic sets (F1 0.990 and 0.994 over all 30 languages), which the card itself says flatter the model. Its limitations section says English is the largest language and the other 29 have fewer examples |
| `jhu-clsp/mmBERT-small` (the backbone). <https://huggingface.co/jhu-clsp/mmBERT-small> | MIT | 140M parameters, 42M outside the embeddings; 8,192-token context | Trained on more than 1,800 languages. The page does not list them, so Turkish is not confirmed there |
| `meta-llama/Llama-Prompt-Guard-2-86M`. <https://huggingface.co/meta-llama/Llama-Prompt-Guard-2-86M> | Llama 4 Community License | 86M backbone (mDeBERTa); 512-token context | Evaluated in eight languages: English, French, German, Hindi, Italian, Portuguese, Spanish, Thai. Turkish is not mentioned. One aggregate multilingual AUC (.995), no per-language figure |
| `protectai/deberta-v3-base-prompt-injection-v2`. <https://huggingface.co/protectai/deberta-v3-base-prompt-injection-v2> | Apache-2.0 | about 0.2B parameters | English only by its own card; does not target jailbreaks; the project is archived |

No public measurement of these guard models on Turkish text was found.

So the "before" for Tamga's classifier in Turkish is not known from any
source. It has to be measured, and `classifier/finetune/` is written to
measure it first.

## Phrasing people use

From a Turkish-language write-up of jailbreak techniques
(<https://www.flowhunt.io/tr/blog/jailbreaking-ai-chatbots-techniques-and-defenses/>),
by category. These are descriptions; the texts in the Tamga set were written
from them, not copied.

| Category | How it is said in Turkish |
|---|---|
| Instruction override | An override verb on the instructions: "yok say", "görmezden gel", "unut", "boşver", "dikkate alma", "geçersiz say", on "talimat", "kural", "yönerge", "komut", usually with "önceki", "tüm" or the second-person possessive ("talimatlarını") |
| System prompt extraction | "sistem istemi", "sistem promptu", "sistem mesajı", "gizli talimatlar" with "göster", "yaz", "paylaş", "tekrarla", "ifşa et", "sızdır"; the write-up calls it "sistem prompt çıkarma" |
| Role and persona | "bundan sonra … gibi davran", "sen artık …sın", "rol yap", a named persona with "hiçbir kısıtlaması olmayan"; the write-up's terms are "kişilik ve rol yapma saldırıları" and "karakter canlandırma" |
| Developer mode and DAN | "geliştirici modu etkin", "DAN modu", a forged system line announcing the mode, the claim that content policies are off in it |
| Authority | "yetki sahteciliği" in the write-up: a claimed administrator, developer or vendor, usually with urgency |
| Indirect | Instructions inside a document, a note or stored memory addressed to the model |
| Obfuscation | "kodlama gizleme", "leet konuşma", "token kaçakçılığı", "sınır enjeksiyonu" (delimiters) in the write-up |
| Fictional and hypothetical framing | "kurgusal çerçeve", "varsayımsal ve eğitimsel çerçeveleme" |
| Gradual escalation | "çok adımlı kademeli yükselme": the request is built over several turns |

## Evasions specific to Turkish

| Evasion | Example | Source that names it | Tamga today |
|---|---|---|---|
| Dotted and dotless I | "İGNORE", "ıgnore", "TALIMATLARI" for "TALİMATLARI"; a filter that lower-cases with ASCII rules sees another word | The PyPI description of `garak-multilingual`, which credits a casefold probe in NVIDIA/garak | Folded before matching: every I, İ, ı and i is one letter (`normalize`, `turkishLower`) |
| Diacritics dropped | "onceki talimatlari yok say" | not named by a cited source | Folded: ç, ğ, ö, ş, ü are read as c, g, o, s, u, so both spellings are one text |
| Suffix chains | "talimat", "talimatları", "talimatlarını", "talimatlarınızı", "talimatlardan" | not named by a cited source | Since 2026-10-11 the Turkish rules are stem plus a bounded class of endings (`injection_turkish.go`). Before, fixed phrases and a few expressions with one form each |
| Verb forms | "unut", "unutun", "unutunuz", "unutalım", "unutmalısın", "unutmanı istiyorum" against "unuttum", "unutmadan" | not named by a source; follows from the previous row | The same file: an ending decides whether the verb is a command |
| Turkish-English mixing | "tüm talimatları ignore et", "system promptu dump et" | code-switching in arXiv:2607.10112 | English verb with "et-" and English noun with a Turkish ending are read |
| Turkish leetspeak | "t4l1m4tl4r1 y0k s4y" | not named by a cited source | Read for the words the rules know (the leetspeak view); the new Turkish stems were not added to that list in this round |
| Split or spaced words | "tal imatları", "önce ki talimat ları" | not named by a cited source | Letter-by-letter spelling is read; a word cut in two at an arbitrary place is not |
| Circumflex vowels, reduplication, archaic synonyms | "kâide", "mülga" | not named by a cited source | Circumflex vowels fold to the plain vowel. Archaic synonyms are not in any rule |

Text that looks like an attack and is not, which no source above lists and
the Tamga benign half was written to cover:

- **Banking.** "Talimat" is a payment order: "düzenli ödeme talimatımı
  iptal et", "havale talimatı".
- **Law.** "Önceki tüm anlaşmalar geçersizdir" is an entire-agreement
  clause; "talimat" is what a court or a prosecutor gives.
- **Teaching.** "Önceki soruyu yok say", "kuralları unut, baştan anlat".
- **Software.** Linter, firewall, CSS and `.gitignore` "rules" are ignored,
  reset and overridden all day; "geliştirici modu" is a phone setting.
- **Customer service.** "Önceki mesajımı dikkate almayın", "hesabımdaki
  kısıtlamayı kaldırır mısınız".
- **Talk about the attack.** A security team asking how to detect "önceki
  talimatları yok say" writes the attack. A rule cannot tell mention from
  use; the Tamga set keeps such rows, labelled benign, and counts them as
  the false positives they are.

## Could not verify

- **Minionese (arXiv:2607.10112) and Turkish.** A search summary placed
  Turkish in its "mid-low" tier with Indonesian, Hindi and Swahili. The
  abstract page does not name the languages and the paper was not opened.
  No Turkish figure from it is used here.
- **MultiJail and XSafety language lists.** Taken from a search summary of
  the two papers, not from the papers.
- **An MDPI Applied Sciences article (volume 16, issue 13, article 6740;
  <https://www.mdpi.com/2076-3417/16/13/6740>).** A search snippet says it
  notes that comparative evidence for Turkish open-source, localized and
  commercial models is limited. The page answered 403. Title, authors and
  results are unknown.
- **`garak-multilingual`.** <https://pypi.org/project/garak-multilingual/>
  did not render in the fetch tool. That it is a plugin for NVIDIA garak
  with Turkish injection probes and a dotted/dotless-I probe, and that it
  credits NVIDIA/garak pull request 1997, is from the search engine's
  summary of the page. Licence, author and version are unknown.
- **The update dates of the dataset cards.** None was shown.
- **Turkish forum threads and community posts with attack text in the
  wild.** The searches run returned the datasets and the write-up above and
  no forum thread. That is a statement about four searches, not about the
  web.
- **Unicode's Turkish casing rules** (I lowers to ı and İ to i under the
  `tr` and `az` locales, in `SpecialCasing.txt`) are stated from general
  knowledge; the file was not re-read on this date.
- Every figure quoted from a web page reached this document through a
  summarising fetch. The tables of arXiv:2310.02446 and arXiv:2511.00689
  were returned as tables and are the best supported; anyone citing them
  onward should open the papers.
