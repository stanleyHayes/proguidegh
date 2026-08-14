"use client";

import { usePathname } from "next/navigation";

const links = [
  ["Dashboard", "/guide"],
  ["Jobs", "/guide/jobs"],
  ["Tours", "/guide/tours"],
  ["Wallet", "/guide/wallet"],
  ["Training", "/guide/training"],
  ["Verification", "/guide/verification"],
  ["Profile", "/guide/profile"],
] as const;

export function SiteNav() {
  const pathname = usePathname();

  return (
    <nav className="site-nav" aria-label="Guide">
      {links.map(([label, href]) => {
        const active = href === "/guide" ? pathname === href : pathname === href || pathname.startsWith(`${href}/`);
        return <a aria-current={active ? "page" : undefined} href={href} key={href}>{label}</a>;
      })}
    </nav>
  );
}
