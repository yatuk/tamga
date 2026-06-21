export const metadata = {
  title: "KVKK Uyumu — Tamga Trust",
  description:
    "6698 Sayılı Kişisel Verilerin Korunması Kanunu kapsamında Tamga'nın teknik ve idari tedbirleri.",
};

export default function KVKKPage() {
  return (
    <main className="mx-auto w-full max-w-4xl px-6 py-16 text-zinc-800 dark:text-zinc-200">
      <p className="font-mono text-[11px] uppercase tracking-[0.2em] text-red-400">
        TRUST // KVKK
      </p>
      <h1 className="mt-2 text-4xl font-extrabold tracking-tight text-white">
        KVKK uyum notları
      </h1>
      <p className="mt-4 text-sm leading-7 text-zinc-600 dark:text-zinc-400">
        Tamga, 6698 sayılı Kişisel Verilerin Korunması Kanunu (KVKK) kapsamında veri
        işleyen (data processor) konumundadır. Müşterilerimiz veri sorumlusu
        (controller) olarak kalır; Tamga onların talimatı doğrultusunda işleme yapar.
      </p>

      <h2 className="mt-10 text-2xl font-semibold tracking-tight text-white">
        Madde 12 — Güvenlik yükümlülükleri
      </h2>
      <ul className="mt-3 list-disc space-y-2 pl-6 text-sm leading-7 text-zinc-600 dark:text-zinc-400">
        <li>Teknik: TLS 1.2+, at-rest AES-256, mTLS opsiyonu, RBAC, audit hash-chain.</li>
        <li>İdari: DPIA şablonu, tedarikçi güvenlik değerlendirmesi, yıllık penetrasyon testi.</li>
        <li>Erişim kontrolü: Clerk SSO + SCIM 2.0 otomatik provizyon.</li>
      </ul>

      <h2 className="mt-10 text-2xl font-semibold tracking-tight text-white">
        Madde 7 — Silme, yok etme ve anonim hale getirme
      </h2>
      <p className="mt-3 text-sm leading-7 text-zinc-600 dark:text-zinc-400">
        Veri sahibinin silme talebi halinde veri sorumlusu,{" "}
        <code className="rounded bg-zinc-100 dark:bg-zinc-900 px-1.5 py-0.5 text-xs text-amber-300">DELETE /api/v1/events/subject</code>{" "}
        endpoint&apos;ini çağırarak ilgili request_log kayıtlarını anında siler. İşlem
        audit log&apos;a yazılır.
      </p>

      <h2 className="mt-10 text-2xl font-semibold tracking-tight text-white">
        Madde 9 — Yurt dışına aktarım
      </h2>
      <p className="mt-3 text-sm leading-7 text-zinc-600 dark:text-zinc-400">
        TR deploy modunda hiçbir kişisel veri yurt dışına çıkmaz. Hybrid modda yalnızca
        politika tarafından izin verilen provider&apos;lar kullanılır; istekler redacted
        halde yabancı LLM sağlayıcısına iletilir.
      </p>

      <h2 className="mt-10 text-2xl font-semibold tracking-tight text-white">
        Saklama süreleri
      </h2>
      <ul className="mt-3 list-disc space-y-2 pl-6 text-sm leading-7 text-zinc-600 dark:text-zinc-400">
        <li>request_logs: varsayılan 90 gün, müşteri policy.data.retention_days ile özelleştirir.</li>
        <li>audit ring: bellek-içi 512 girdi; kalıcı audit store etkinse süresiz.</li>
        <li>findings JSON: policy.data.hash_findings=true ile yalnızca SHA-256 özet saklanır.</li>
      </ul>

      <h2 className="mt-10 text-2xl font-semibold tracking-tight text-white">
        VERBİS kaydı
      </h2>
      <p className="mt-3 text-sm leading-7 text-zinc-600 dark:text-zinc-400">
        Veri sorumlusu sıfatıyla VERBİS kaydı müşteriye aittir. Tamga, işleme
        envanterine eklenmesi gereken alan adlarını (endpoint, kategori, aktarım
        alıcısı) müşteriye hazır şablon olarak sunar.
      </p>
    </main>
  );
}
