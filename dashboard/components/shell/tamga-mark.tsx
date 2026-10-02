import { cn } from "@/lib/utils";

/**
 * The Tamga mark: a T-shaped inspection gate over a downward seal. The body
 * follows the text color so it works on either theme; the bars stay brand red.
 */
export function TamgaMark({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 64 64" role="img" aria-label="Tamga" className={cn("size-7 shrink-0", className)}>
      <path
        fill="currentColor"
        fillRule="evenodd"
        d="M6 8h52v18H43v19.5L32 57 21 45.5V26H6V8Zm8 8v4h15v22.3l3 3.2 3-3.2V20h15v-4H14Z"
      />
      <g className="fill-brand">
        <rect x="29" y="20" width="6" height="24" />
        <rect x="6" y="31" width="8" height="5" />
        <rect x="50" y="31" width="8" height="5" />
      </g>
    </svg>
  );
}
