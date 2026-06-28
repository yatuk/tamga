import Link from "next/link";
import { MarketingDoc } from "@/app/(marketing)/_components/marketing/MarketingDoc";

export const metadata = {
  title: "Subprocessors — Tamga",
  description:
    "Tamga AI Güvenlik Proxy'sinin kullandığı alt işleyici (subprocessor) listesi ve her birinin veri kapsamı.",
};

const rows: Array<{
  vendor: string;
  purpose: string;
  region: string;
  data: string;
  contract: string;
}> = [
  {
    vendor: "Amazon Web Services (EKS, RDS, S3)",
    purpose: "Barındırma ve nesne depolama",
    region: "eu-central-1 (Frankfurt) / opsiyonel eu-central-2",
    data: "Tüm trafik, log, şifreli yedek",
    contract: "DPA + SCC",
  },
  {
    vendor: "Clerk",
    purpose: "SSO + kullanıcı kimlik yönetimi",
    region: "us-east-1 / eu-west-1",
    data: "Email, ad, görsel, rol",
    contract: "DPA + SCC",
  },
  {
    vendor: "Neon / Supabase (opsiyonel)",
    purpose: "Yönetilen Postgres (denetim, olay, politika)",
    region: "Müşteri seçimine göre EU",
    data: "Denetim kaydı, olay durumları",
    contract: "DPA + SCC",
  },
  {
    vendor: "OpenTelemetry Collector",
    purpose: "Dağıtık iz ve metrik toplama",
    region: "Müşteri self-host veya EU",
    data: "Span metadatası, metrik sayaçları (PII içermez)",
    contract: "Self-host",
  },
  {
    vendor: "Cloudflare",
    purpose: "CDN + edge WAF (yalnızca marketing sitesi)",
    region: "Global anycast",
    data: "Genel erişim log'u",
    contract: "DPA",
  },
  {
    vendor: "Resend",
    purpose: "Sözleşme ve bildirim e-postaları",
    region: "eu-west-1",
    data: "Email, içerik başlığı",
    contract: "DPA",
  },
];

export default function SubprocessorsPage() {
  return (
    <MarketingDoc
      eyebrow="LEGAL // SUBPROCESSORS"
      title="Alt İşleyici Listesi"
      lastUpdated="17 Nisan 2026"
      intro={
        <p>
          Tamga aşağıdaki alt işleyicilerle (subprocessor) çalışır. Listede
          yapılan her değişiklik en az 30 gün önceden e-posta ile duyurulur;
          müşterilerimizin itiraz hakkı saklıdır. Değişiklik bildirimi için{" "}
          <Link href="/responsible-disclosure">güvenlik kanalımıza</Link> abone
          olabilirsiniz.
        </p>
      }
    >
      <table>
        <thead>
          <tr>
            <th>Sağlayıcı</th>
            <th>Amaç</th>
            <th>Bölge</th>
            <th>Veri</th>
            <th>Sözleşme</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((r) => (
            <tr key={r.vendor}>
              <td>{r.vendor}</td>
              <td>{r.purpose}</td>
              <td>{r.region}</td>
              <td>{r.data}</td>
              <td>{r.contract}</td>
            </tr>
          ))}
        </tbody>
      </table>

      <h2>Self-hosted dağıtım</h2>
      <p>
        KVKK hassasiyeti yüksek kurumlar (banka, sağlık, kamu) için Tamga tam
        self-hosted çalışır; bu durumda Tamga hiçbir alt işleyici
        görevlendirmez ve trafik müşterinin VPC&apos;sinden dışarı çıkmaz.
        Helm ve Terraform şablonları için{" "}
        <Link href="/docs/quickstart">Quickstart</Link> sayfasına bakın.
      </p>
    </MarketingDoc>
  );
}
