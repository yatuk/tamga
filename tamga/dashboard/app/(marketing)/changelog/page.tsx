import type { Metadata } from "next";
import { ChangelogContent } from "./ChangelogContent";

export const metadata: Metadata = {
  title: "Changelog — Tamga",
  description: "Tamga AI Security Proxy sürüm notları ve değişiklik geçmişi.",
};

export default function ChangelogPage() {
  return <ChangelogContent />;
}
