# Tamga Dashboard — Design Reference

> Kaynak: compliance/security SaaS ürünlerinin (Datadog CSM, Drata, Wiz) insan tarafından
> görsel incelemesinden çıkarılan pattern'ler. Her pattern kaynağıyla eşleştirilmiştir.

## Kaynak

| Pattern | Kaynak | Ekran görüntüsü |
|---|---|---|
| Posture Score kartı (hero sayı + sparkline + gradient) | Datadog CSM | `/tmp/design-research/datadog-posture-blog.png` |
| Top Failing Findings listesi | Datadog CSM | `/tmp/design-research/datadog-posture-blog.png` |
| Rules Severity Breakdown (gradient bar) | Datadog CSM | `/tmp/design-research/datadog-posture-blog.png` |
| Framework Readiness kartları (ince çizgi progress) | Drata | `/tmp/design-research/drata-overview.png` |
| Donut + legend widget'ları (Policies / Vendor risks) | Drata | `/tmp/design-research/drata-overview.png` |
| Drill-down slide-in panel | Drata | `/tmp/design-research/drata-overview.png` |
| Weighted risk score (tek sayı metodolojisi) | Wiz | `/tmp/design-research/wiz-scanning-report.png` |

---

## Değişen Bileşenler

### 1. Posture Score (yeni) — Datadog pattern

- **Component:** `components/dashboard/PostureScore.tsx` (yeni)
- **Nereden:** Datadog + Wiz
- **Görsel:**
  - Sol üstte küçük gri "Posture Score" başlığı + yanında `ⓘ` bilgi ikonu (tıklanınca hesaplama metodolojisi tooltip).
  - Kart gövdesinde ortalanmış **çok büyük yüzde** (ör. `88.5%`), skora göre renkli (koyu yeşil/amber/kırmızı).
  - Sayının arkasında/altında **sparkline**: açık renkli dolgu, dalgalı çizgi; grafiğin baş/son değerleri üstte ve altta küçük sayılarla (dönem başı `89.6%`, dönem sonu `87.2%`).
  - Kart arka planı skora göre **gradient** (iyi → açık yeşil, orta → amber, kötü → kırmızı/turuncu). Gradient tüm kart boyunca, sadece sayının arkasında değil.
  - Dikey dikdörtgen (kare değil).
- **Hesaplama mantığı (Tamga):** `Posture Score = 100 − weighted_input_risk`. `avgInputRiskPct` (0-100, yüksek=kötü) ters çevrilir; ağırlıklar finding severity'sine göre (critical > high > medium > low). Formül `ⓘ` tooltip'te açıklanır.
- **Renk eşikleri:** `≥90` → yeşil (`status-pass`), `70–89` → amber (`status-medium`), `<70` → kırmızı (`status-critical`). Gradient buna göre.
- **Props (mock için):** `{ score: number; delta: number|null; series: {t:number;v:number}[]; methodology?: string }`.
- **Empty state:** veri yoksa score `—`, sparkline yerine "No posture history" tek satır; gradient nötr (`surface-card`).

### 2. Compliance Readiness (yeni) — Drata pattern

- **Component:** `components/dashboard/ComplianceReadiness.tsx` (yeni)
- **Nereden:** Drata (kart yaklaşımı — **donut değil**)
- **Framework'ler (Tamga):** KVKK, BDDK, GDPR, OWASP LLM Top 10 (responsive grid, 3-4 kart yan yana).
- **Görsel (kart başına):**
  - Solda dairesel ikon (framework'e özel).
  - Sağda framework adı (bold) + versiyon/standart alt satırda gri.
  - Altında **tam genişlikte ince yatay çizgi** (progress): dolu kısım yeşil, kalan kısım açık gri. (`h-1` ince çizgi, kalın bar değil.)
  - Çizginin altında küçük gri "Framework readiness" etiketi.
  - İki sütun yan yana: **"Ready controls"** (büyük bold yüzde, `100%`) ve **"Requirements"** (büyük bold sayı, `229`).
  - En altta "Controls" etiketi + toplam sayı.
- **Data (mock):** her framework için `{ id, name, icon, readyPct, readyControls, totalControls, remaining }`. `remaining = totalControls − readyControls`.
- **Empty state:** `readyPct` yoksa `0%`, çizgi boş; "Not evaluated yet" alt satır.

### 3. Top Findings (yeni) — Datadog pattern

- **Component:** `components/dashboard/TopFindings.tsx` (yeni)
- **Nereden:** Datadog CSM
- **Görsel:**
  - Başlık "Top Failing Findings" + sağda "View all in Events →" linki.
  - Her satır: solda **3-4px ince dikey renkli çizgi** (severity indicator), sonra bulgu metni (`truncate` + `…`), sağda iki sütun: **RESOURCES** (sayı) ve **TRIAGE** (mavi pill badge, `1 OPEN`).
  - Satırlar arası ince gri ayraç; hover yok, sade liste.
- **Data (Tamga):** son 24h/7d'de en kritik finding'ler (severity + type + category). Kaynak: `derived.recentEvents` üzerindeki `findings[]`. Severity sol çizgi rengini belirler (`critical`→`status-critical`, `high`→`status-high`, `medium`→`status-medium`, `low`→`status-low`).
- **Props:** `{ findings: { id, text, severity, resources, triageOpen }[]; onViewAll: () => void }`.
- **Empty state:** "No failing findings detected" + küçük pas-yeşil onay ikonu.

### 4. Severity Breakdown (yeni) — Datadog pattern

- **Component:** `components/dashboard/SeverityBreakdown.tsx` (yeni)
- **Nereden:** Datadog CSM
- **Görsel:**
  - Her severity (CRITICAL, HIGH, MEDIUM, LOW, INFO) bir satır.
  - Solda severity adı **düz gri metin** (kırmızı badge değil).
  - Sağında yatay progress bar, **kırmızıdan yeşile geçen gradient dolgu**.
  - Bar'ın sağında yüzde + fraction: `66%  2 of 3` (tabular-nums monospace).
- **Data:** bulguların severity'ye göre sayımı (critical/high/medium/low). INFO hariç tutulabilir (Tamga'da yok).
- **Renk:** bar dolgusu severity'ye göre değil, **tek gradient** (yeşil→kırmızı) — geçen kısım yeşil, kalan kısım açık gri/red-ish. Adı gri.
- **Empty state:** tüm sayılar 0 ise "No findings in range".

### 5. KPI Card (evrim, mevcut) — Datadog

- **Mevcut:** `components/dashboard/MetricStat.tsx` (zaten `sparkline` + `delta` + `tooltip` + `live` destekli).
- **Değişiklik:** Yeni `PostureScore` "hero sayı + büyük sparkline + gradient" pattern'ini üstlenir; `MetricStat` küçük KPI kartları olarak kalır, görsel olarak Datadog KPI kartına hizalanır (gömülü sparkline zaten var — yeni sparkline eklemeye gerek yok).
- **KPI seti:** Total Requests, Blocked, Redacted, Warned, Avg Input Risk, P95 Scan Latency, MTTR, Budget Burn, Active Models, Cost.

---

## Layout Hiyerarşisi

Üstten alta (overview `page.tsx`):

1. **Satır 1 — Hero grid (3 sütun):** `PostureScore` | `TopFindings` | `SeverityBreakdown`
2. **Satır 2 — Compliance:** `ComplianceReadiness` (KVKK / BDDK / GDPR / OWASP, 4 kart)
3. **Satır 3 — KPI grid:** `MetricStat` kartları (Total/Blocked/Redacted/Warned + P95/MTTR)
4. **Satır 4 — Trafik:** `OverviewTrafficChart` (mevcut)
5. **Satır 5 — Risk dağılımı + canlı akış:** provider pie + `RecentIncidents` listesi (mevcut `OverviewViewPartB/C`)

**Filtre:** sadece **severity** ve **zaman aralığı** (24h/7d/30d). Datadog'un Account/Service/Env/Team dropdown'ları **alınmayacak** (Tamga veri modeline uygun değil).

---

## Renk & Tipografi Kararları

- **Posture score eşikleri:** `≥90` → `status-pass` (yeşil), `70–89` → `status-medium` (amber), `<70` → `status-critical` (kırmızı).
- **Severity (tek kaynak `lib/badges.ts`):** critical → `status-critical`, high → `status-high`, medium → `status-medium`, low → `status-low`, pass → `status-pass`.
- **Severity adı:** düz gri `text-fg-muted` (renkli badge değil) — Datadog'un severity breakdown'ındaki gibi.
- **Fraction formatı:** `"N of M"`, `font-mono tabular-nums`.
- **Pill badge (TRIAGE / severity):** mevcut `ActionBadge`/`SeverityBadge` token'ları (kalın renkli değil, `border-status-*/40 bg-status-*-bg text-status-*`).
- **Hero sayı:** `text-4xl`+ boyut, `font-mono tabular-nums`, skor rengi.
- **Progress bar:** `h-1` ince çizgi (Drata readiness), gradient dolgu (Datadog severity).
- Mevcut OKLCH token sistemi (`--surface-*`, `--fg-*`, `--status-*`, `--chart-*`) kullanılır; **yeni renk sistemi icat edilmez**.

---

## Empty State Stratejisi

| Component | Boş veri davranışı |
|---|---|
| `PostureScore` | score `—`, sparkline yerine "No posture history", nötr gradient |
| `ComplianceReadiness` | `0%`, boş çizgi, "Not evaluated yet" |
| `TopFindings` | "No failing findings detected" + pas-yeşil onay ikonu |
| `SeverityBreakdown` | "No findings in range" |
| `MetricStat` | mevcut `value` `—` (count-up 0'dan) |

Her yeni component mock data ile **izole render edilebilir** olmalı (Storybook yoksa bile `npm run dev`'de ayrı ayrı test edilebilir).
