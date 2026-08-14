import { ImageResponse } from "next/og";

/**
 * The card that appears when the site is shared on WhatsApp, X, LinkedIn or
 * iMessage. For a Ghanaian consumer product WhatsApp is the main distribution
 * channel, so this is closer to a billboard than an afterthought.
 *
 * Generated at build time from the same palette as the site. Deliberately
 * type-only: a photograph would be a stock image of a beach, which is the
 * opposite of what this brand argues.
 */
export const alt = "ProGuideGH — certified tour guides in Ghana";
export const size = { width: 1200, height: 630 };
export const contentType = "image/png";

export default function OpenGraphImage() {
  return new ImageResponse(
    (
      <div
        style={{
          width: "100%",
          height: "100%",
          display: "flex",
          flexDirection: "column",
          justifyContent: "space-between",
          background: "#052a1d",
          padding: "72px",
        }}
      >
        <div style={{ display: "flex", alignItems: "center", gap: "20px" }}>
          <svg fill="none" height="56" viewBox="0 0 32 32" width="56">
            <rect fill="#0b6e4f" height="32" rx="9" width="32" />
            <path d="M16 5.5 L20.8 19.4 L16 16.5 L11.2 19.4 Z" fill="#ffffff" />
            <circle cx="16" cy="24" r="2.1" fill="#f5b70a" />
          </svg>
          <div style={{ display: "flex", fontSize: 38, fontWeight: 700, color: "#ffffff" }}>
            ProGuide<span style={{ color: "#f5b70a" }}>GH</span>
          </div>
        </div>

        <div
          style={{
            display: "flex",
            fontSize: 76,
            fontWeight: 700,
            color: "#ffffff",
            lineHeight: 1.05,
            letterSpacing: "-0.03em",
            maxWidth: "980px",
          }}
        >
          Every guide is licensed, checked, and tracked for the whole tour.
        </div>

        <div style={{ display: "flex", alignItems: "center", gap: "16px" }}>
          <div style={{ display: "flex", width: "64px", height: "4px", background: "#f5b70a" }} />
          <div style={{ display: "flex", fontSize: 28, color: "#9fb4a8", letterSpacing: "0.04em" }}>
            Accra · Cape Coast · Kumasi
          </div>
        </div>
      </div>
    ),
    size,
  );
}
