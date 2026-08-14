"use client";

import type { ReactNode } from "react";
import { useEffect, useMemo, useState } from "react";
import { usePathname } from "next/navigation";

type IconName = "pulse" | "tour" | "shield" | "quality" | "guide" | "badge" | "users" | "learn" | "wallet" | "report" | "content" | "legal" | "audit" | "settings";
type NavLink = { label: string; href: string; icon: IconName };
type NavGroup = { label: string; links: readonly NavLink[] };

const groups: readonly NavGroup[] = [
  { label: "Operations", links: [
    { label: "Overview", href: "/", icon: "pulse" }, { label: "Tour operations", href: "/admin/tours", icon: "tour" },
    { label: "Safety desk", href: "/admin/incidents", icon: "shield" }, { label: "Quality review", href: "/admin/quality", icon: "quality" },
  ] },
  { label: "Network", links: [
    { label: "Guide directory", href: "/admin/guides", icon: "guide" }, { label: "Certification", href: "/admin/certification", icon: "badge" },
    { label: "Users & roles", href: "/admin/users", icon: "users" }, { label: "Training", href: "/admin/training", icon: "learn" },
  ] },
  { label: "Governance", links: [
    { label: "Finance & payouts", href: "/admin/finance", icon: "wallet" }, { label: "Reports", href: "/admin/reports", icon: "report" },
    { label: "Content", href: "/admin/content", icon: "content" }, { label: "Legal", href: "/admin/legal", icon: "legal" },
    { label: "Audit trail", href: "/admin/audit", icon: "audit" }, { label: "Configuration", href: "/admin/settings", icon: "settings" },
  ] },
];

const routeCopy: Record<string, { title: string; eyebrow: string }> = {
  "/": { title: "Command center", eyebrow: "Platform pulse" }, "/admin/tours": { title: "Tour operations", eyebrow: "Live operations" },
  "/admin/incidents": { title: "Safety desk", eyebrow: "Trust & safety" }, "/admin/quality": { title: "Quality review", eyebrow: "Guide standards" },
  "/admin/guides": { title: "Guide directory", eyebrow: "People" }, "/admin/certification": { title: "Certification", eyebrow: "People" },
  "/admin/users": { title: "Users & roles", eyebrow: "Access control" }, "/admin/training": { title: "Training", eyebrow: "Guide standards" },
  "/admin/finance": { title: "Finance & payouts", eyebrow: "Stewardship" }, "/admin/reports": { title: "Reports", eyebrow: "Platform intelligence" },
  "/admin/content": { title: "Content", eyebrow: "Publishing" }, "/admin/legal": { title: "Legal documents", eyebrow: "Governance" },
  "/admin/audit": { title: "Audit trail", eyebrow: "Accountability" }, "/admin/settings": { title: "Platform configuration", eyebrow: "System" },
  "/admin/account": { title: "Account settings", eyebrow: "Personal workspace" },
  "/login": { title: "Administrator access", eyebrow: "Secure session" },
};

const paths: Record<IconName, string> = {
  pulse: "M4 12h3l2-6 4 12 2-6h5", tour: "M5 19V7l7-3 7 3v12l-7 3-7-3Zm7-15v18M5 7l7 3 7-3",
  shield: "M12 3 4.5 6v5.5c0 4.7 3.1 7.6 7.5 9.5 4.4-1.9 7.5-4.8 7.5-9.5V6L12 3Zm0 5v5m0 3h.01",
  quality: "m12 3 2.3 4.7 5.2.8-3.8 3.7.9 5.3-4.6-2.5-4.6 2.5.9-5.3-3.8-3.7 5.2-.8L12 3Z",
  guide: "M8 8a4 4 0 1 0 8 0 4 4 0 0 0-8 0Zm-3 13c.6-4 3-6 7-6s6.4 2 7 6", badge: "M12 3 9 5H5v4l-2 3 2 3v4h4l3 2 3-2h4v-4l2-3-2-3V5h-4l-3-2Zm-3 9 2 2 4-4",
  users: "M8 11a3 3 0 1 0 0-6 3 3 0 0 0 0 6Zm8-1a2.5 2.5 0 1 0 0-5M3 20c.4-4 2-6 5-6s4.6 2 5 6m1-6c3 0 5 2 5 6",
  learn: "M4 5h11a3 3 0 0 1 3 3v11H7a3 3 0 0 0-3 2V5Zm0 0v14a3 3 0 0 1 3-3h11", wallet: "M4 6h14a2 2 0 0 1 2 2v10H6a2 2 0 0 1-2-2V6Zm0 3h16m-5 5h2",
  report: "M5 20V10m7 10V4m7 16v-7", content: "M5 3h10l4 4v14H5V3Zm10 0v5h4M8 12h8M8 16h6",
  legal: "M12 3v18M6 6h12M7 6l-3 7h6L7 6Zm10 0-3 7h6l-3-7ZM8 21h8", audit: "M5 4h14v16H5V4Zm4 5h6M9 13h6M9 17h4",
  settings: "M12 8a4 4 0 1 0 0 8 4 4 0 0 0 0-8Zm0-5v2m0 14v2M3 12h2m14 0h2M5.6 5.6 7 7m10 10 1.4 1.4m0-12.8L17 7M7 17l-1.4 1.4",
};

function Icon({ name }: { name: IconName }) { return <svg className="admin-nav__icon" viewBox="0 0 24 24" aria-hidden="true"><path d={paths[name]} /></svg>; }
function Connector({ last, active }: { last: boolean; active: boolean }) {
  return <svg className={`admin-nav__connector${active ? " is-active" : ""}`} viewBox="0 0 28 44" preserveAspectRatio="none" aria-hidden="true"><path d={last ? "M8 0V18Q8 24 14 24H28" : "M8 0V44M8 24H28"} /></svg>;
}
function isActive(pathname: string, href: string) { return href === "/" ? pathname === href : pathname === href || pathname.startsWith(`${href}/`); }
function contextFor(pathname: string) {
  if (routeCopy[pathname]) return routeCopy[pathname];
  const parent = Object.keys(routeCopy).filter((path) => path !== "/" && pathname.startsWith(`${path}/`)).sort((a, b) => b.length - a.length)[0];
  return (parent ? routeCopy[parent] : undefined) ?? routeCopy["/"]!;
}

export function AdminShell({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  const page = contextFor(pathname);
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [accountOpen, setAccountOpen] = useState(false);
  const [notificationsOpen, setNotificationsOpen] = useState(false);
  const [tourStep, setTourStep] = useState<number | null>(null);
  const [closedGroups, setClosedGroups] = useState<Record<string, boolean>>({});
  const activeGroup = useMemo(() => groups.find((group) => group.links.some((link) => isActive(pathname, link.href)))?.label, [pathname]);

  useEffect(() => setDrawerOpen(false), [pathname]);
  useEffect(() => { document.body.style.overflow = drawerOpen ? "hidden" : ""; return () => { document.body.style.overflow = ""; }; }, [drawerOpen]);
  useEffect(() => {
    if (pathname.startsWith("/login")) return;
    if (!window.localStorage.getItem("proguidegh-admin-tour-complete")) setTourStep(0);
  }, [pathname]);

  if (pathname.startsWith("/login")) return <main className="auth-stage" id="admin-content">{children}</main>;

  const tour = [
    ["Welcome to operations", "This command center keeps tours, safety, people and platform stewardship in one workspace."],
    ["Navigation stays put", "The rail is fixed while each navigation group and the page content scroll independently."],
    ["Your account lives here", "Use the account menu for profile, security, preferences, notifications, or to replay this walkthrough."],
  ] as const;
  function closeTour() { window.localStorage.setItem("proguidegh-admin-tour-complete", "true"); setTourStep(null); }

  return <div className="admin-frame">
    <button className={`admin-scrim${drawerOpen ? " is-open" : ""}`} aria-label="Close navigation" onClick={() => setDrawerOpen(false)} />
    <aside className={`admin-rail${drawerOpen ? " is-open" : ""}`}>
      <div className="admin-brand-row"><a className="admin-brand" href="/"><span className="brand-mark">PG</span><span>ProGuideGH<small>Operations console</small></span></a><button className="admin-rail__close" type="button" aria-label="Close navigation" onClick={() => setDrawerOpen(false)}>×</button></div>
      <div className="admin-workspace"><small>Live workspace</small><strong>Ghana operations</strong><span><i /> Systems nominal</span></div>
      <nav className="admin-nav" aria-label="Administration">{groups.map((group) => {
        const open = group.label === activeGroup || !closedGroups[group.label];
        return <section className="admin-nav__group" key={group.label}>
          <button type="button" aria-expanded={open} onClick={() => setClosedGroups((current) => ({ ...current, [group.label]: open }))}><span>{group.label}</span><b aria-hidden="true">⌄</b></button>
          <div className={`admin-nav__links${open ? " is-open" : ""}`}><div>{group.links.map((link, index) => {
            const active = isActive(pathname, link.href);
            return <a aria-current={active ? "page" : undefined} key={link.href} href={link.href}><Connector last={index === group.links.length - 1} active={active} /><Icon name={link.icon} /><span>{link.label}</span>{active && <i aria-hidden="true">✓</i>}</a>;
          })}</div></div>
        </section>;
      })}</nav>
      <button className="admin-profile" type="button" onClick={() => setAccountOpen((open) => !open)} aria-expanded={accountOpen}><span>SA</span><div><strong>System administrator</strong><small>Secure operations account</small></div><b aria-hidden="true">•••</b></button>
    </aside>
    <section className="admin-stage">
      <header className="admin-topbar"><button className="admin-menu" type="button" aria-label="Open navigation" onClick={() => setDrawerOpen(true)}><span /><span /><span /></button><div className="admin-topbar__title"><span className="eyebrow">{page.eyebrow}</span><strong>{page.title}</strong></div><div className="admin-topbar__actions"><a className="admin-alert-link" href="/admin/incidents"><span /> Safety desk</a><div className="admin-popover-anchor"><button className="admin-icon-button" type="button" aria-label="Notifications" onClick={() => { setNotificationsOpen((open) => !open); setAccountOpen(false); }}>♢<i /></button>{notificationsOpen && <div className="admin-popover admin-notifications"><header><strong>Notifications</strong><button onClick={() => setNotificationsOpen(false)}>×</button></header><div className="admin-empty"><span>✓</span><strong>You are all caught up</strong><p>Operational alerts and account updates will appear here.</p></div><a href="/admin/account?tab=notifications">Notification preferences</a></div>}</div><div className="admin-popover-anchor"><button className="admin-avatar" type="button" onClick={() => { setAccountOpen((open) => !open); setNotificationsOpen(false); }} aria-expanded={accountOpen}>SA</button>{accountOpen && <div className="admin-popover admin-account-menu"><header><span>SA</span><div><strong>System administrator</strong><small>admin@proguidegh.com</small></div></header><a href="/admin/account?tab=profile"><b>Profile</b><small>Account details and identity</small></a><a href="/admin/account?tab=security"><b>Security</b><small>Password and multi-factor authentication</small></a><a href="/admin/account?tab=preferences"><b>Preferences</b><small>Workspace appearance and behavior</small></a><a href="/admin/account?tab=notifications"><b>Notifications</b><small>Choose the alerts you receive</small></a><button type="button" onClick={() => { setAccountOpen(false); setTourStep(0); }}><b>Replay walkthrough</b><small>Tour the command center again</small></button><a className="is-danger" href="/login"><b>Sign out</b><small>End this secure session</small></a></div>}</div></div></header>
      <main className="container" id="admin-content">{children}</main>
    </section>
    {tourStep !== null && <div className="admin-tour" role="dialog" aria-modal="true" aria-labelledby="tour-title"><div className="admin-tour__card"><span>STEP {tourStep + 1} OF {tour.length}</span><button aria-label="Close walkthrough" onClick={closeTour}>×</button><div className="admin-tour__guide">PG</div><h2 id="tour-title">{tour[tourStep][0]}</h2><p>{tour[tourStep][1]}</p><footer>{tourStep > 0 ? <button onClick={() => setTourStep(tourStep - 1)}>Back</button> : <button onClick={closeTour}>Skip</button>}<button className="is-primary" onClick={() => tourStep === tour.length - 1 ? closeTour() : setTourStep(tourStep + 1)}>{tourStep === tour.length - 1 ? "Finish" : "Next"}</button></footer></div></div>}
  </div>;
}
