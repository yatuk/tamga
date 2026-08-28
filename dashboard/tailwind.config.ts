/** @type {import('tailwindcss').Config} */
module.exports = {
  content: [
    "./app/**/*.{js,ts,jsx,tsx,mdx}",
    "./components/**/*.{js,ts,jsx,tsx,mdx}",
    "./lib/**/*.{js,ts,jsx,tsx,mdx}",
  ],
  darkMode: "class",
  theme: {
    extend: {
      borderRadius: {
        sm: "0.75rem",
        md: "0.875rem",
        lg: "1rem",
      },
      boxShadow: {
        elevated:
          "inset 0 1px 0 0 oklch(1 0 0 / 0.04), 0 20px 40px -20px oklch(0 0 0 / 0.6), 0 8px 16px -8px oklch(0 0 0 / 0.35)",
        "inner-hi": "inset 0 1px 0 0 oklch(1 0 0 / 0.04)",
      },
      colors: {
        // Semantic surface tokens — prefer these for new components.
        surface: {
          DEFAULT: "var(--surface-base)",
          subtle: "var(--surface-subtle)",
          card: "var(--surface-card)",
          elevated: "var(--surface-elevated)",
          overlay: "var(--surface-overlay)",
        },
        fg: {
          DEFAULT: "var(--fg)",
          muted: "var(--fg-muted)",
          subtle: "var(--fg-subtle)",
          faint: "var(--fg-faint)",
        },
        ring: "var(--ring)",
        accent: {
          DEFAULT: "var(--accent)",
          hover: "var(--accent-hover)",
          foreground: "var(--accent-foreground)",
        },

        // Legacy aliases kept so existing components continue to render
        // correctly while we progressively migrate.
        border: "var(--border)",
        "border-strong": "var(--border-strong)",
        "border-subtle": "var(--border-subtle)",
        background: "var(--surface-base)",
        foreground: "var(--fg)",
        card: {
          DEFAULT: "var(--surface-card)",
          foreground: "var(--fg)",
        },
        muted: {
          DEFAULT: "var(--surface-subtle)",
          foreground: "var(--fg-subtle)",
        },

        status: {
          // DEFAULT uses `oklch(var(--x-ch) / <alpha-value>)` so opacity
          // modifiers (`bg-status-critical/10`, `border-status-high/40`) work.
          // A bare `var(--status-*)` cannot take an opacity suffix — Tailwind
          // silently drops the rule — so the raw channels are supplied via
          // `--status-*-ch` in globals.css.
          critical: {
            DEFAULT: "oklch(var(--status-critical-ch) / <alpha-value>)",
            bg: "var(--status-critical-bg)",
          },
          high: {
            DEFAULT: "oklch(var(--status-high-ch) / <alpha-value>)",
            bg: "var(--status-high-bg)",
          },
          medium: {
            DEFAULT: "oklch(var(--status-medium-ch) / <alpha-value>)",
            bg: "var(--status-medium-bg)",
          },
          low: {
            DEFAULT: "oklch(var(--status-low-ch) / <alpha-value>)",
            bg: "var(--status-low-bg)",
          },
          pass: {
            DEFAULT: "oklch(var(--status-pass-ch) / <alpha-value>)",
            bg: "var(--status-pass-bg)",
          },
          block: {
            DEFAULT: "oklch(var(--status-block-ch) / <alpha-value>)",
            bg: "var(--status-block-bg)",
          },
          redact: {
            DEFAULT: "oklch(var(--status-redact-ch) / <alpha-value>)",
            bg: "var(--status-redact-bg)",
          },
          warn: {
            DEFAULT: "oklch(var(--status-warn-ch) / <alpha-value>)",
            bg: "var(--status-warn-bg)",
          },
        },

        // Retained for any lingering `bg-security-*` usages.
        security: {
          critical: "var(--status-critical)",
          high: "var(--status-high)",
          medium: "var(--status-medium)",
          low: "var(--status-low)",
          pass: "var(--status-pass)",
          bg: "var(--surface-base)",
          panel: "var(--surface-card)",
          border: "var(--border)",
        },
      },
    },
  },
  plugins: [require("@tailwindcss/forms")],
};
