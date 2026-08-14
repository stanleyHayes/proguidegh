"use client";

import { usePathname } from "next/navigation";

const links = [
  ["Explore", "/search"],
  ["Bookings", "/bookings"],
  ["Profile", "/profile"],
] as const;

export function SiteNav() {
  const pathname = usePathname();

  return (
    <nav className="site-nav" aria-label="Tourist">
      {links.map(([label, href]) => (
        <a
          aria-current={pathname === href || pathname.startsWith(`${href}/`) ? "page" : undefined}
          href={href}
          key={href}
        >
          {label}
        </a>
      ))}
    </nav>
  );
}
