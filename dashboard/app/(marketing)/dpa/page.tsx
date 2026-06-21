import Link from "next/link";
import { MarketingDoc } from "@/app/(marketing)/_components/marketing/MarketingDoc";

export const metadata = {
  title: "Veri İşleyici Sözleşmesi (DPA) — Tamga",
  description:
    "Tamga AI Güvenlik Proxy'si için KVKK ve GDPR uyumlu Veri İşleme Sözleşmesi (Data Processing Agreement) taslağı.",
};

export default function DPAPage() {
  return (
    <MarketingDoc
      eyebrow="LEGAL // DPA"
      title="Veri İşleme Sözleşmesi"
      lastUpdated="17 Nisan 2026"
      intro={
        <p>
          Bu Veri İşleme Sözleşmesi (&quot;DPA&quot;), Tamga&apos;nın
          (&quot;Veri İşleyen&quot;) KVKK Madde 11 ve GDPR Madde 28 uyarınca
          müşteri kuruluş (&quot;Veri Sorumlusu&quot;) adına kişisel verileri
          hangi koşullarda işleyeceğini tanımlar. Son imzalı PDF için{" "}
          <a href="mailto:legal@tamga.dev">legal@tamga.dev</a> adresine
          ulaşabilirsiniz.
        </p>
      }
    >
      <h2>1. Tanımlar</h2>
      <p>
        Bu DPA kapsamında &quot;kişisel veri&quot;, &quot;veri sorumlusu&quot;,
        &quot;veri işleyen&quot;, &quot;işleme&quot;, &quot;ilgili kişi&quot;,
        &quot;ihlal&quot; terimleri 6698 sayılı KVKK ve GDPR&apos;ın ilgili
        maddelerindeki anlamlarıyla kullanılır.
      </p>

      <h2>2. İşleme Kapsamı</h2>
      <ul>
        <li>
          <strong>Amaç:</strong> Veri Sorumlusu&apos;nun LLM sağlayıcılarına
          gönderdiği isteklerin güvenlik taraması, PII/PCI maskelemesi ve
          politika uygulaması.
        </li>
        <li>
          <strong>Süre:</strong> Aktif abonelik süresi + 30 gün silme penceresi.
        </li>
        <li>
          <strong>Veri kategorileri:</strong> Prompt metinleri, model yanıtları,
          API kullanım telemetrisi, kullanıcı kimliği (opak SSO subject ID).
        </li>
        <li>
          <strong>İlgili kişi kategorileri:</strong> Veri Sorumlusu&apos;nun
          çalışanları, müteahhitleri ve son kullanıcıları.
        </li>
      </ul>

      <h2>3. Veri İşleyen Yükümlülükleri</h2>
      <ul>
        <li>Verileri yalnızca yazılı talimatlar doğrultusunda işlemek.</li>
        <li>
          Verilere erişen personel için gizlilik taahhütleri almak ve yıllık
          KVKK farkındalık eğitimi uygulamak.
        </li>
        <li>
          TLS 1.3+, AES-256 depolama, ve mTLS admin erişimi dahil{" "}
          <Link href="/trust/security">güvenlik kontrolleri</Link> uygulamak.
        </li>
        <li>
          <Link href="/subprocessors">Alt işleyen listesindeki</Link> değişiklikler için
          30 gün önceden bildirim vermek.
        </li>
        <li>
          İhlalleri 72 saat içinde Veri Sorumlusu&apos;na bildirmek (KVKK 12/5
          ve GDPR 33 uyumlu).
        </li>
      </ul>

      <h2>4. İlgili Kişi Hakları</h2>
      <p>
        Tamga, Veri Sorumlusu&apos;nun ilgili kişi taleplerini karşılamasını
        kolaylaştırmak için şu operasyonel araçları sağlar:
      </p>
      <ul>
        <li>
          <code>POST /api/v1/compliance/erase</code> — konu bazlı silme
          (tombstone işareti + denetim kaydı).
        </li>
        <li>
          <code>GET /api/v1/events/export</code> — konu bazlı veri çıkarma
          (JSON/CSV).
        </li>
        <li>
          <code>POST /api/v1/policy/retention</code> — saklama penceresinin
          yürütülmesi.
        </li>
      </ul>

      <h2>5. Uluslararası Aktarım</h2>
      <p>
        Veriler varsayılan olarak AWS <code>eu-central-1</code> (Frankfurt)
        bölgesinde işlenir. Türkiye veri yerleşimi isteyen kurumsal müşteriler
        için <code>eu-central-2</code> Zürih veya Türk veri merkezi
        seçenekleri sunulur. AB dışı aktarım yalnızca Standart Sözleşme
        Maddeleri (SCC) ile ve Veri Sorumlusu onayıyla yapılır.
      </p>

      <h2>6. Denetim</h2>
      <p>
        Veri Sorumlusu, makul bildirim sonrasında yılda bir kez Tamga&apos;nın
        güvenlik kontrollerini denetleyebilir. Bağımsız SOC 2 Type II raporu
        NDA altında paylaşılır (Q3 2026&apos;da hazır olacak).
      </p>

      <h2>7. Sözleşme Sonrası Veri</h2>
      <p>
        Sözleşme sona erdiğinde tüm kişisel veri 30 gün içinde silinir veya —
        Veri Sorumlusu talep ederse — şifrelenmiş export paketi olarak
        teslim edilir.
      </p>
    </MarketingDoc>
  );
}
