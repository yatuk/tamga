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
            "group toast rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 text-zinc-900 dark:text-zinc-100",
          title: "text-[11px] font-semibold",
          description: "group-[.toast]:text-zinc-500 dark:text-zinc-400 text-[10px]",
          actionButton:
            "group-[.toast]:rounded-sm group-[.toast]:bg-red-600 group-[.toast]:text-white group-[.toast]:text-[10px] group-[.toast]:px-3 group-[.toast]:py-1",
          cancelButton:
            "group-[.toast]:rounded-sm group-[.toast]:bg-zinc-800 group-[.toast]:text-zinc-300 group-[.toast]:text-[10px] group-[.toast]:px-3 group-[.toast]:py-1",
          success:
            "!border-emerald-500/20 !bg-emerald-500/[0.04] [&_svg]:text-emerald-400",
          error:
            "!border-red-500/20 !bg-red-500/[0.04] [&_svg]:text-red-400",
          warning:
            "!border-amber-500/20 !bg-amber-500/[0.04] [&_svg]:text-amber-400",
          info: "!border-blue-500/20 !bg-blue-500/[0.04] [&_svg]:text-blue-400",
        },
      }}
      {...props}
    />
  );
};

export { Toaster };
