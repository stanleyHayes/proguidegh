import type { ReactNode } from "react";

export function AuthShell({ eyebrow, title, children }: { eyebrow: string; title: string; children: ReactNode }) {
  return <div className="auth-shell auth-shell--guide"><aside className="auth-story"><a className="auth-brand" href="/"><span>PG</span> ProGuideGH Guides</a><div><p>{eyebrow}</p><h2>{title}</h2><ul><li><i>✓</i> Build a trusted public guide profile</li><li><i>✓</i> Receive and manage tour offers</li><li><i>✓</i> Track earnings and weekly payouts</li></ul></div><small>Your professional guide workspace.</small></aside><section className="auth-panel"><div className="auth-panel__inner">{children}</div><p className="auth-legal">Guide access is protected. Certification is required before receiving tours.</p></section></div>;
}
