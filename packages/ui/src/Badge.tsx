import type { ReactNode } from "react";

export type BadgeTone = "neutral" | "success" | "warning" | "danger";

export interface BadgeProps {
  tone?: BadgeTone;
  children: ReactNode;
}

export function Badge({ tone = "neutral", children }: BadgeProps) {
  return <span className={`gg-badge gg-badge--${tone}`}>{children}</span>;
}
