import { ImageResponse } from "next/og";

/**
 * Home-screen icon for iOS. Generated rather than shipped as a binary so the
 * mark has exactly one definition — change `components/logo.tsx` and this
 * follows. Apple does not render SVG favicons, hence the raster.
 */
export const size = { width: 180, height: 180 };
export const contentType = "image/png";

export default function AppleIcon() {
  return new ImageResponse(
    (
      <div
        style={{
          width: "100%",
          height: "100%",
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          background: "#0b6e4f",
        }}
      >
        <svg fill="none" height="180" viewBox="0 0 32 32" width="180">
          <path d="M16 5.5 L20.8 19.4 L16 16.5 L11.2 19.4 Z" fill="#ffffff" />
          <circle cx="16" cy="24" r="2.1" fill="#f5b70a" />
        </svg>
      </div>
    ),
    size,
  );
}
