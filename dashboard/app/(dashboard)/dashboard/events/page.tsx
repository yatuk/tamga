import type { Metadata } from "next";
import { EventsView } from "./events-view";

export const metadata: Metadata = { title: "Event explorer" };

export default function EventsPage() {
  return <EventsView />;
}
