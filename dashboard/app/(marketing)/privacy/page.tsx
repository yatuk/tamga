import Link from "next/link";
import { MarketingDoc } from "@/app/(marketing)/_components/marketing/MarketingDoc";

export const metadata = {
  title: "Gizlilik Politikası — Tamga",
  description:
    "Tamga AI Güvenlik Proxy'sinin KVKK ve GDPR uyumlu Gizlilik Politikası.",
};

export default function PrivacyPage() {
  return (
    <MarketingDoc
      eyebrow="LEGAL // PRIVACY"
      title="Gizlilik Politikası"
      lastUpdated="17 Nisan 2026"
      intro={
        <p>
          Bu politika, Tamga (&quot;biz&quot;) tarafından toplanan, kullanılan
          ve paylaşılan kişisel verileri ve bunların işlenme amaçlarını
          açıklar. KVKK Madde 10 ve GDPR Madde 13-14 kapsamında aydınlatma
          metni olarak da kullanılır.
        </p>
      }
    >
      <h2>1. Topladığımız veriler</h2>
      <ul>
        <li>
          <strong>Hesap bilgileri:</strong> ad, e-posta, organizasyon, rol
          (Clerk üzerinden yönetilir).
        </li>
        <li>
          <strong>Teknik veriler:</strong> IP adresi, user-agent, oturum
          tanımlayıcıları (log&apos;larda 30 gün saklanır).
        </li>
        <li>
          <strong>Ürün telemetri:</strong> proxy istek sayacı, engellenen/
          maskelenen sayacı, politika versiyonu.
        </li>
      </ul>
      <p>
        Tamga proxy&apos;si <strong>müşteri prompt içeriklerini</strong>{" "}
        varsayılan olarak kalıcı olarak saklamaz. Self-host dağıtımlarda tüm
        trafik müşterinin altyapısında kalır; SaaS modelde yalnızca
        anonimleştirilmiş bulgu özetleri (PII maskelenmiş) denetim amacıyla
        loglanır.
      </p>

      <h2>2. Kullanım amaçları</h2>
      <ul>
        <li>Hizmetin sunulması ve sözleşmesel yükümlülüklerin yerine getirilmesi.</li>
        <li>Güvenlik olayı müdahalesi ve suistimal önleme.</li>
        <li>Ürün kalitesini ölçmek ve istatistiksel analiz.</li>
        <li>Yasal yükümlülüklere uyum (vergi, talep yanıtı).</li>
      </ul>

      <h2>3. Hukuki sebep</h2>
      <p>
        Sözleşme (GDPR 6/1/b, KVKK 5/2/c), hukuki yükümlülük (6/1/c, 5/2/a),
        meşru menfaat (6/1/f, 5/2/f). Pazarlama iletileri için ayrı açık rıza
        alınır.
      </p>

      <h2>4. Paylaşım</h2>
      <p>
        Yalnızca <Link href="/subprocessors">alt işleyici listemizdeki</Link>
        hizmet sağlayıcılarla ve yasal zorunluluk halinde yetkili makamlarla
        paylaşılır.
      </p>

      <h2>5. Saklama süresi</h2>
      <ul>
        <li>Hesap verisi: hesap aktif süresince + 30 gün silme penceresi.</li>
        <li>Denetim log&apos;u: 2 yıl (yasal zorunluluk).</li>
        <li>Ürün telemetri: 90 gün anonimleştirilmiş, sonra toplam istatistik.</li>
      </ul>

      <h2>6. Haklarınız</h2>
      <p>
        KVKK Madde 11 ve GDPR 15-22 kapsamındaki haklarınızı{" "}
        <a href="mailto:dpo@tamga.dev">dpo@tamga.dev</a> adresine yazarak
        kullanabilirsiniz. 30 gün içinde yanıt veririz.
      </p>

      <h2>7. İletişim</h2>
      <p>
        Veri sorumlusu: Tamga A.Ş.<br />
        E-posta: <a href="mailto:privacy@tamga.dev">privacy@tamga.dev</a>
        <br />
        DPO: <a href="mailto:dpo@tamga.dev">dpo@tamga.dev</a>
      </p>
    </MarketingDoc>
  );
}
