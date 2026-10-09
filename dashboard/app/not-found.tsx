import type { Metadata } from "next";
import { NoAccounts } from "@/components/console/auth";

export const metadata: Metadata = { title: "Page not found" };

export default function NotFound() {
  return <NoAccounts title="There is no page here">The address does not match anything Tamga serves.</NoAccounts>;
}
