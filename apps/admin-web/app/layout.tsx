import type { Metadata, Viewport } from "next";
import "@proguidegh/ui/tokens.css";
import "@proguidegh/ui/components.css";
import "./globals.css";
import "./route-polish.css";
import { AdminShell } from "./components/AdminShell";


export const metadata: Metadata = {
  title: "ProGuideGH — Admin command center",
  description:
    "Operations command center for the ProGuideGH platform: guides, tours, finance and safety.",
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
        <a className="skip-link" href="#admin-content">Skip to workspace</a>
        <AdminShell>{children}</AdminShell>
      </body>
    </html>
  );
}
