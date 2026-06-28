import { notFound } from "next/navigation";
import type { Metadata } from "next";
import { GUIDES, GUIDE_KINDS } from "../_data/guides";
import { GuideView } from "../_components/GuideView";
import type { WebhookKind } from "@/lib/api";

// Statically generate a page per known provider. Unknown kinds fall
// through to notFound().
export function generateStaticParams() {
  return GUIDE_KINDS.map((kind) => ({ kind }));
}

export async function generateMetadata({
  params,
}: {
  params: Promise<{ kind: string }>;
}): Promise<Metadata> {
  const { kind } = await params;
  const guide = GUIDES[kind as WebhookKind];
  if (!guide) return { title: "Integration not found · Tamga" };
  return {
    title: `${guide.name} setup · Tamga integrations`,
    description: guide.overview,
  };
}

export default async function IntegrationGuidePage({
  params,
}: {
  params: Promise<{ kind: string }>;
}) {
  const { kind } = await params;
  const guide = GUIDES[kind as WebhookKind];
  if (!guide) notFound();
  return <GuideView guide={guide} />;
}
