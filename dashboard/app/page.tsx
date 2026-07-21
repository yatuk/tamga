import { redirect } from "next/navigation";

// The marketing site moved to https://tamgaproxy.com — this app now only
// serves the dashboard, so the root path goes straight there.
export default function RootPage() {
  redirect("/dashboard");
}
