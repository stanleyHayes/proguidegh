"use client";

import type { ReactNode } from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";

export function ActiveLink({ href, children, className }: { href: string; children: ReactNode; className?: string }) {
  const pathname = usePathname();
  const active = href === "/" ? pathname === "/" : pathname === href || pathname.startsWith(`${href}/`);
  return <Link href={href} className={className} aria-current={active ? "page" : undefined}>{children}</Link>;
}
