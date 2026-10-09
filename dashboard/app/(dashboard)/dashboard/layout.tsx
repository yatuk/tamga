import { TamgaShell } from "@/components/console/shell";

export default function DashboardLayout({ children }: { children: React.ReactNode }) {
  return (
    <div className="tamga">
      <TamgaShell>{children}</TamgaShell>
    </div>
  );
}
