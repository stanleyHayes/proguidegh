"use client";
import type { ReactNode } from "react";
import { usePathname } from "next/navigation";
import { ConnectivityBanner } from "./ConnectivityBanner";
import { SiteNav } from "./SiteNav";
const authRoutes = ["/login", "/register", "/forgot-password", "/reset-password"];
function ProfileIcon() { return <span className="site-footer__icon" aria-hidden="true"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round"><circle cx="12" cy="8" r="4"/><path d="M4.5 21a7.5 7.5 0 0 1 15 0M17 4l1 1 2-2"/></svg></span>; }
export function AppShell({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  const auth = authRoutes.some((route) => pathname === route || pathname.startsWith(`${route}/`));
  if (auth) return <main id="main-content">{children}</main>;
  return <><header className="site-header"><div className="container site-header__inner"><a className="site-header__brand" href="/"><span className="brand-mark" aria-hidden="true">PG</span><span>Guide workspace<small>ProGuideGH partner desk</small></span></a><SiteNav /></div></header><main className="container" id="main-content"><ConnectivityBanner />{children}</main><footer className="site-footer"><div className="container site-footer__inner"><strong>Guide support</strong><span>Keep your profile, certification and availability current.</span><a href="/guide/profile"><ProfileIcon />Review profile</a></div></footer></>;
}
