import type { MetadataRoute } from "next";

/**
 * Web app manifest. The marketing site is not an installable app — the real
 * apps are on the stores — but the manifest gives Android Chrome the name,
 * theme colour and icon it uses for a home-screen shortcut and for the
 * browser UI tint.
 */
export default function manifest(): MetadataRoute.Manifest {
  return {
    name: "ProGuideGH — Certified tour guides in Ghana",
    short_name: "ProGuideGH",
    description:
      "Book a licensed, background-checked tour guide in Accra, Cape Coast or Kumasi.",
    start_url: "/",
    display: "browser",
    background_color: "#052a1d",
    theme_color: "#052a1d",
    lang: "en-GH",
    categories: ["travel", "tourism"],
    icons: [
      { src: "/icon.svg", sizes: "any", type: "image/svg+xml" },
      { src: "/apple-icon", sizes: "180x180", type: "image/png" },
    ],
  };
}
