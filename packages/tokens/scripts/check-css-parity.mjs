/**
 * Fails if `packages/ui/src/tokens.css` and `packages/tokens/src/index.ts`
 * disagree (Phase M, M-01).
 *
 * Web imports the CSS custom properties; React Native cannot, so it imports
 * the TypeScript values instead. Nothing stops the two drifting except this
 * check — run in CI and by `pnpm --filter @proguidegh/tokens test`.
 *
 * rem values are compared after conversion: `0.875rem` === `14` at remBase 16.
 */
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, resolve } from "node:path";

import { colors, contentMax, fontSize, radius, remBase, space } from "../src/index.ts";

const here = dirname(fileURLToPath(import.meta.url));
const cssPath = resolve(here, "../../ui/src/tokens.css");

/** `--gg-color-primary: #176b63;` -> Map { "--gg-color-primary" => "#176b63" } */
function parseCustomProperties(css) {
  const found = new Map();
  for (const [, name, value] of css.matchAll(/(--[\w-]+)\s*:\s*([^;]+);/g)) {
    found.set(name, value.replace(/\/\*[\s\S]*?\*\//g, "").trim());
  }
  return found;
}

/** `1.375rem` -> 22 at remBase 16; `999px` -> 999. Returns null if not a length. */
function toPixels(value) {
  const rem = /^(-?[\d.]+)rem$/.exec(value);
  if (rem) return Number(rem[1]) * remBase;
  const px = /^(-?[\d.]+)px$/.exec(value);
  if (px) return Number(px[1]);
  return null;
}

// token value (JS) -> CSS custom property name
const expected = [
  ...Object.entries(colors).map(([key, value]) => [
    `--gg-color-${key.replace(/[A-Z]/g, (c) => `-${c.toLowerCase()}`)}`,
    value,
    "color",
  ]),
  ...Object.entries(fontSize).map(([key, value]) => [`--gg-text-${key}`, value, "length"]),
  ...Object.entries(space).map(([key, value]) => [`--gg-space-${key}`, value, "length"]),
  ...Object.entries(radius).map(([key, value]) => [`--gg-radius-${key}`, value, "length"]),
  ["--gg-content-max", contentMax, "length"],
];

const css = parseCustomProperties(readFileSync(cssPath, "utf8"));
const problems = [];

for (const [property, tokenValue, kind] of expected) {
  const cssValue = css.get(property);
  if (cssValue === undefined) {
    problems.push(`${property} — declared in packages/tokens but missing from tokens.css`);
    continue;
  }
  if (kind === "color") {
    if (cssValue.toLowerCase() !== String(tokenValue).toLowerCase()) {
      problems.push(`${property} — tokens.css has ${cssValue}, packages/tokens has ${tokenValue}`);
    }
    continue;
  }
  const cssPixels = toPixels(cssValue);
  if (cssPixels === null) {
    problems.push(`${property} — tokens.css value ${cssValue} is not a rem/px length`);
  } else if (cssPixels !== tokenValue) {
    problems.push(
      `${property} — tokens.css has ${cssValue} (${cssPixels}px), packages/tokens has ${tokenValue}`,
    );
  }
}

if (problems.length > 0) {
  console.error("Design tokens have drifted between web and native:\n");
  for (const problem of problems) console.error(`  ✗ ${problem}`);
  console.error(
    "\nFix packages/tokens/src/index.ts and packages/ui/src/tokens.css together, in one commit.",
  );
  process.exit(1);
}

console.log(`✓ ${expected.length} tokens match between tokens.css and @proguidegh/tokens`);
