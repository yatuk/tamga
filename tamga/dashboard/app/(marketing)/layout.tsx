import { CookieBanner } from "@/app/(marketing)/_components/marketing/CookieBanner";
import { MarketingNav } from "@/app/(marketing)/_components/marketing/MarketingNav";
import { I18nProvider } from "@/lib/i18n";

export default function MarketingLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <I18nProvider>
      <div className="min-h-screen bg-white dark:bg-zinc-950 text-zinc-900 dark:text-zinc-100 transition-[background-color,color] duration-200">
        <MarketingNav />
        {children}
        <CookieBanner />
      </div>
    </I18nProvider>
  );
}
