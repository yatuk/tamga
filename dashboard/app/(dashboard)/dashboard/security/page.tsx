import type { Metadata } from "next";
import { Incidents } from "./incidents";

export const metadata: Metadata = { title: "Incidents" };

export default function IncidentsPage() {
  return <Incidents />;
}
