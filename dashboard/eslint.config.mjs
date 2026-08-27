import { dirname } from "path";
import { fileURLToPath } from "url";
import { FlatCompat } from "@eslint/eslintrc";
import jsxA11y from "eslint-plugin-jsx-a11y";

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);

const compat = new FlatCompat({ baseDirectory: __dirname });

// Ban hardcoded color literals (hex, rgb/hsl/oklch functions) so components
// are forced to use the design tokens in globals.css / tailwind.config.ts.
const noHardcodedColor = {
  meta: {
    type: "problem",
    docs: {
      description: "Disallow hardcoded color literals in favor of design tokens",
    },
    messages: {
      hardcodedColor:
        "Hardcoded color detected — use a design token (surface-*, fg-*, status-*, accent, border) instead.",
    },
  },
  create(context) {
    const COLOR_RE =
      /(#[0-9a-fA-F]{3,8}\b|rgba?\(|hsla?\(|oklch\(|oklab\(|lab\(|lch\(|hwb\(|color\()/;
    function check(node, value) {
      if (typeof value === "string" && COLOR_RE.test(value)) {
        context.report({ node, messageId: "hardcodedColor" });
      }
    }
    return {
      Literal(node) {
        check(node, node.value);
      },
      TemplateElement(node) {
        check(node, node.value?.cooked);
      },
    };
  },
};

const eslintConfig = [
  ...compat.extends("next/core-web-vitals"),
  ...compat.extends("next/typescript"),
  {
    plugins: {
      "jsx-a11y": jsxA11y,
      tamga: { rules: { "no-hardcoded-color": noHardcodedColor } },
    },
    rules: {
      "@typescript-eslint/no-unused-vars": ["warn", {
        argsIgnorePattern: "^_",
        varsIgnorePattern: "^_",
      }],
      "@typescript-eslint/no-explicit-any": "warn",
      "react-hooks/exhaustive-deps": "warn",
      "react/no-unescaped-entities": "off",
      "tamga/no-hardcoded-color": "warn",
      "jsx-a11y/alt-text": "warn",
      "jsx-a11y/aria-props": "warn",
      "jsx-a11y/aria-proptypes": "warn",
      "jsx-a11y/click-events-have-key-events": "warn",
      "jsx-a11y/no-static-element-interactions": "warn",
      "jsx-a11y/iframe-has-title": "warn",
      "jsx-a11y/label-has-associated-control": "warn",
    },
  },
];

export default eslintConfig;
