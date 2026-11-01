// Flat ESLint config (ESLint 9) for the horos TS core. Mirrors the Rust lane's
// fmt+clippy bar (horos-internal#21): a non-pedantic lint gate that holds the line on obvious
// quality issues (unused vars, debugger, etc.) without churning the byte-for-byte
// port in src/index.ts. Formatting is owned by Prettier (.prettierrc) and checked
// separately via `prettier --check` in the `lint` script — ESLint here is
// correctness/hygiene only, not style.
import js from "@eslint/js";
import tsParser from "@typescript-eslint/parser";
import tsPlugin from "@typescript-eslint/eslint-plugin";

export default [
  // Never lint build output or vendored deps.
  {
    ignores: ["dist/**", "node_modules/**"],
  },
  // Base JS recommended rules.
  js.configs.recommended,
  // TypeScript sources and tests.
  {
    files: ["**/*.ts"],
    languageOptions: {
      parser: tsParser,
      ecmaVersion: "latest",
      sourceType: "module",
    },
    plugins: {
      "@typescript-eslint": tsPlugin,
    },
    rules: {
      ...tsPlugin.configs.recommended.rules,
      // Unused vars are an error, but allow the conventional `_`-prefix escape
      // hatch for intentionally-unused bindings.
      "@typescript-eslint/no-unused-vars": [
        "error",
        { argsIgnorePattern: "^_", varsIgnorePattern: "^_" },
      ],
      // The core uses the non-null assertion `!` deliberately and pervasively to
      // express "this child index is in range" invariants that mirror the Rust
      // reference's indexing; flagging every one would be pure churn with no bug
      // surfaced. Off, matching the deliberate style.
      "@typescript-eslint/no-non-null-assertion": "off",
      // `no-undef` is redundant under TypeScript (tsc already resolves globals and
      // the flat-config JS rule does not know the TS/DOM/node lib surface).
      "no-undef": "off",
    },
  },
];
