import type { Metadata, Viewport } from "next";
import "@proguidegh/ui/tokens.css";
import "@proguidegh/ui/components.css";
import "./globals.css";
import { ServiceWorkerRegister } from "./components/ServiceWorkerRegister";
import { ConnectivityBanner } from "./components/ConnectivityBanner";

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
  themeColor: "#0b6e4f",
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="en">
      <body>
        <header className="site-header">
          <div className="container site-header__inner">
            <a className="site-header__brand" href="/">
              ProGuideGH · Guides
            </a>
            <nav className="nav-actions" aria-label="Guide">
              <a href="/guide">Dashboard</a>
              <a href="/guide/jobs">Jobs</a>
              <a href="/guide/tours">Tours</a>
              <a href="/guide/wallet">Wallet</a>
              <a href="/guide/training">Training</a>
              <a href="/guide/verification">Verification</a>
              <a href="/guide/profile">Profile</a>
            </nav>
          </div>
        </header>
        <main className="container">
          <ConnectivityBanner />
          {children}
        </main>
        <ServiceWorkerRegister />
      </body>
    </html>
  );
}
