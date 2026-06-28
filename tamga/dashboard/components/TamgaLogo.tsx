import Image from "next/image";
import { cn } from "@/lib/utils";

type TamgaLogoProps = {
  /** Display edge length in px (source asset is square). */
  size?: number;
  className?: string;
  /** White tray + ring for dark UIs (e.g. sidebar). */
  contained?: boolean;
  priority?: boolean;
};

export function TamgaLogo({ size = 32, className, contained = false, priority = false }: TamgaLogoProps) {
  const image = (
    <Image
      src="/tamga-logo.png"
      alt="Tamga"
      width={512}
      height={512}
      priority={priority}
      className="object-contain"
      style={{ width: size, height: size }}
      sizes={`${size}px`}
    />
  );

  if (!contained) {
    return (
      <span className={cn("inline-flex shrink-0", className)} style={{ width: size, height: size }}>
        {image}
      </span>
    );
  }

  return (
    <span
      className={cn(
        "inline-flex shrink-0 items-center justify-center rounded-md bg-white p-1 shadow-sm ring-1 ring-slate-200 dark:ring-slate-600",
        className
      )}
    >
      {image}
    </span>
  );
}
