/** @type {import('tailwindcss').Config} */
export default {
  content: ["./index.html", "./src/**/*.{js,ts,jsx,tsx}"],
  darkMode: "class",
  theme: {
    extend: {
      colors: {
        // Surface palette — both light and dark variants are driven from
        // CSS vars defined in index.css (light = :root, dark = .dark).
        // Tailwind classes like `bg-surface-2` resolve to the active
        // variant automatically. Existing class names kept; only the
        // hex values they map to change with the theme.
        surface: {
          0: "var(--surface-0)",
          1: "var(--surface-1)",
          2: "var(--surface-2)",
          3: "var(--surface-3)",
          4: "var(--surface-4)",
          5: "var(--surface-5)",
        },
        border: {
          DEFAULT: "var(--border)",
          light: "var(--border-light)",
        },
        // Galaxy brand accent — Xanh Ngọc (#14B795) replaces the
        // template's indigo #6366f1. Lifted to #4fd9bc on hover for
        // legibility on dark surfaces; on light surfaces stays at
        // DEFAULT for contrast. Primary brand Xanh Đậm (#1E398D) is
        // reserved for one-off CTAs / brand chrome via brand.* below.
        accent: {
          DEFAULT: "var(--accent)",
          hover: "var(--accent-hover)",
          muted: "var(--accent-muted)",
        },
        brand: {
          DEFAULT: "#1E398D",
          light: "#5a7fd9",
          teal: "#14B795",
        },
        // Foreground text tokens for components that switched away from
        // hard-coded `text-gray-*` classes during the light-theme work.
        fg: {
          DEFAULT: "var(--fg)",
          muted: "var(--fg-muted)",
          subtle: "var(--fg-subtle)",
        },
      },
      fontFamily: {
        sans: ["Inter", "-apple-system", "BlinkMacSystemFont", "Segoe UI", "sans-serif"],
        mono: ["JetBrains Mono", "Fira Code", "Consolas", "monospace"],
      },
      animation: {
        "pulse-slow": "pulse 3s cubic-bezier(0.4, 0, 0.6, 1) infinite",
        "fade-in": "fadeIn 0.3s ease-out",
        "slide-up": "slideUp 0.3s ease-out",
      },
      keyframes: {
        fadeIn: {
          "0%": { opacity: "0" },
          "100%": { opacity: "1" },
        },
        slideUp: {
          "0%": { opacity: "0", transform: "translateY(8px)" },
          "100%": { opacity: "1", transform: "translateY(0)" },
        },
      },
    },
  },
  plugins: [],
};
