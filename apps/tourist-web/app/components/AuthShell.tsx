import type { ReactNode } from "react";

export function AuthShell({ eyebrow, title, children }: { eyebrow: string; title: string; children: ReactNode }) {
  return <div className="auth-shell"><aside className="auth-story"><a className="auth-brand" href="/"><span>PG</span> ProGuideGH</a><div><p>{eyebrow}</p><h2>{title}</h2><ul><li><i>✓</i> Certified, identity-checked local guides</li><li><i>✓</i> Fixed prices before you pay</li><li><i>✓</i> Tracked tours with live safety support</li></ul></div><small>Travel with local authority.</small></aside><section className="auth-panel"><div className="auth-panel__inner">{children}</div><p className="auth-legal">By continuing, you agree to responsible use of ProGuideGH and its safety standards.</p></section></div>;
}
