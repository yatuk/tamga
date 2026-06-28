export const metadata = {
  title: "Güvenlik — Tamga Trust",
  description:
    "Tamga güvenlik mimarisi, tehdit modeli ve olay müdahale yaklaşımı.",
};

export default function SecurityPage() {
  return (
    <main className="mx-auto w-full max-w-4xl px-6 py-16 text-zinc-800 dark:text-zinc-200">
      <p className="font-mono text-[11px] uppercase tracking-[0.2em] text-red-400">
        TRUST // SECURITY
      </p>
      <h1 className="mt-2 text-4xl font-extrabold tracking-tight text-white">
        Güvenlik mimarisi
      </h1>

      <h2 className="mt-10 text-2xl font-semibold tracking-tight text-white">
        Tehdit modeli
      </h2>
      <p className="mt-3 text-sm leading-7 text-zinc-600 dark:text-zinc-400">
        Tamga, LLM trafiğini in-line olarak tarar. Tehdit modelimiz OWASP LLM Top 10
        (2025) kapsamını birebir karşılar: prompt injection (LLM01), hassas bilgi
        ifşası (LLM02), zehirli model çıktısı (LLM05), aşırı yetki verme (LLM08),
        model servis reddi (LLM04) ve zincir modellerdeki aracı-araç saldırıları
        (LLM07).
      </p>

      <h2 className="mt-10 text-2xl font-semibold tracking-tight text-white">
        Katmanlı savunma
      </h2>
      <ol className="mt-3 list-decimal space-y-2 pl-6 text-sm leading-7 text-zinc-600 dark:text-zinc-400">
        <li>
          <strong className="text-zinc-800 dark:text-zinc-200">Input scan:</strong> PII/PCI/Secret/Injection
          tarayıcıları, 5 ms altı hedef ile Aho-Corasick DFA üzerinde çalışır.
        </li>
        <li>
          <strong className="text-zinc-800 dark:text-zinc-200">Policy engine:</strong> YAML tabanlı kural
          motoru; versiyonlanmış, iki-kişi onaylı, rollback edilebilir.
        </li>
        <li>
          <strong className="text-zinc-800 dark:text-zinc-200">Output scan:</strong> Canary token + PII/secret
          tarayıcıları yanıt gövdesine de uygulanır; streaming yanıtlar için sliding
          window stratejisi.
        </li>
        <li>
          <strong className="text-zinc-800 dark:text-zinc-200">Circuit breaker:</strong> Provider kısa
          devre; rate-limit ve budget cap koruması.
        </li>
      </ol>

      <h2 className="mt-10 text-2xl font-semibold tracking-tight text-white">
        Kriptografi
      </h2>
      <ul className="mt-3 list-disc space-y-2 pl-6 text-sm leading-7 text-zinc-600 dark:text-zinc-400">
        <li>TLS 1.2/1.3 (ECDHE + AES-GCM / ChaCha20-Poly1305).</li>
        <li>Client-cert auth (mTLS) opsiyonu — regüle edilen müşteriler için aktif.</li>
        <li>Audit log hash-chain: SHA-256(prev_hash || canonical_json(entry)).</li>
      </ul>

      <h2 className="mt-10 text-2xl font-semibold tracking-tight text-white">
        Olay müdahalesi
      </h2>
      <p className="mt-3 text-sm leading-7 text-zinc-600 dark:text-zinc-400">
        T+0–1 saat: olay tespiti & triage. T+4 saat: containment & client notification.
        T+72 saat: KVKK Madde 12 kapsamında KVKK Kurumu&apos;na bildirim hazırlığı.
        T+30 gün: post-mortem ve control gap closure.
      </p>
    </main>
  );
}
