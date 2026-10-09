import type { Metadata } from "next";
import { Overview } from "./overview";

export const metadata: Metadata = { title: "Overview" };

export default function DashboardPage() {
  return <Overview />;
}
