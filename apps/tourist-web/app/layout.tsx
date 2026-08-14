import type { Metadata, Viewport } from "next";
import "@proguidegh/ui/tokens.css";
import "@proguidegh/ui/components.css";
import "./globals.css";
import "./route-polish.css";
import { ServiceWorkerRegister } from "./components/ServiceWorkerRegister";
import { AppShell } from "./components/AppShell";


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
        <AppShell>{children}</AppShell>
        <ServiceWorkerRegister />
      </body>
    </html>
  );
}
