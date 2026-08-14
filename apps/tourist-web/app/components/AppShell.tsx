"use client";

import type { ReactNode } from "react";
import { usePathname } from "next/navigation";
import { ConnectivityBanner } from "./ConnectivityBanner";
import { SiteNav } from "./SiteNav";

const authRoutes = ["/login", "/register", "/forgot-password", "/reset-password"];

export function AppShell({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  const auth = authRoutes.some((route) => pathname === route || pathname.startsWith(`${route}/`));
  if (auth) return <main id="main-content">{children}</main>;
  return <><header className="site-header"><div className="container site-header__inner"><a className="site-header__brand" href="/"><span className="brand-mark" aria-hidden="true">PG</span><span>ProGuideGH<small>Travel with local authority</small></span></a><SiteNav /><a className="gg-button gg-button--primary header-cta" href="/search">Find a guide</a></div></header><main className="container" id="main-content"><ConnectivityBanner />{children}</main><footer className="site-footer"><div className="container site-footer__inner"><div><strong>ProGuideGH</strong><p>Certified local knowledge, booked with confidence.</p></div><nav aria-label="Footer"><a href="/search">Find a guide</a><a href="/bookings">My bookings</a><a href="/account/delete">Account & privacy</a></nav><small>Built for safer travel across Ghana.</small></div></footer></>;
}
