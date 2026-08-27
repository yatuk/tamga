"use client";

import {
  CheckCircle2,
  Info,
  LoaderCircle,
  OctagonX,
  TriangleAlert,
} from "lucide-react";
import { useTheme } from "next-themes";
import { Toaster as Sonner } from "sonner";

type ToasterProps = React.ComponentProps<typeof Sonner>;

const Toaster = ({ ...props }: ToasterProps) => {
  const { theme = "system" } = useTheme();

  return (
    <Sonner
      theme={theme as ToasterProps["theme"]}
      className="toaster group"
      icons={{
        success: <CheckCircle2 className="h-4 w-4" />,
        info: <Info className="h-4 w-4" />,
        warning: <TriangleAlert className="h-4 w-4" />,
        error: <OctagonX className="h-4 w-4" />,
        loading: <LoaderCircle className="h-4 w-4 animate-spin" />,
      }}
      toastOptions={{
        classNames: {
          toast:
            "group toast rounded-sm border border-border bg-surface-card text-fg",
          title: "text-[11px] font-semibold",
          description: "group-[.toast]:text-fg-muted text-[10px]",
          actionButton:
            "group-[.toast]:rounded-sm group-[.toast]:bg-status-critical group-[.toast]:text-accent-foreground group-[.toast]:text-[10px] group-[.toast]:px-3 group-[.toast]:py-1",
          cancelButton:
            "group-[.toast]:rounded-sm group-[.toast]:bg-surface-subtle group-[.toast]:text-fg-muted group-[.toast]:text-[10px] group-[.toast]:px-3 group-[.toast]:py-1",
          success:
            "!border-status-pass/30 !bg-status-pass-bg [&_svg]:text-status-pass",
          error:
            "!border-status-critical/30 !bg-status-critical-bg [&_svg]:text-status-critical",
          warning:
            "!border-status-warn/30 !bg-status-warn-bg [&_svg]:text-status-warn",
          info: "!border-status-low/30 !bg-status-low-bg [&_svg]:text-status-low",
        },
      }}
      {...props}
    />
  );
};

export { Toaster };
