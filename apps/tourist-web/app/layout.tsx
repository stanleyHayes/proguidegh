import type { Metadata, Viewport } from "next";
import "@proguidegh/ui/tokens.css";
import "@proguidegh/ui/components.css";
import "./globals.css";
import "./route-polish.css";
import { ServiceWorkerRegister } from "./components/ServiceWorkerRegister";
import { ConnectivityBanner } from "./components/ConnectivityBanner";
import { SiteNav } from "./components/SiteNav";


export const metadata: Metadata = {
  title: "ProGuideGH — Find a certified tour guide",
  description:
    "Book certified, vetted tour guides across Ghana. Safe, transparent, and official.",
  manifest: "/manifest.webmanifest",
  appleWebApp: {
    capable: true,
    title: "ProGuideGH",
    statusBarStyle: "default",
  },
  icons: { apple: "/icons/icon-192.png" },
};

export const viewport: Viewport = {
  width: "device-width",
  initialScale: 1,
  themeColor: "#0b3532",
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="en-GH">
      <body>
        <a className="skip-link" href="#main-content">Skip to content</a>
        <header className="site-header">
          <div className="container site-header__inner">
            <a className="site-header__brand" href="/">
              <span className="brand-mark" aria-hidden="true">PG</span>
              <span>ProGuideGH<small>Travel with local authority</small></span>
            </a>
            <SiteNav />
            <a className="gg-button gg-button--primary header-cta" href="/search">Find a guide</a>
          </div>
        </header>
        <main className="container" id="main-content">
          <ConnectivityBanner />
          {children}
        </main>
        <footer className="site-footer">
          <div className="container site-footer__inner">
            <div><strong>ProGuideGH</strong><p>Certified local knowledge, booked with confidence.</p></div>
            <nav aria-label="Footer"><a href="/search">Find a guide</a><a href="/bookings">My bookings</a><a href="/account/delete">Account & privacy</a></nav>
            <small>Built for safer travel across Ghana.</small>
          </div>
        </footer>
        <ServiceWorkerRegister />
      </body>
    </html>
  );
}
