import type { ReactNode } from "react";

export function AdminAuthShell({ children }: { children: ReactNode }) {
  return (
    <div className="admin-auth">
      <aside className="admin-auth__brief">
        <a className="admin-auth__brand" href="/" aria-label="ProGuideGH home"><span>PG</span><div>ProGuideGH<small>Operations command center</small></div></a>
        <div className="admin-auth__story">
          <p className="admin-auth__eyebrow">Restricted workspace · Ghana</p>
          <h2>One secure desk for every critical decision.</h2>
          <p>Review guide credentials, watch active tours, resolve safety events and protect every financial action.</p>
          <ul>
            <li><span>01</span><div><strong>Identity controlled</strong><small>Role-based access with MFA enforcement</small></div></li>
            <li><span>02</span><div><strong>Actions traceable</strong><small>Privileged changes are recorded for review</small></div></li>
            <li><span>03</span><div><strong>Operations live</strong><small>Safety and platform signals in one workspace</small></div></li>
          </ul>
        </div>
        <footer><i aria-hidden="true" /> Platform services monitored <span>Accra · GMT</span></footer>
      </aside>
      <section className="admin-auth__panel">
        <div className="admin-auth__panel-inner">{children}</div>
        <p className="admin-auth__legal">Authorized personnel only · Sessions and privileged activity are audited.</p>
      </section>
    </div>
  );
}
