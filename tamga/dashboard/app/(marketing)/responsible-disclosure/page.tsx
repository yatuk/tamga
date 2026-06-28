import Link from "next/link";
import { MarketingDoc } from "@/app/(marketing)/_components/marketing/MarketingDoc";

export const metadata = {
  title: "Responsible Disclosure — Tamga",
  description:
    "Tamga güvenlik açığı sorumlu ifşa (responsible disclosure) programı ve iletişim kanalları.",
};

export default function DisclosurePage() {
  return (
    <MarketingDoc
      eyebrow="SECURITY // RESPONSIBLE DISCLOSURE"
      title="Güvenlik Açığı İfşa Programı"
      lastUpdated="17 Nisan 2026"
      intro={
        <p>
          Güvenlik araştırmacılarını değerli ortaklarımız olarak görüyoruz.
          Bu sayfa, Tamga hizmetinde bir güvenlik açığı tespit ettiğinizde
          izlemenizi istediğimiz süreci açıklar.
        </p>
      }
    >
      <h2>Kapsam</h2>
      <ul>
        <li>
          <code>*.tamga.dev</code> üzerindeki marketing ve uygulama alan
          adları
        </li>
        <li>
          Tamga proxy self-host ikili dosyası (latest stable tag)
        </li>
        <li>
          Helm chart (<code>deploy/helm/tamga-proxy</code>) ve Terraform
          modülü
        </li>
      </ul>

      <h2>Kapsam dışı</h2>
      <ul>
        <li>Test kullanıcılarına ait ortamlar (<code>*.staging.tamga.dev</code>)</li>
        <li>Sosyal mühendislik saldırıları</li>
        <li>DoS/DDoS testleri (ayrı yazılı izin gereklidir)</li>
        <li>Üçüncü taraf SaaS sağlayıcılarının açıkları (Clerk, AWS vb.)</li>
      </ul>

      <h2>Raporlama</h2>
      <ol>
        <li>
          E-posta:{" "}
          <a href="mailto:security@tamga.dev">security@tamga.dev</a>
        </li>
        <li>
          PGP anahtarı:{" "}
          <a href="/security/pgp.asc">security/pgp.asc</a> (fingerprint{" "}
          <code>B21F 4C2A · D3E1 AA01 · 65B8 77AA</code>)
        </li>
        <li>
          HackerOne özel programı: davet bazlı (davet talebi için e-posta
          atın).
        </li>
      </ol>

      <h2>Taahhütlerimiz</h2>
      <ul>
        <li>72 saat içinde ilk yanıt veririz.</li>
        <li>
          Rapor sahibini suçlamayız veya yasal yola başvurmayız (iyi niyet
          şartına uyulduğu sürece).
        </li>
        <li>
          İlk public advisory&apos;de araştırmacı ismini (izniyle) kredi
          listesine ekleriz.
        </li>
        <li>
          Kritik bulgularda bug bounty ödüllendirmesi uygulanır (kapsam ve
          etkiye göre 250 USD — 10.000 USD).
        </li>
      </ul>

      <h2>Sizin taahhütleriniz</h2>
      <ul>
        <li>Veri exfiltration yapmayın; sadece minimum düzeyde kanıt toplayın.</li>
        <li>Diğer kullanıcıların verisine erişmeyin; sorunu doğrular doğrulamaz durun.</li>
        <li>
          Tamga kamuya açıklama yapana kadar bulguları paylaşmayın (varsayılan
          embargo: 90 gün).
        </li>
      </ul>

      <h2>Güvenlik kontrolleri</h2>
      <p>
        SOC 2 yol haritamız, ISO 27001 hedefimiz ve penetrasyon test
        bulgularımız{" "}
        <Link href="/trust/security">Güvenlik Sayfasında</Link> detaylıdır.
      </p>
    </MarketingDoc>
  );
}
