import Link from "next/link";
import { MarketingDoc } from "@/app/(marketing)/_components/marketing/MarketingDoc";

export const metadata = {
  title: "Hizmet Şartları — Tamga",
  description:
    "Tamga AI Güvenlik Proxy'sinin kullanımını düzenleyen Hizmet Şartları.",
};

export default function TermsPage() {
  return (
    <MarketingDoc
      eyebrow="LEGAL // TERMS"
      title="Hizmet Şartları"
      lastUpdated="17 Nisan 2026"
      intro={
        <p>
          Tamga hizmetini kullanarak aşağıdaki şartları kabul etmiş olursunuz.
          Sözleşmenin tam, imzalanmış kurumsal sürümü için{" "}
          <a href="mailto:legal@tamga.dev">legal@tamga.dev</a> adresine
          başvurabilirsiniz.
        </p>
      }
    >
      <h2>1. Hizmet Tanımı</h2>
      <p>
        Tamga, LLM API çağrılarınızı güvenlik açısından tarayan, PII/PCI
        maskeleyen, prompt injection saldırılarını engelleyen ve denetim izi
        üreten bir ters-proxy hizmetidir. Hizmet, SaaS veya self-hosted
        (Helm/Terraform) modelleriyle sunulabilir.
      </p>

      <h2>2. Hesap ve Güvenlik</h2>
      <ul>
        <li>Hesap kimlik bilgilerinizin gizliliğinden siz sorumlusunuz.</li>
        <li>API anahtarlarını asla istemci tarafı kod&apos;a gömmeyin.</li>
        <li>
          Şüpheli erişim tespit ederseniz{" "}
          <a href="mailto:security@tamga.dev">security@tamga.dev</a> adresine
          bildirin.
        </li>
      </ul>

      <h2>3. Kabul Edilebilir Kullanım</h2>
      <p>
        Aşağıdaki faaliyetler yasaktır:
      </p>
      <ul>
        <li>Hizmeti yasa dışı, zararlı veya aldatıcı amaçlarla kullanmak.</li>
        <li>Hizmetin kapasitesini aşan otomatik yük uygulamak (rate-limit bypass denemesi).</li>
        <li>Tersine mühendislik, güvenlik açığı istismarı (bug bounty dışında).</li>
        <li>Başka bir müşterinin verisine erişmeye çalışmak.</li>
      </ul>

      <h2>4. Fiyatlandırma ve Ödeme</h2>
      <p>
        Planlara özel fiyatlar{" "}
        <Link href="/pricing">fiyatlandırma sayfasında</Link> yayınlanır. Kurumsal
        planlarda yıllık fatura kesilir; KDV ilgili yerel mevzuata göre
        eklenir.
      </p>

      <h2>5. Hizmet Seviyesi (SLA)</h2>
      <ul>
        <li>Business planda %99.9 aylık uptime (proxy API).</li>
        <li>Enterprise planda %99.95 + P1 için 30 dk müdahale taahhüdü.</li>
        <li>
          SLA hesaplama ayrıntıları ve kredi süreci{" "}
          <Link href="/trust/security">güvenlik sayfasında</Link>.
        </li>
      </ul>

      <h2>6. Veri Koruma</h2>
      <p>
        Veri işleme{" "}
        <Link href="/dpa">Veri İşleme Sözleşmesi (DPA)</Link> ve{" "}
        <Link href="/privacy">Gizlilik Politikası</Link> ile yönetilir.
      </p>

      <h2>7. Fikri Mülkiyet</h2>
      <p>
        Tamga&apos;nın yazılımı, logosu, dokümantasyonu Tamga A.Ş.&apos;ye
        aittir. Müşterinin prompt ve yanıt içeriği müşteriye aittir;
        Tamga bu veriler üzerinde yalnızca hizmeti sunmak için sınırlı
        kullanım hakkına sahiptir.
      </p>

      <h2>8. Sorumluluk Sınırı</h2>
      <p>
        Yasaların izin verdiği azami ölçüde, Tamga&apos;nın sorumluluğu son
        12 aylık hizmet bedeliyle sınırlıdır. Tamga, ağır ihmal hali dışında
        dolaylı veya sonuç zararlarından sorumlu değildir.
      </p>

      <h2>9. Fesih</h2>
      <p>
        Her iki taraf 30 gün önceden yazılı bildirimle sözleşmeyi
        feshedebilir. Fesih halinde Tamga 30 gün içinde kişisel veri siler
        veya <Link href="/dpa">DPA</Link> uyarınca geri iade eder.
      </p>

      <h2>10. Uygulanacak Hukuk</h2>
      <p>
        Bu sözleşme Türkiye Cumhuriyeti hukukuna tabidir. İstanbul Merkez
        (Çağlayan) Mahkemeleri ve İcra Daireleri yetkilidir.
      </p>
    </MarketingDoc>
  );
}
