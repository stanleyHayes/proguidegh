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
  /** Ghana green — primary actions, active states. */
  primary: "#0b6e4f",
  primaryStrong: "#084c36",
  /** Ghana red — destructive and alert emphasis only. */
  accent: "#c8102e",
  /** Ghana gold — ratings, badges, highlights. */
  gold: "#f5b70a",
  ink: "#1a1a1a",
  muted: "#5a5a5a",
  surface: "#ffffff",
  surfaceAlt: "#f6f7f6",
  border: "#d9ddd9",
  focus: "#1d4ed8",
  success: "#0b6e4f",
  warning: "#92400e",
  danger: "#b91c1c",
} as const;

/** Font sizes in density-independent pixels. */
export const fontSize = {
  sm: 14,
  base: 16,
  lg: 18,
  xl: 22,
  "2xl": 28,
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
  sm: 6,
  md: 12,
  full: 999,
} as const;

/** Widest content column, matching the web `--gg-content-max`. */
export const contentMax = 1152;

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
