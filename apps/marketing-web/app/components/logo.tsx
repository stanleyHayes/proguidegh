/**
 * ProGuideGH mark.
 *
 * A compass needle above a waypoint. Wayfinding is the literal job — a guide
 * goes ahead of you and knows where the thing is — and a needle reads at 16px
 * where anything more illustrative turns to mush.
 *
 * Two colours only: the deep green ground and a single gold waypoint. The
 * earlier tricolour treatment read as flag bunting rather than a brand, so red
 * is now reserved for genuine danger states and appears nowhere decorative.
 */

export function LogoMark({ size = 28 }: { size?: number }) {
  return (
    <svg
      aria-hidden
      fill="none"
      height={size}
      viewBox="0 0 32 32"
      width={size}
      xmlns="http://www.w3.org/2000/svg"
    >
      <rect fill="#0b3532" height="32" rx="9" width="32" />
      {/* Needle: asymmetric so it reads as pointing, not as a diamond. */}
      <path d="M16 5.5 L20.8 19.4 L16 16.5 L11.2 19.4 Z" fill="#ffffff" />
      <circle cx="16" cy="24" r="2.1" fill="#c9973d" />
    </svg>
  );
}

/** Mark plus wordmark, for the nav and footer. */
export function Logo({ size = 28 }: { size?: number }) {
  return (
    <>
      <LogoMark size={size} />
      <span>
        ProGuide<span style={{ color: "#c9973d" }}>GH</span>
      </span>
    </>
  );
}
