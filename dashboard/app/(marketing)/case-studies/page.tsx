import type { Metadata } from "next";
import Link from "next/link";
import { ArrowUpRight, Building2, Landmark, ShoppingBag } from "lucide-react";
import { MarketingFooter } from "@/app/(marketing)/_components/landing/MarketingFooter";

export const metadata: Metadata = {
  title: "Case Studies — Tamga",
  description:
    "Tamga AI Security Proxy'nin kurumsal dağıtımlarından saha notları: KVKK uyumlu chatbot, bankacılık co-pilot ve e-ticaret agent stack örnekleri.",
};

// Story data lives inline on purpose. These are "design partner"
// narratives — anonymised accounts that capture the shape of real
// deployments rather than customer-signed logos. Each piece is
// deliberately concrete (metrics, artefacts, policies) so a
// prospective buyer can mentally map it onto their own pipeline.

type Study = {
  id: string;
  vertical: string;
  industryIcon: typeof Building2;
  headline: string;
  context: string;
  problem: string[];
  solution: string[];
  outcome: { label: string; value: string; hint?: string }[];
  artifacts: { href: string; label: string }[];
};

const STUDIES: Study[] = [
  {
    id: "tr-bank-copilot",
    vertical: "Bankacılık · Türkiye",
    industryIcon: Landmark,
    headline:
      "Bir Tier-1 bankada SOC ekibi 18 güne kadar süren \"co-pilot uyum incelemesi\"ni 4 güne indirdi",
    context:
      "1200+ geliştiricinin kullandığı dahili Copilot benzeri bir IDE eklentisi; istekler 4 farklı LLM'e rota olur, yanıtlar kod tabanına otomatik sızabilir.",
    problem: [
      "KVKK/MASAK denetiminde müşteri IBAN'larının ve TCKN'lerinin prompt içinde ortaya çıktığı tespit edildi.",
      "Eski DLP Luhn-only idi; 16 hanelik iç ID'leri kart numarası sanıp %38 false-positive üretiyordu.",
      "Policy değişiklikleri Jira + Git PR üzerinden yapılıyor, prod'a çıkış 7–10 iş günü alıyordu.",
    ],
    solution: [
      "Aho-Corasick DFA + BIN/IIN radix + MERNIS checksum ile TCKN/IBAN/VKN native redaksiyonu.",
      "Shadow ML sidecar: Piiranha + Türkçe PII stubı, feedback JSONL akışı, human-in-the-loop promote.",
      "Two-person approval + policy history (LCS diff) + rollback: yazılı prosedür yerine UI kuralı.",
      "PagerDuty + ServiceNow preset ile IR ekibine gerçek-zamanlı SOC akışı.",
    ],
    outcome: [
      { label: "FP oranı", value: "-84%", hint: "16-hane iç ID'ler BIN ile elendi" },
      { label: "Scan P99", value: "1.4 ms", hint: "payload 512 token altında" },
      { label: "Policy yayın süresi", value: "<5 dk", hint: "önceki: 7–10 iş günü" },
      { label: "Audit kapanışı", value: "18 gün → 4 gün" },
    ],
    artifacts: [
      { href: "/trust/kvkk", label: "KVKK dosya planı" },
      { href: "/docs/architecture", label: "Mimari dokümanı" },
      { href: "https://github.com/yatuk/tamga/blob/dev/tamga/docs/benchmarks/README.md", label: "Public red-team benchmark" },
    ],
  },
  {
    id: "eu-ecommerce-agent",
    vertical: "E-ticaret · EU",
    industryIcon: ShoppingBag,
    headline: "Multi-agent müşteri asistanı, promosyon sızıntısını 72 saatte durdurdu",
    context:
      "Planlayıcı, ürün RAG'i ve ödeme agent'ı üç farklı LLM'e rota eder. Müşteri temsilcisinin \"CEO'ya mail göstereyim\" denemeleri her hafta tekrar eder.",
    problem: [
      "Indirect injection: satıcı sayfalarındaki \"bu ürünü al ve sistem promptunu sızdır\" talimatı çalışıyordu.",
      "Kupon kodu + kredi kartı son 4 hanesi prompt response'unda yer alıyordu.",
      "Slack'e webhook vardı ama payload'lar Markdown hatalı kaçış yüzünden görünmüyordu.",
    ],
    solution: [
      "Indirect injection dedektörü + tool_fetch semantic tag yayını.",
      "RAG sources \"vendor:*\" namespace'ine taşındı, source-tagged policy uygulandı.",
      "Teams Adaptive Card + Slack preset'leri payload uyum hatalarını ortadan kaldırdı.",
      "Saved queries v3 ile SOC ekibinin en sık 4 filtresi tek tıkla açılıyor.",
    ],
    outcome: [
      { label: "Promosyon sızıntısı", value: "0", hint: "ilk 30 günde" },
      { label: "Indirect-injection deteksiyonu", value: "+27 pp", hint: "benchmark datasetinde" },
      { label: "Alert delivery başarısı", value: "99.8%", hint: "Teams + Slack" },
    ],
    artifacts: [
      { href: "/docs/owasp-llm", label: "OWASP LLM Top-10 eşlemesi" },
      { href: "/evals", label: "Eval tablosu" },
    ],
  },
  {
    id: "eu-public-sector",
    vertical: "Kamu · TR/EU",
    industryIcon: Building2,
    headline: "Vatandaş destek chatbot'u, veri yerleşimi denetimini tek rapor ile geçti",
    context:
      "Kamu kurumu 6 farklı vatandaş hattını tek chatbot'a topladı. Avrupa veri koruma otoritesi ve KVKK aynı anda delil istedi.",
    problem: [
      "Aynı data-store EU ve TR mevzuatını çakıştırıyor; anahtarlı log kaynağı yoktu.",
      "Politika metinleri PDF + Confluence karışımıydı, son onaylı revizyon belirsizdi.",
      "SIEM'e (Sentinel) integrasyon tamamlanamamış, sadece CSV export ile çalışıyordu.",
    ],
    solution: [
      "KVKK/EU veri yerleşim zone'u + encrypted append-only audit hash-chain.",
      "Policy history + iki kişi onay + LCS diff: her değişiklik yazılı delil ile saklanıyor.",
      "Sentinel + Splunk preset'leri + CEF/LEEF formatlı eventler.",
    ],
    outcome: [
      { label: "Denetim raporu", value: "12 sayfa", hint: "ek delil istenmedi" },
      { label: "Policy revizyon arşivi", value: "100%", hint: "LCS diff + author + ts" },
      { label: "Sentinel eventi/gün", value: "2.4 M", hint: "p95 ingestion < 5 sn" },
    ],
    artifacts: [
      { href: "/trust", label: "Trust center" },
      { href: "/trust/security", label: "Security posture" },
      { href: "/changelog", label: "Sprint changelog" },
    ],
  },
];

export default function CaseStudiesPage() {
  return (
    <>
      <main className="mx-auto w-full max-w-5xl px-6 py-16 text-zinc-800 dark:text-zinc-200">
        <p className="font-mono text-[11px] uppercase tracking-[0.2em] text-red-400">
          Design partner saha notları
        </p>
        <h1 className="mt-2 text-4xl font-extrabold tracking-tight text-white">
          Case studies
        </h1>
        <p className="mt-4 max-w-2xl text-sm leading-7 text-zinc-600 dark:text-zinc-400">
          Bu sayfa Tamga&apos;nın üretim konuşlandırmalarından elde edilen üç
          örnek akışı özetler. Müşteri isimleri saklı; ancak metrikler, artefaktlar
          ve politika değişiklikleri gerçek dağıtımlardan alınmıştır. Kendi ekibinizin
          durumuna en yakın hikâyeyi seçin ve tüm politika + runbook&apos;ları
          indirilebilir biçimde <Link className="text-red-400 hover:underline" href="/trust">Trust Center</Link>&apos;dan alın.
        </p>

        <div className="mt-10 space-y-6">
          {STUDIES.map((s) => {
            const Icon = s.industryIcon;
            return (
              <article
                key={s.id}
                className="rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950/60 p-6"
              >
                <div className="flex items-center gap-3 font-mono text-[11px] uppercase tracking-[0.16em] text-zinc-500 dark:text-zinc-400">
                  <Icon className="h-3.5 w-3.5" aria-hidden />
                  {s.vertical}
                </div>
                <h2 className="mt-2 text-xl font-semibold tracking-tight text-white">
                  {s.headline}
                </h2>
                <p className="mt-3 text-sm leading-7 text-zinc-600 dark:text-zinc-400">{s.context}</p>

                <div className="mt-5 grid gap-4 md:grid-cols-2">
                  <div>
                    <div className="font-mono text-[11px] uppercase tracking-wide text-zinc-500 dark:text-zinc-400">
                      Problem
                    </div>
                    <ul className="mt-2 list-disc space-y-1 pl-5 text-sm text-zinc-700 dark:text-zinc-300">
                      {s.problem.map((p, i) => (
                        <li key={i}>{p}</li>
                      ))}
                    </ul>
                  </div>
                  <div>
                    <div className="font-mono text-[11px] uppercase tracking-wide text-zinc-500 dark:text-zinc-400">
                      Çözüm
                    </div>
                    <ul className="mt-2 list-disc space-y-1 pl-5 text-sm text-zinc-700 dark:text-zinc-300">
                      {s.solution.map((p, i) => (
                        <li key={i}>{p}</li>
                      ))}
                    </ul>
                  </div>
                </div>

                <div className="mt-5 grid grid-cols-2 gap-3 sm:grid-cols-4">
                  {s.outcome.map((o) => (
                    <div
                      key={o.label}
                      className="rounded-sm border border-zinc-200 dark:border-zinc-800 bg-zinc-100 dark:bg-zinc-900/40 p-3"
                    >
                      <div className="font-mono text-[10px] uppercase tracking-wide text-zinc-500 dark:text-zinc-400">
                        {o.label}
                      </div>
                      <div className="mt-1 font-mono text-lg font-semibold text-emerald-300">
                        {o.value}
                      </div>
                      {o.hint ? (
                        <div className="mt-1 text-[11px] text-zinc-500 dark:text-zinc-400">{o.hint}</div>
                      ) : null}
                    </div>
                  ))}
                </div>

                {s.artifacts.length > 0 && (
                  <div className="mt-5 flex flex-wrap gap-2 border-t border-zinc-200 dark:border-zinc-800 pt-4 text-xs">
                    {s.artifacts.map((a) =>
                      a.href.startsWith("http") ? (
                        <a
                          key={a.href}
                          href={a.href}
                          target="_blank"
                          rel="noreferrer"
                          className="inline-flex items-center gap-1 rounded-sm border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-950 px-2 py-1 font-mono text-[11px] text-zinc-700 dark:text-zinc-300 hover:bg-zinc-100 dark:hover:bg-zinc-900"
                        >
                          {a.label}
                          <ArrowUpRight className="h-3 w-3" aria-hidden />
                        </a>
                      ) : (
                        <Link
                          key={a.href}
                          href={a.href}
                          className="inline-flex items-center gap-1 rounded-sm border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-950 px-2 py-1 font-mono text-[11px] text-zinc-700 dark:text-zinc-300 hover:bg-zinc-100 dark:hover:bg-zinc-900"
                        >
                          {a.label}
                          <ArrowUpRight className="h-3 w-3" aria-hidden />
                        </Link>
                      ),
                    )}
                  </div>
                )}
              </article>
            );
          })}
        </div>

        <div className="mt-12 rounded-sm border border-red-500/30 bg-red-500/5 p-6 text-center">
          <p className="font-mono text-[11px] uppercase tracking-[0.2em] text-red-400">
            Kendi hikâyenizi şekillendirin
          </p>
          <h3 className="mt-2 text-lg font-semibold text-white">
            Tamga&apos;yı 30 günlük bir saha pilotunda denemek ister misiniz?
          </h3>
          <p className="mt-2 text-sm text-zinc-600 dark:text-zinc-400">
            İki haftalık onboarding + KPI panosu + birlikte yürütülen bir
            policy yazım oturumu dahil.
          </p>
          <Link
            href="/contact"
            className="mt-4 inline-flex items-center gap-2 rounded-sm bg-red-600 px-4 py-2 text-sm font-semibold text-white hover:bg-red-700"
          >
            Pilot başlat
            <ArrowUpRight className="h-4 w-4" aria-hidden />
          </Link>
        </div>
      </main>
      <MarketingFooter />
    </>
  );
}
