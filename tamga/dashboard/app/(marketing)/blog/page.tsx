import type { Metadata } from "next";
import Link from "next/link";
import { ArrowRight, Clock, Tag } from "lucide-react";
import { MarketingFooter } from "@/app/(marketing)/_components/landing/MarketingFooter";
import { getAllPosts, formatPostDate } from "@/lib/blog-data";

export const metadata: Metadata = {
  title: "Blog — Tamga",
  description:
    "LLM güvenliği, PII tespiti, KVKK uyumluluğu ve inline proxy mühendisliği üzerine Tamga mühendislik ekibinden notlar.",
};

export default function BlogPage() {
  const posts = getAllPosts();

  return (
    <>
      <main className="mx-auto w-full max-w-4xl px-6 py-16 text-zinc-800 dark:text-zinc-200">
        <p className="font-mono text-[11px] uppercase tracking-[0.2em] text-red-400">
          Engineering Blog
        </p>
        <h1 className="mt-2 text-4xl font-extrabold tracking-tight text-white">
          LLM Security Engineering
        </h1>
        <p className="mt-4 max-w-2xl text-sm leading-7 text-zinc-600 dark:text-zinc-400">
          Tamga mühendislik ekibinin LLM güvenliği, PII tespiti, KVKK uyumluluğu
          ve inline proxy mimarisi üzerine yazdığı teknik notlar. Her yazı
          açık kaynak kod tabanındaki bir karara veya bir tasarım ortağı
          dağıtımından alınan bir saha notuna bağlanır.
        </p>

        <div className="mt-10 space-y-6">
          {posts.map((post) => (
            <Link
              key={post.slug}
              href={`/blog/${post.slug}`}
              className="group block rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950/60 p-6 transition-colors hover:border-red-500/30 hover:bg-zinc-100 dark:hover:bg-zinc-900/60"
            >
              <article>
                <div className="flex flex-wrap items-center gap-3 text-xs">
                  <span className="font-mono text-zinc-500 dark:text-zinc-400">
                    {formatPostDate(post.date)}
                  </span>
                  <span className="flex items-center gap-1 font-mono text-zinc-500 dark:text-zinc-400">
                    <Clock className="h-3 w-3" aria-hidden />
                    {post.readTimeMin} dk
                  </span>
                  {post.tags.map((t) => (
                    <span
                      key={t}
                      className="inline-flex items-center gap-1 rounded-sm border border-zinc-200 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 px-2 py-0.5 font-mono text-[10px] uppercase tracking-wider text-zinc-600 dark:text-zinc-400"
                    >
                      <Tag className="h-2.5 w-2.5" aria-hidden />
                      {t}
                    </span>
                  ))}
                </div>
                <h2 className="mt-3 text-xl font-semibold tracking-tight text-white transition-colors group-hover:text-red-300">
                  {post.title}
                </h2>
                <p className="mt-2 text-sm leading-relaxed text-zinc-600 dark:text-zinc-400">
                  {post.excerpt}
                </p>
                <div className="mt-4 inline-flex items-center gap-1.5 font-mono text-xs text-red-400 transition-transform group-hover:translate-x-0.5">
                  Read more
                  <ArrowRight className="h-3 w-3" aria-hidden />
                </div>
              </article>
            </Link>
          ))}
        </div>

        <div className="mt-12 rounded-sm border border-red-500/30 bg-red-500/5 p-6 text-center">
          <p className="font-mono text-[11px] uppercase tracking-[0.2em] text-red-400">
            Stay updated
          </p>
          <h3 className="mt-2 text-lg font-semibold text-white">
            Yeni yazılardan haberdar olmak ister misiniz?
          </h3>
          <p className="mt-2 text-sm text-zinc-600 dark:text-zinc-400">
            Blog yazılarını GitHub&apos;da yayınlıyoruz. Repo&apos;yu takip
            edin veya RSS/Atom feed için bize yazın.
          </p>
          <div className="mt-4 flex flex-wrap justify-center gap-3">
            <a
              href="https://github.com/tamga-dev/tamga"
              target="_blank"
              rel="noreferrer"
              className="inline-flex items-center gap-2 rounded-sm bg-red-600 px-4 py-2 text-sm font-semibold text-white hover:bg-red-700"
            >
              GitHub&apos;da takip et
              <ArrowRight className="h-4 w-4" aria-hidden />
            </a>
            <Link
              href="/changelog"
              className="inline-flex items-center gap-2 rounded-sm border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-950 px-4 py-2 text-sm text-zinc-800 dark:text-zinc-200 hover:bg-zinc-100 dark:hover:bg-zinc-900"
            >
              Changelog
            </Link>
          </div>
        </div>
      </main>
      <MarketingFooter />
    </>
  );
}
