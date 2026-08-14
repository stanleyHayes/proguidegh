import type { Metadata, Viewport } from "next";
import { IBM_Plex_Mono, Outfit } from "next/font/google";
import "./globals.css";
import { Footer, Nav } from "./components/site";
import { getContent, SITE_URL } from "./lib/content";

/**
 * Outfit carries the brand at two weights — one geometric sans doing display
 * and body keeps the page coherent, and its wide apertures hold up at the
 * hero size. IBM Plex Mono stays for record data only: licence numbers,
 * credential field labels, source citations. Outfit has no mono, and that
 * "official document" voice is doing real work on the credential card.
 * Both self-hosted by next/font, so no runtime request leaves the page.
 */
const display = Outfit({
  subsets: ["latin"],
  weight: ["500", "600", "700"],
  variable: "--font-display",
  display: "swap",
});

const body = Outfit({
  subsets: ["latin"],
  weight: ["400", "500", "600"],
  variable: "--font-body",
  display: "swap",
});

const mono = IBM_Plex_Mono({
  subsets: ["latin"],
  weight: ["400", "500"],
  variable: "--font-mono",
  display: "swap",
});

export const metadata: Metadata = {
  metadataBase: new URL(SITE_URL),
  title: {
    default: "ProGuideGH — Certified tour guides in Ghana",
    template: "%s · ProGuideGH",
  },
  description:
    "Book a licensed, background-checked tour guide in Accra, Cape Coast or Kumasi. Fixed prices, tracked tours, and an SOS button on every booking.",
  applicationName: "ProGuideGH",
  // Not keyword stuffing — these are the phrases the product actually answers,
  // and several engines still read them for disambiguation.
  keywords: [
    "tour guide Ghana",
    "certified tour guide Accra",
    "Cape Coast Castle guide",
    "Kumasi tour guide",
    "Kakum National Park guide",
    "licensed guide Ghana",
    "Ghana heritage tours",
  ],
  authors: [{ name: "ProGuideGH" }],
  creator: "ProGuideGH",
  publisher: "ProGuideGH",
  category: "travel",
  formatDetection: { telephone: true, address: false, email: false },
  openGraph: {
    type: "website",
    siteName: "ProGuideGH",
    locale: "en_GH",
    url: SITE_URL,
    title: "ProGuideGH — Certified tour guides in Ghana",
    description:
      "Book a licensed, background-checked tour guide in Accra, Cape Coast or Kumasi. Fixed prices, tracked tours, and an SOS button on every booking.",
  },
  twitter: {
    card: "summary_large_image",
    title: "ProGuideGH — Certified tour guides in Ghana",
    description:
      "Licensed, background-checked guides in Accra, Cape Coast and Kumasi. Fixed prices and tracked tours.",
  },
  robots: {
    index: true,
    follow: true,
    googleBot: { index: true, follow: true, "max-image-preview": "large", "max-snippet": -1 },
  },
  alternates: { canonical: "/" },
};

export const viewport: Viewport = {
  width: "device-width",
  initialScale: 1,
  themeColor: "#052a1d",
};

export default async function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const content = await getContent();

  return (
    <html lang="en-GH" className={`${display.variable} ${body.variable} ${mono.variable}`}>
      <body>
        <a className="skip-link" href="#main">
          Skip to content
        </a>
        <Nav />
        <main id="main">{children}</main>
        <Footer content={content} />
      </body>
    </html>
  );
}
