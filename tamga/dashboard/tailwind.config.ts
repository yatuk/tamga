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
        sm: "0.125rem",
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
          critical: "var(--status-critical)",
          high: "var(--status-high)",
          medium: "var(--status-medium)",
          low: "var(--status-low)",
          pass: "var(--status-pass)",
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
