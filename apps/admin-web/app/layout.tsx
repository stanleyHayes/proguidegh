import type { Metadata, Viewport } from "next";
import "@proguidegh/ui/tokens.css";
import "@proguidegh/ui/components.css";
import "./globals.css";

export const metadata: Metadata = {
  title: "ProGuideGH — Admin command center",
  description:
    "Operations command center for the ProGuideGH platform: guides, tours, finance and safety.",
};

export const viewport: Viewport = {
  width: "device-width",
  initialScale: 1,
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
              ProGuideGH · Admin
            </a>
            <nav className="nav-actions" aria-label="Admin">
              <a href="/admin/tours">Tours</a>
              <a href="/admin/guides">Guides</a>
              <a href="/admin/certification">Certification</a>
              <a href="/admin/users">Users</a>
              <a href="/admin/content">Content</a>
              <a href="/admin/legal">Legal</a>
            </nav>
          </div>
        </header>
        <main className="container">{children}</main>
      </body>
    </html>
  );
}
