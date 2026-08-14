import type { Metadata, Viewport } from "next";
import "@proguidegh/ui/tokens.css";
import "@proguidegh/ui/components.css";
import "./globals.css";
import "./route-polish.css";
import { ServiceWorkerRegister } from "./components/ServiceWorkerRegister";
import { ConnectivityBanner } from "./components/ConnectivityBanner";
import { SiteNav } from "./components/SiteNav";


export const metadata: Metadata = {
  title: "ProGuideGH — Guide dashboard",
  description:
    "Dashboard for certified Ghana tour guides: jobs, tours, wallet and training.",
  manifest: "/manifest.webmanifest",
  appleWebApp: {
    capable: true,
    title: "ProGuideGH Guides",
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
        <a className="skip-link" href="#main-content">Skip to workspace</a>
        <header className="site-header">
          <div className="container site-header__inner">
            <a className="site-header__brand" href="/">
              <span className="brand-mark" aria-hidden="true">PG</span>
              <span>Guide workspace<small>ProGuideGH partner desk</small></span>
            </a>
            <SiteNav />
          </div>
        </header>
        <main className="container" id="main-content">
          <ConnectivityBanner />
          {children}
        </main>
        <footer className="site-footer"><div className="container site-footer__inner"><strong>Guide support</strong><span>Keep your profile, certification and availability current.</span><a href="/guide/profile">Review profile</a></div></footer>
        <ServiceWorkerRegister />
      </body>
    </html>
  );
}
