import expo from "eslint-config-expo/flat.js";

/**
 * React Native lint config. Deliberately NOT extending
 * `@proguidegh/config/eslint.config.base.mjs`: that base assumes DOM/browser
 * globals, while this app runs on Hermes. Shared rules live in the base for the
 * web workspaces; `eslint-config-expo` covers the native ones — including the
 * TypeScript rules, so this file adds no rule overrides of its own.
 */
export default [...expo, { ignores: [".expo/**", "dist/**", "expo-env.d.ts"] }];
