"use client";
import type { ReactNode } from "react";
import { usePathname } from "next/navigation";
import { ConnectivityBanner } from "./ConnectivityBanner";
import { SiteNav } from "./SiteNav";
const authRoutes = ["/login", "/register", "/forgot-password", "/reset-password"];
function FooterIcon({ name }: { name: "search" | "calendar" | "privacy" }) {
  const paths = { search: <><circle cx="10.5" cy="10.5" r="5.5"/><path d="m15 15 4 4"/></>, calendar: <><rect x="3" y="5" width="18" height="16" rx="2"/><path d="M8 3v4M16 3v4M3 10h18M8 14h3M8 17h6"/></>, privacy: <><rect x="5" y="10" width="14" height="11" rx="2"/><path d="M8 10V7a4 4 0 0 1 8 0v3M12 14v3"/></> };
  return <span className="site-footer__icon" aria-hidden="true"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">{paths[name]}</svg></span>;
}
function FooterLink({ href, icon, title, description, pathname }: { href: string; icon: "search" | "calendar" | "privacy"; title: string; description: string; pathname: string }) {
  const active = pathname === href || pathname.startsWith(`${href}/`);
  return <a href={href} aria-current={active ? "page" : undefined}><FooterIcon name={icon}/><span><strong>{title}</strong><small>{description}</small></span><b aria-hidden="true">↗</b></a>;
}
export function AppShell({ children }: { children: ReactNode }) {
  const pathname = usePathname(); const auth = authRoutes.some((route) => pathname === route || pathname.startsWith(`${route}/`));
  if (auth) return <main id="main-content">{children}</main>;
  return <><header className="site-header"><div className="container site-header__inner"><a className="site-header__brand" href="/"><span className="brand-mark" aria-hidden="true">PG</span><span>ProGuideGH<small>Travel with local authority</small></span></a><SiteNav/><a className="gg-button gg-button--primary header-cta" href="/search">Find a guide</a></div></header><main className="container" id="main-content"><ConnectivityBanner/>{children}</main><footer className="site-footer"><div className="container site-footer__inner"><div className="site-footer__brand"><strong>ProGuideGH</strong><p>Certified local knowledge,<br/>booked with confidence.</p><span><i/>Live across Ghana</span></div><nav aria-label="Footer navigation"><FooterLink pathname={pathname} href="/search" icon="search" title="Find a guide" description="Search certified experts"/><FooterLink pathname={pathname} href="/bookings" icon="calendar" title="My bookings" description="Manage upcoming tours"/><FooterLink pathname={pathname} href="/account/delete" icon="privacy" title="Account & privacy" description="Control your information"/></nav><small className="site-footer__note">Built for safer travel<br/>across Ghana.</small></div></footer></>;
}
