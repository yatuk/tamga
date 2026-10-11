# What Turkish DLP deployments look for, and what Tamga detects of it

> This page is research, not legal advice. It records what vendors and
> regulators have published and what was read of it. A statement here about
> a law or a product is only as good as the source next to it; read the
> source before relying on it.

A buyer who already runs a classic DLP product (Microsoft Purview,
Symantec, Forcepoint, Trellix, Palo Alto and others) will ask whether Tamga
catches what that product catches. This page answers from published
material: which data types those products document for Türkiye, which data
the law names, how Turkish organisations word their rules for AI tools, and
what Tamga detects of each type. Everything was read on **2026-10-11**.

## How "most used" is meant here

No usage data was found: no vendor, regulator or analyst publishes how often
which DLP rule is enabled in Turkish organisations, and this page invents
none. The order of the table below is by **how many independent published
sources list the type** (vendor documentation that names a Türkiye-specific
detector, plus the law where it names the data). That measures what vendors
chose to ship and what the law names, which is a reason to expect a buyer to
ask for it. It does not measure what any organisation has turned on.

How each source was read is marked: **opened** means the page or document
itself was read; **search summary** means only a search engine's summary of
it was seen, and the claim should be treated as unconfirmed.

## Sources: DLP products

| Vendor and document | How read | Türkiye-specific content |
|---|---|---|
| Microsoft Purview, [Sensitive information type entity definitions](https://learn.microsoft.com/en-us/purview/sit-sensitive-information-type-entity-definitions) | opened | Two types in the whole list: "Turkey national identification number" and "Turkey physical addresses". No Turkish passport, tax number, driving licence or phone type |
| Microsoft Purview, [Türkiye national identification number](https://learn.microsoft.com/en-us/purview/sit-defn-turkey-national-identification-number) | opened | Format 11 digits; checksum: yes; medium confidence on the checksum function alone, high confidence with a keyword within 300 characters. Keywords: TC Kimlik No, TC Kimlik numarası, Vatandaşlık numarası, Vatandaşlık no |
| Microsoft Purview, [Türkiye physical addresses](https://learn.microsoft.com/en-us/purview/sit-defn-turkey-physical-addresses) | opened | A named-entity detector for postal addresses, medium confidence. No pattern is published |
| Google Cloud Sensitive Data Protection, [infoTypes reference](https://docs.cloud.google.com/sensitive-data-protection/docs/infotypes-reference) | opened (the whole page, in two parts) | One Türkiye-specific infoType: `TURKEY_ID_NUMBER`. `IBAN_CODE`, `IMEI_HARDWARE_ID`, `PASSPORT`, `CREDIT_CARD_NUMBER` and `VEHICLE_IDENTIFICATION_NUMBER` are global types. There is no `TURKEY_TCKN` |
| Amazon Macie, [Managed data identifiers by type](https://docs.aws.amazon.com/macie/latest/user/mdis-reference-quick.html) | opened | One: `TURKIYE_BANK_ACCOUNT_NUMBER` (the Turkish IBAN, no keyword required). No Turkish national id, passport, tax number, phone or driving licence identifier |
| Symantec DLP (Broadcom) 16.1, [Selecting a data identifier breadth](https://techdocs.broadcom.com/us/en/symantec-security-software/information-security/data-loss-prevention/16-1/about-data-loss-prevention-policy-authoring/data-identifiers/configuring-data-identifier-policy-conditions/selecting-a-data-identifier-breadth.html) | opened | Six system data identifiers: Turkish Identification Number, Turkey Tax Identification Number, Turkey Value Added Tax (VAT) Number, Turkey Passport Number, Turkey Local Phone Number, Turkey Mobile Phone Number. Their patterns and validators are on pages that were not opened |
| Symantec DLP 15.5, [Content Packs](https://techdocs.broadcom.com/content/dam/broadcom/techdocs/symantec-security-software/information-security/data-loss-prevention/generated-pdfs/Symantec_DLP_15.5_Content_Packs.pdf) | opened | Content Pack 2020-01 added the passport, VAT and two phone identifiers and "Turkish Personal Data Protection Law 6698 policy templates". What those templates contain is not in the document |
| Forcepoint DLP v8.4, [Predefined Policies and Classifiers](https://help.forcepoint.com/docs/data/v84/policy_classifier/policies_and_classifiers.pdf) | opened | Policies "Turkey PII" (TC Kimlik, alone and with support terms), "Turkey Finance" (Turkish IBAN, Turkish Tax IDs) and "Turkey Protection of Personal Data Draft Law" (TC Kimlik, TC Kimlik with a card number, spreadsheets, "Confidential in Header Footer"). Classifiers: validated TC Kimlik numbers, validated Turkish tax numbers, Turkish IBAN, and spreadsheets whose column headers name Turkish personal data. An IMEI policy exists that is not specific to Türkiye. This is an old version; the current list was not found in the 10.x pages that were opened |
| Forcepoint DLP v8.7.2 release notes | search summary (the URL now redirects) | Said to add classifiers and rules for Turkish passport and phone numbers |
| Palo Alto Networks Enterprise DLP, [National Id - Turkey Identification Number](https://docs.paloaltonetworks.com/enterprise-dlp/reference/predefined-data-pattern-keywords/nationalid-turkeyidentificationnumber) and [Passport - Turkey](https://docs.paloaltonetworks.com/enterprise-dlp/reference/predefined-data-pattern-keywords/passport-turkey) | opened | National id keywords: vatandaş kimlik numarası, türkiye cumhuriyeti kimlik numarası, kişisel kimlik numarası, kişisel kimlik no, vatandaşlık no, vatandaş kimliği, kimlik numarası, vatandaşlık numarası, tc kimlik numarası. Passport keywords: pasaport, geçişlik, gecislik. The patterns are not published. A "Bank - IBAN - Turkey" pattern: search summary only |
| Skyhigh Security (formerly McAfee), [Middle Eastern Personal Identity](https://success.skyhighsecurity.com/Skyhigh_Data_Loss_Prevention/Data_Identifiers/Middle_Eastern_Personal_Identity) | opened | "Turkish Identification Number": patterns `[1-9]{1}[0-9]{10}` and `\d{11}`, a keyword list in Turkish and English, no checksum listed. The page says the feature is a legacy one |
| SonicWall Cloud App Security, [predefined DLP rules: Turkey](https://www.sonicwall.com/support/technical-documentation/docs/cas-administration_guide_office365/Content/DLP/dlp-predefinedrules-countries-turkey.htm) | opened | One rule: Turkish identification number |
| OpenText (Micro Focus) IDOL Eduction grammars 26.2, [entities for tr](https://origin-www.microfocus.com/documentation/idol/knowledge-discovery-26.2/EductionGrammars_26.2_Documentation/Help/Content/GrammarReference/cc_tr.htm) | opened | National id, passport, driving licence, tax number and VAT number, IBAN and SWIFT code, telephone, postcode, postal address, personal name, nationality, e-mail. No checksum is mentioned. No plate, no health entity |
| Trellix DLP | not opened (TLS error) | A search listed pages titled "Turkish ID" and "Turkish Identification Number" in the Trellix documentation |

Not found, and so not counted: Netskope (its list of predefined identifiers
is for customers only, by a search summary), Zscaler (no Turkish dictionary
in the pages a search returned), Digital Guardian (Fortra), Proofpoint,
Nightfall, and any Turkish DLP vendor's public list of detectors.

## Sources: law and regulators

| Source | How read | What it names |
|---|---|---|
| Kişisel Verilerin Korunması Kanunu, No. 6698, [consolidated text](https://www.mevzuat.gov.tr/MevzuatMetin/1.5.6698.pdf), Md. 3/1-d | opened | Personal data is any information relating to an identified or identifiable natural person ("kimliği belirli veya belirlenebilir gerçek kişiye ilişkin her türlü bilgi"). The law lists no identifiers |
| Same law, Md. 6/1 | opened | Special categories: race, ethnic origin, political opinion, philosophical belief, religion, sect or other beliefs, appearance and dress, membership of an association, foundation or trade union, health, sexual life, criminal conviction and security measures, biometric and genetic data |
| Kişisel Verileri Koruma Kurumu, Kişisel Veri Güvenliği Rehberi (Teknik ve İdari Tedbirler), Ocak 2018, read in the copy at [turmob.org.tr](https://www.turmob.org.tr/arsiv/mbs/resmigazete/-veri_guvenligi_rehberi.pdf) | opened (a copy, not the authority's own site) | Defines "Veri kaybı/sızıntısı önleme (DLP)" as software that stops personal data leaving the organisation or reports it, and lists "Veri Kaybı Önleme Yazılımları" and "Veri Maskeleme" among technical measures. It names no data types for such software |
| Kişisel Verileri Koruma Kurumu, [Türkiye Cumhuriyeti Kimlik Numaralarının İşlenmesi Hakkında Rehber](https://www.kvkk.gov.tr/Icerik/7798/Turkiye-Cumhuriyeti-Kimlik-Numaralarinin-Islenmesi-Hakkinda-Rehber) | search summary | A guide of the authority on processing the national id number, on the ground that the number gives access to a person's other data |
| Bankacılık Kanunu, No. 5411, [consolidated text](https://www.mevzuat.gov.tr/MevzuatMetin/1.5.5411.pdf), Md. 73, third paragraph | opened | Data of natural and legal persons that arises after a customer relationship with a bank is established becomes a customer secret ("müşteri sırrı"). It is not shared with or transferred to third parties in Türkiye or abroad without a request or instruction from the customer, even with explicit consent under Law 6698, outside the listed exceptions. The law names no data fields |
| BDDK, Sır Niteliğindeki Bilgilerin Paylaşılması Hakkında Yönetmelik, Resmî Gazete 4/6/2021, sayı 31501, Md. 4, read in the copy at [alomaliye.com](https://www.alomaliye.com/2021/06/04/sir-niteligindeki-bilgilerin-paylasilmasi-hakkinda-yonetmelik/) | opened (a copy, read through a summarising tool) | Information that shows a person is a bank's customer is a customer secret; data from before the relationship becomes one when processed together with data that shows the person is a customer |
| BDDK, Bankaların Bilgi Sistemleri ve Elektronik Bankacılık Hizmetleri Hakkında Yönetmelik | read earlier for [compliance/bddk.md](../compliance/bddk.md) | Md. 6: classification by whether data is "hassas veri, kişisel veri ya da sır kapsamındaki veri". No list of identifiers |

SPK, TCMB, BKM and PCI DSS were not read for this page. Card numbers are
covered by every product above as a global type, and Tamga's handling of
them (Luhn, the issuer table, last four digits only) predates this work.

**What follows from the two laws for a detector.** Neither law gives a list
of fields. Law 6698 makes any identifier personal data once it points to a
person; Law 5411 makes anything about a bank's customer a secret, including
the fact of being a customer. A pattern can find a number with a format. It
cannot find "a customer's account history" or "a diagnosis" written as a
sentence. Every product in the first table has the same limit and answers
it with identifiers, keyword dictionaries and document fingerprints.

## Data types

Ordered by the number of independent sources that list a Türkiye-specific
detector for the type (products opened, of the nine: Purview, Google, Macie,
Symantec, Forcepoint, Palo Alto, Skyhigh, SonicWall, IDOL).

| Type | Who lists it | Format and check digit | Context words the sources give | Source for the format |
|---|---|---|---|---|
| T.C. kimlik no (TCKN) | 8 of 9: Purview, Google (`TURKEY_ID_NUMBER`), Symantec, Forcepoint, Palo Alto, Skyhigh, SonicWall, IDOL. Not Macie. Law 6698 through the authority's guide on the number | 11 digits, the first not 0, the last two are check digits. In use: d10 = ((d1+d3+d5+d7+d9)×7 − (d2+d4+d6+d8)) mod 10, d11 = (d1+…+d10) mod 10 | TC Kimlik No, TC Kimlik numarası, Vatandaşlık no, kimlik numarası, vatandaş kimliği, kişisel kimlik no (Purview, Palo Alto, Skyhigh) | [python-stdnum `tr.tckimlik`](https://arthurdejong.org/python-stdnum/doc/2.1/stdnum.tr.tckimlik) (opened: 11 digits, two check digits, not starting with 0; it points to tckimlik.nvi.gov.tr). Purview states that a checksum exists. No document of the Nüfus ve Vatandaşlık İşleri Genel Müdürlüğü that states the formula was found |
| IBAN (TR) | 5 of 9: Macie (`TURKIYE_BANK_ACCOUNT_NUMBER`), Forcepoint (Turkish IBAN), IDOL; Purview and Google as a global IBAN type. Palo Alto by a search summary | `TR`, 2 check digits, 5 digit bank code, 1 reserved digit (0), 16 character account number: 26 characters. ISO 13616 mod-97 | IBAN, hesap | Search summaries of bank pages that describe the Merkez Bankası rule; the TCMB page itself returned 404. The length (26) is the ISO 13616 registry's |
| Passport number | 3 of 9 opened: Symantec, Palo Alto, IDOL; Forcepoint by a search summary; Google as a global type | Not established. No check digit is documented by any source opened | pasaport, geçişlik (Palo Alto) | None found. See "Could not verify" |
| Vergi kimlik no (VKN) | 3 of 9: Symantec (tax number, and VAT number separately), Forcepoint (validated Turkish Tax IDs), IDOL | 10 digits, the last a check digit | vergi, VKN (none of the opened pages publishes its list) | [python-stdnum `tr.vkn`](https://arthurdejong.org/python-stdnum/doc/2.1/stdnum.tr.vkn) (opened: 10 digits, a check digit; it points to the Gelir İdaresi verification service on e-Devlet). The formula in use is not published by the Gelir İdaresi in anything found |
| Telephone number | 3 of 9: Symantec (local and mobile), IDOL; Forcepoint by a search summary | `+90` or `0`, a three digit code (2xx to 4xx geographic, 5xx mobile), seven digits. No check digit | telefon, cep, GSM | The BTK numbering plan was not opened; the format is the one in common use |
| Card number | Every product, as a global type; Forcepoint pairs it with TC Kimlik in its Türkiye policy | 13 to 19 digits, Luhn | kart, kredi kartı, CVV | ISO/IEC 7812; the repo's issuer table (see `NOTICE`) |
| Postal address | 2 of 9: Purview, IDOL | Free text. Postcode: five digits | adres, mahalle, sokak, posta kodu | Not researched further: no pattern is published by either |
| Personal name | 1 of 9 (IDOL); Forcepoint through spreadsheet column headers | Free text | ad, soyad | No pattern exists; products use dictionaries or models |
| Driving licence number | 1 of 9: IDOL | Not established | ehliyet, sürücü belgesi | None found |
| IMEI | Global types only: Google (`IMEI_HARDWARE_ID`), Forcepoint (an IMEI policy) | 15 digits: an 8 digit type allocation code, a 6 digit serial number, a Luhn check digit. The 16 digit IMEISV has none | IMEI | 3GPP TS 23.003, section 6.2.1 (search summary of a copy at itecspec.com) |
| Yabancı kimlik no | No product lists it separately | 11 digits starting with 99; said to follow the TCKN check digits | yabancı kimlik no, YKN | Not verified against an official source. Numbers starting with 99 that pass the TCKN check are found by any TCKN detector |
| MERSİS no | No product | 16 digits, built from the T.C. kimlik no for a natural person and from the vergi no for a legal entity | MERSİS | [Ticaret Bakanlığı, ticaret sicili FAQ](https://ticaret.gov.tr/ic-ticaret/sikca-sorulan-sorular/ticaret-sicili) (opened) for the 16 digits and what the number is built from. Where the tax number or the national id sits in the 16 digits ("0" + VKN + five digits; TCKN + five digits) is from search summaries of bank pages only |
| Vehicle plate | No product | Province code 01 to 81, one to three letters, two to five digits, in six combinations: 99 X 9999, 99 X 99999, 99 XX 999, 99 XX 9999, 99 XXX 99, 99 XXX 999. Q, W and X are not used | plaka | Karayolları Trafik Yönetmeliği Md. 47 (search summary: the number is given from province codes and groups of letters and digits listed by the Emniyet Genel Müdürlüğü). The six shapes and the letters are from [Wikipedia, Vehicle registration plates of Turkey](https://en.wikipedia.org/wiki/Vehicle_registration_plates_of_Turkey) (opened), which is thinly sourced and contradicts itself on the letter X |
| SGK işyeri sicil no | No product | Said to be 26 digits: nature of the workplace, branch of work, unit, serial number, province, district, check number, intermediary code | SGK, işyeri sicil | Search summaries of accounting sites that paraphrase the Sosyal Sigorta İşlemleri Yönetmeliği. The check number's rule is not published |
| e-Fatura / e-Arşiv no, ETTN | No product | Invoice number said to be 16 characters (3 characters, the year, 9 digits). The ETTN is a UUID | fatura no, ETTN | Search summaries of integrators' pages. The Gelir İdaresi guides were not opened |
| KEP address | No product | An e-mail address under `hsNN.kep.tr` | KEP | Search summaries. Any e-mail detector finds it |
| Ticaret sicil no, TAPU ada/parsel, UYAP esas/karar no, posta kodu, öğrenci no | No product lists any of them for Türkiye (IDOL lists a postcode entity) | Not researched: short numbers without a check digit | | |
| Special categories of Md. 6 (health, religion, union membership, criminal record and the rest) | Forcepoint and Purview ship health dictionaries for other languages; no Turkish dictionary was seen in anything opened | Not a format | | Law 6698 Md. 6/1 |

## Policy wording: what Turkish organisations say must not go into an AI tool

Only what a published text says is recorded. Glosses are this page's, in
English; the Turkish in quotation marks is the document's own term.

| Source | How read | What it says about data |
|---|---|---|
| Kişisel Verileri Koruma Kurumu, [İş Yerlerinde Üretken Yapay Zekâ Araçlarının Kullanımı](https://www.kvkk.gov.tr/Icerik/8674/is-yerlerinde-uretken-yapay-zeka-araclarinin-kullanimi) ([PDF](https://www.kvkk.gov.tr/SharedFolderServer/CMSFiles/MTY5YTdkNjdjNzJlMjM.pdf)), Şubat 2026. Guidance, not a rule | opened | What staff are observed to share: meeting notes, internal correspondence, draft reports, information on employees or customers (p. 6). Risk to trade secrets: "kaynak kod", product designs, business strategies, "ticari sır" (p. 8). Some organisations' policies allow public tools only where the input holds no personal data, trade secret or sensitive corporate information, and may rule out "müşteri dosyaları", "insan kaynakları verileri" and "iç yazışmalar" (p. 12). Prefer anonymised and general wording without names, dates or places, with more caution for health data, financial information and information on legal proceedings (pp. 12-13). The document forbids nothing itself and says an outright ban is not expected to work (see [compliance/kvkk.md](../compliance/kvkk.md)) |
| Ankara Üniversitesi, [yönerge on generative AI in graduate work](https://sosbilens.ankara.edu.tr/wp-content/uploads/sites/271/2026/08/ANKARA-UNIVERSITESINDE-YURUTULEN-LISANSUSTU-CALISMALARDA-URETKEN-YAPAY-ZEKA-ARACLARININ-KULLANIMININ-USUL-VE-ESASLARINA-ILISKIN-YONERGE.pdf) (file dated August 2026), Md. 7(1)(ç) | opened | Students, advisers, jury members and staff may not upload personal data, ethics committee documents, confidential or unpublished information, or research data to publicly available generative AI systems. Anonymised parts without personal data or third parties' protected content are outside the ban |
| Sakarya Üniversitesi, [Üretken Yapay Zekâ Kullanım İlkeleri Rehberi](https://ortadogu.sakarya.edu.tr/sites/ortadogu.sakarya.edu.tr/file/SAKARYA_UNIVERSITESI_URETKEN_YAPAY_ZEKA_KULLANIM_ILKELERI_REHBERI.pdf), Senate decision of 25.07.2025 | opened | Personal and sensitive data is to be anonymised before use. Entering confidential, sensitive or proprietary university data into public tools is forbidden; its examples are student records, research data and financial information. Unpublished research and invention disclosures are named too |
| Sinop Üniversitesi, [yapay zekâ politikası](https://kvys.sinop.edu.tr/Home/Indir/4363) (a .docx; said by a search summary to be in force from 12.08.2025), item 9.3 | opened | AI systems are not to be used directly on sensitive data of students, staff or the institution. For research data it recommends anonymisation and not using the tools for sensitive data |
| Çanakkale Onsekiz Mart Üniversitesi, Yabancı Diller Yüksekokulu, [yapay zekâ uygulamaları politika metni](https://cdn.comu.edu.tr/cms/ydyo/files/2003-yapay-zek-uygulamalari-politika-metni-taslak-metin.pdf) (marked as a draft; file dated July 2025) | opened | Administrative staff anonymise content that holds personal data before giving it to an AI system. Entering other people's data without permission is treated as a KVKK breach. Giving exam questions, a teacher's notes and similar "gizli belge" to an AI tool is a disciplinary matter |
| TED Üniversitesi, [Etik ve Sorumlu Üretken Yapay Zeka Kullanım Politikası](https://www.tedu.edu.tr/sites/default/files/docs/TEDUniversitesiEtikveSorumluUretkenYapayZekaKullanimPolitikasi.pdf) (file dated February 2025) | opened | Data used in generative AI systems is handled under the university's privacy policy and data protection law. It names no kinds of data |
| TÜBİTAK, [Yapay Zekâ Politika Belgesi v1.0](https://tubitak.gov.tr/sites/default/files/2026-03/TUBITAK_Yapay_Zeka_Politika_Belgesi_v1.0_tr.pdf) (2026) | opened | Compliance with data privacy law and the confidentiality of "kurumsal bilgiler" are priorities. It names no kinds of data |

Read together, and going no further than the texts: the kinds of content
that at least one of them names are personal data (all), customer files and
HR data (the authority), internal correspondence (the authority), source
code and trade secrets (the authority), health, financial and legal
information (the authority), student records and research data (Ankara,
Sakarya), ethics committee documents (Ankara), financial information of the
institution (Sakarya), and exam questions (ÇOMÜ YDYO). None of them names
passwords or other credentials; none names contract texts or financial
statements before disclosure.

A pattern can act on few of these: an identifier inside a customer file,
a credential inside source code. "Internal correspondence" and "research
data" have no format.

## Could not verify

- **Any usage figure.** No source gives how many Turkish organisations run
  which DLP rule. The ranking above is by published sources.
- **A BDDK or Türkiye Bankalar Birliği text on staff use of generative
  AI.** Searches returned news about an AI workshop and a planned test
  environment for models, nothing on what staff may put into a chat tool.
- **Guidance of the Cumhurbaşkanlığı Dijital Dönüşüm Ofisi, or of a
  ministry or a municipality, on generative AI use by staff.** None was
  found. The Bilgi ve İletişim Güvenliği Rehberi was not opened.
- **A published AI usage policy of a Turkish bank or telecom operator.**
  None was found.
- **The YÖK ethics guide on generative AI in research (May 2024).** The
  address a search gave for it returned 404. Sinop's policy says it rests
  on that guide.
- **The authority's "15 Soruda" guide on generative AI and its note on chat
  bots.** Not opened (see [compliance/kvkk.md](../compliance/kvkk.md)).
- **A decision of the Kişisel Verileri Koruma Kurulu that a vehicle plate
  is personal data.** A search did not find one. A university's training
  slide lists "araç plakası" among examples of personal data; that is not a
  decision. Under Md. 3 a plate is personal data when it leads to a person.
- **VERBİS data categories.** The authority's own list was not opened.
- **The Turkish passport number format.** A letter followed by eight digits
  is common knowledge and is what Tamga's pattern takes, next to the word
  "pasaport" or "passport". No official source and no vendor page that was
  opened states the format, and older series may differ.
- **The driving licence number format.** Nothing found. No detector.
- **That every yabancı kimlik no passes the TCKN check.** Widely stated,
  not confirmed from the issuing authority.
- **The layout of a MERSİS number beyond what the ministry's FAQ says.**
  The detector requires the embedded tax number or national id to pass its
  own check digits, which holds whatever the last five digits mean; a
  MERSİS number laid out differently would not be found.
- **The six plate shapes and the unused letters**, beyond Wikipedia.
- **The TCMB communiqué on IBAN**, the BTK numbering plan, the Gelir
  İdaresi guides on e-Fatura numbers, the SGK workplace number's check
  rule, GSMA TS.06.
- **The detectors of Trellix, Netskope, Zscaler, Digital Guardian,
  Proofpoint and of Turkish DLP vendors.** Pages not reachable, for
  customers only, or not found.
- **Symantec's and Palo Alto's patterns and validators.** Only the names
  and, for Palo Alto, the keywords are on the pages opened.
- **The current Forcepoint list.** What was read is v8.4.

## Gap analysis against Tamga

"Before" is the code at commit `79c865c`. "Now" is after the change that
this page was written with (`proxy/internal/scanner/pii_tr_ids.go`).
Categories are the `category` of a finding of type `pii`.

| Type | Before | Now |
|---|---|---|
| T.C. kimlik no | Detected: `tc_kimlik`, severity critical, check digits validated, a list of known test and placeholder numbers; context words raise the confidence and are not required. Read in fullwidth digits, spelled as number words, split by separators, reversed | Unchanged |
| Yabancı kimlik no | Detected as `tc_kimlik` when it passes the check (not tested) | Unchanged; now pinned by a test and a corpus row |
| IBAN (TR and 69 other countries) | Detected: `iban`, critical, registry length per country and mod-97. The bank code is not checked against a list | Unchanged |
| Card number | Detected: `credit_card`, critical, Luhn, issuer table for brand and bank; the match keeps the last four digits. A fifteen digit IMEI and one MERSİS number in ten were reported as cards | An IMEI or a MERSİS number that the text names as one is no longer a `credit_card` finding (see below). Otherwise unchanged |
| Vergi kimlik no | Detected: `vkn`, high, check digit validated, only next to "vergi", "vkn", "tax id" or "tax number" | Unchanged |
| Telephone | Detected: `phone_tr` (0 or +90, codes 2xx to 5xx), `phone` (other countries, with "+") | Unchanged |
| E-mail, KEP address | Detected: `email` | Unchanged |
| Passport number | Partly: `passport_number` needs the English word "passport" and takes the 6 to 12 letters or digits after it, so "passport renewal" matches and "Pasaport No: U12345678" does not. No shipped rule lists it | New: `passport_tr`, high: a capital letter and eight digits within 40 bytes after "pasaport" or "passport" on the same line. No check digit exists to validate. `passport_number` is left as it was |
| MERSİS no | Not detected | New: `mersis_no`. 16 digits (plain, or with spaces or hyphens) next to the word MERSİS, of which the embedded number must pass: "0" + a valid VKN (severity high), or a valid TCKN at the start (severity critical, since it holds a person's national id in full) |
| IMEI | Not as such: reported as `credit_card` | New: `imei`, high: 15 digits with a valid Luhn check digit next to the word IMEI |
| Vehicle plate | Not detected | New: `license_plate_tr`, medium: province code 01-81, capital letters without Q, W, X, in the six shapes, next to "plaka" or "plate". The shipped policy does not act on it; the templates do |
| Date of birth | Detected: `date_of_birth` next to "doğum tarihi", "DOB", "birth", "born" | Unchanged |
| Postal address, personal name | Not detected | Not detected. No pattern exists; the products that list them use named-entity models or dictionaries. The optional classifier in Tamga is for injections, not for entities |
| Driving licence, SGK işyeri sicil no, ticaret sicil no, ada/parsel, UYAP numbers, posta kodu, öğrenci no | Not detected | Not detected: no verified format, or a short number without a check digit that would fire on ordinary text. Write a `custom_entities` pattern for the form your documents use |
| e-Fatura no, ETTN | Not detected | Not detected. An invoice number is not personal data by itself, and a UUID is in every log |
| Customer number, account number | Examples under `custom_entities` in the shipped policy | The same; the bank template carries two example patterns |
| Secrets (keys, tokens, passwords, connection strings) | Detected: 30 categories under type `secret` | Unchanged |
| Special categories of Md. 6 | Not detected, apart from `medical_record` (the English words "medical record" and a number) | Not detected. See below |

### Why there is no signal for special categories

The idea was a low-severity signal for an unambiguous term (a diagnosis,
"sendika üyeliği", "sabıka kaydı") standing next to an identifier. It was
left out for three reasons. No sourced Turkish vocabulary was found: the
products that ship health dictionaries were not seen to ship a Turkish one.
A signal in Tamga today is an `injection` finding (`scanner.Signal`), and
signals also set off the inline classifier and add to the risk score, which
would be wrong for a health term; making a second kind of signal changes
shared code that this work did not own. And a short keyword list would fire
on ordinary text in exactly the organisations that would turn it on: a
hospital's or an insurer's traffic is full of diagnoses. An operator who
needs it can write the terms that matter to them as a `custom_entities`
entry with action `WARN`.

### What the new detectors cost

Each of the four runs only in a text that holds its word, checked with a
byte comparison before any regular expression. Measured with
`go test ./internal/scanner -bench` on the development machine (best of six
runs, alternating old and new code): `ScanAll_WithPII` 4.30 ms before and
4.40 ms after (+2.3%), `ScanAll_SmallPrompt` 0.630 and 0.639 ms (+1.4%),
`ScanAll_MediumPrompt` 3.01 and 2.98 ms, `ScanAll_MixedThreats` 1.57 and
1.55 ms, `ScanSegments_200/segments` 11.0 and 10.9 ms. The differences are
inside the noise of the machine.

On the red-team corpus under the shipped policy (`go test ./cmd/redteam`),
with 22 rows added to `prompts.csv` (8 that hold an identifier the shipped
policy acts on, 2 with a plate, 12 look-alikes without one):

| `prompts.csv`, 415 rows | Caught | Benign flagged | Missed | Benign passed |
|---|---|---|---|---|
| Code and policy before | 214 | 0 | 29 | 172 |
| Code and policy now | 219 | 0 | 24 | 172 |

Of the 8 new rows the old code caught 3: the two IMEI rows, as card
numbers, and the foreigner's identity number. `holdout.csv` was not changed
and scores as before (78 caught, 0 flagged, 0 missed, 91 passed). The new
rows were written with the detectors, so the step is not a gain on text the
rules had not seen.
