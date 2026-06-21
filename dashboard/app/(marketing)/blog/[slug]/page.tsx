import type { Metadata } from "next";
import Link from "next/link";
import { notFound } from "next/navigation";
import { ArrowLeft, Clock, Tag } from "lucide-react";
import { MarketingFooter } from "@/app/(marketing)/_components/landing/MarketingFooter";
import { getAllPosts, getAllSlugs, getPostBySlug, formatPostDate } from "@/lib/blog-data";

export function generateStaticParams() {
  return getAllSlugs().map((slug) => ({ slug }));
}

export async function generateMetadata({
  params,
}: {
  params: Promise<{ slug: string }>;
}): Promise<Metadata> {
  const { slug } = await params;
  const post = getPostBySlug(slug);
  if (!post) return { title: "Not found — Tamga Blog" };
  return {
    title: `${post.title} — Tamga Blog`,
    description: post.excerpt,
  };
}

function renderBody(body: string) {
  return body.split("\n").map((line, i) => {
    // Code block
    if (line.startsWith("```")) {
      return null; // skip code fence lines
    }
    if (line.trim().startsWith("`") && line.trim().endsWith("`")) {
      return (
        <pre
          key={i}
          className="my-3 overflow-x-auto rounded-sm border border-zinc-200 dark:border-zinc-800 bg-zinc-100 dark:bg-zinc-900 p-3 font-mono text-xs text-zinc-700 dark:text-zinc-300"
        >
          {line.trim().replace(/^`|`$/g, "")}
        </pre>
      );
    }
    // h2
    if (line.startsWith("## ")) {
      return (
        <h2
          key={i}
          className="mt-10 mb-3 text-2xl font-bold tracking-tight text-white first:mt-0"
        >
          {line.replace("## ", "")}
        </h2>
      );
    }
    // h3
    if (line.startsWith("### ")) {
      return (
        <h3
          key={i}
          className="mt-8 mb-2 text-lg font-semibold tracking-tight text-zinc-100"
        >
          {line.replace("### ", "")}
        </h3>
      );
    }
    // Bold text
    if (line.startsWith("**") && line.includes(":**")) {
      const [label, ...rest] = line.replace(/\*\*/g, "").split(":**");
      return (
        <p key={i} className="my-2 text-sm leading-7 text-zinc-700 dark:text-zinc-300">
          <strong className="text-zinc-900 dark:text-zinc-100">{label}:</strong>
          {rest.join(":**")}
        </p>
      );
    }
    // Ordered list
    if (/^\d+\.\s/.test(line)) {
      return (
        <li
          key={i}
          className="ml-5 list-decimal text-sm leading-7 text-zinc-700 dark:text-zinc-300 marker:text-zinc-500"
        >
          {line.replace(/^\d+\.\s/, "")}
        </li>
      );
    }
    // Unordered list
    if (line.startsWith("- ")) {
      return (
        <li
          key={i}
          className="ml-5 list-disc text-sm leading-7 text-zinc-700 dark:text-zinc-300 marker:text-red-400"
        >
          {line.replace("- ", "")}
        </li>
      );
    }
    // Empty line
    if (line.trim() === "") {
      return <div key={i} className="h-2" />;
    }
    // Regular paragraph
    return (
      <p key={i} className="my-2 text-sm leading-7 text-zinc-700 dark:text-zinc-300">
        {line}
      </p>
    );
  });
}

export default async function BlogPostPage({
  params,
}: {
  params: Promise<{ slug: string }>;
}) {
  const { slug } = await params;
  const post = getPostBySlug(slug);
  if (!post) notFound();

  const allPosts = getAllPosts();
  const currentIndex = allPosts.findIndex((p) => p.slug === slug);
  const prev = currentIndex < allPosts.length - 1 ? allPosts[currentIndex + 1] : null;
  const next = currentIndex > 0 ? allPosts[currentIndex - 1] : null;

  return (
    <>
      <main className="mx-auto w-full max-w-3xl px-6 py-16 text-zinc-800 dark:text-zinc-200">
        {/* Back link */}
        <Link
          href="/blog"
          className="inline-flex items-center gap-1.5 font-mono text-xs text-zinc-500 dark:text-zinc-400 hover:text-zinc-200 transition-colors"
        >
          <ArrowLeft className="h-3 w-3" aria-hidden />
          All posts
        </Link>

        {/* Header */}
        <p className="mt-6 font-mono text-[11px] uppercase tracking-[0.2em] text-red-400">
          {formatPostDate(post.date)}
        </p>
        <h1 className="mt-2 text-3xl font-extrabold tracking-tight text-white sm:text-4xl">
          {post.title}
        </h1>

        {/* Meta */}
        <div className="mt-4 flex flex-wrap items-center gap-4 text-xs">
          <span className="font-mono text-zinc-500 dark:text-zinc-400">
            {post.author}
          </span>
          <span className="flex items-center gap-1 font-mono text-zinc-500 dark:text-zinc-400">
            <Clock className="h-3 w-3" aria-hidden />
            {post.readTimeMin} dk
          </span>
          <span className="flex flex-wrap gap-1.5">
            {post.tags.map((t) => (
              <span
                key={t}
                className="inline-flex items-center gap-1 rounded-sm border border-zinc-200 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 px-2 py-0.5 font-mono text-[10px] uppercase tracking-wider text-zinc-600 dark:text-zinc-400"
              >
                <Tag className="h-2.5 w-2.5" aria-hidden />
                {t}
              </span>
            ))}
          </span>
        </div>

        {/* Body */}
        <div className="mt-8">{renderBody(post.body)}</div>

        {/* Prev / Next */}
        <div className="mt-16 grid gap-4 border-t border-zinc-200 dark:border-zinc-800 pt-8 sm:grid-cols-2">
          {prev ? (
            <Link
              href={`/blog/${prev.slug}`}
              className="group rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950/60 p-4 transition-colors hover:border-red-500/30"
            >
              <span className="font-mono text-[10px] uppercase tracking-wider text-zinc-500">
                ← Previous
              </span>
              <p className="mt-1 text-sm font-medium text-zinc-700 dark:text-zinc-300 group-hover:text-white truncate">
                {prev.title}
              </p>
            </Link>
          ) : (
            <div />
          )}
          {next ? (
            <Link
              href={`/blog/${next.slug}`}
              className="group rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950/60 p-4 text-right transition-colors hover:border-red-500/30"
            >
              <span className="font-mono text-[10px] uppercase tracking-wider text-zinc-500">
                Next →
              </span>
              <p className="mt-1 text-sm font-medium text-zinc-700 dark:text-zinc-300 group-hover:text-white truncate">
                {next.title}
              </p>
            </Link>
          ) : (
            <div />
          )}
        </div>
      </main>
      <MarketingFooter />
    </>
  );
}
