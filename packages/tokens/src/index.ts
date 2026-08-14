/**
 * ProGuideGH design tokens — platform-neutral source of truth (Phase M, M-01).
 *
 * React Native has no CSS custom properties, so the web apps' `tokens.css` and
 * the native apps cannot share a stylesheet. They share these values instead.
 *
 * `packages/ui/src/tokens.css` remains the file the web apps import; parity
 * between the two is enforced by `pnpm --filter @proguidegh/tokens check:css`,
 * which fails if any value here diverges from the CSS custom property of the
 * same name. Change a value here and in `tokens.css` in the same commit.
 *
 * Native consumers get numbers (React Native styles are unitless density-
 * independent pixels); the CSS uses rem. The parity check converts using
 * `remBase`, so a token expressed as `1rem` here is `16` on native.
 */

/** Root font size the CSS `rem` values are relative to. */
export const remBase = 16;

export const colors = {
  /** Mineral teal — primary actions, navigation and active states. */
  primary: "#176b63",
  primaryStrong: "#0e4c47",
  /** Burnt clay — expressive emphasis; never used as a generic alert colour. */
  accent: "#c7653f",
  /** Restrained brass — ratings, certification and editorial highlights. */
  gold: "#c9973d",
  ink: "#172421",
  muted: "#66736f",
  surface: "#fffdf8",
  surfaceAlt: "#f3f0e8",
  border: "#dcd8cd",
  focus: "#176b63",
  success: "#2e745c",
  warning: "#9a641f",
  danger: "#b5473c",
} as const;

/** Font sizes in density-independent pixels. */
export const fontSize = {
  sm: 14,
  base: 16,
  lg: 19,
  xl: 24,
  "2xl": 32,
} as const;

/** Spacing scale in density-independent pixels. */
export const space = {
  1: 4,
  2: 8,
  3: 12,
  4: 16,
  6: 24,
  8: 32,
  12: 48,
} as const;

export const radius = {
  sm: 8,
  md: 18,
  full: 999,
} as const;

/** Widest content column, matching the web `--gg-content-max`. */
export const contentMax = 1248;

export const tokens = {
  remBase,
  colors,
  fontSize,
  space,
  radius,
  contentMax,
} as const;

export type Tokens = typeof tokens;
export type ColorToken = keyof typeof colors;
export type SpaceToken = keyof typeof space;

export default tokens;
