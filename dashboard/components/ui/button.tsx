import * as React from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/utils";

const buttonVariants = cva(
  "inline-flex items-center justify-center gap-1.5 rounded-sm text-[11px] font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-50 cursor-pointer",
  {
    variants: {
      variant: {
        default: "bg-fg text-surface-base hover:opacity-90",
        destructive: "bg-status-critical text-accent-foreground hover:opacity-90",
        outline:
          "border border-border bg-surface-card text-fg hover:bg-surface-subtle",
        secondary:
          "border border-border bg-surface-card text-fg hover:bg-surface-subtle",
        ghost: "text-fg-muted hover:bg-surface-subtle hover:text-fg",
        link: "text-fg-muted underline-offset-4 hover:underline",
        accent:
          "border border-status-warn/40 bg-status-warn-bg text-status-warn hover:bg-status-warn/20",
      },
      size: {
        sm: "h-7 px-2 text-[10px]",
        md: "h-8 px-3",
        lg: "h-10 px-4 text-sm",
        icon: "h-7 w-7 p-0",
        "icon-sm": "h-6 w-6 p-0",
      },
    },
    defaultVariants: {
      variant: "outline",
      size: "md",
    },
  },
);

export interface ButtonProps
  extends React.ButtonHTMLAttributes<HTMLButtonElement>,
    VariantProps<typeof buttonVariants> {
  asChild?: boolean;
}

function Button({ className, variant, size, ...props }: ButtonProps) {
  return (
    <button
      className={cn(buttonVariants({ variant, size }), className)}
      {...props}
    />
  );
}

export { Button, buttonVariants };
