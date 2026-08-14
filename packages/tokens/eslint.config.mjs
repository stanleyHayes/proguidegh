import base from "@proguidegh/config/eslint.config.base.mjs";

export default [
  ...base,
  {
    // Parity checker runs under Node, not in a browser or RN runtime.
    files: ["scripts/**/*.mjs"],
    languageOptions: {
      globals: { console: "readonly", process: "readonly" },
    },
  },
];
