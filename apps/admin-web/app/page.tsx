import { Badge, Card } from "@proguidegh/ui";
import type { HealthResponse } from "@proguidegh/contracts";

// Phase 0 placeholder until the API client is generated from OpenAPI.
const apiHealth: HealthResponse = {
  status: "ok",
  service: "proguidegh-api",
  version: "0.0.0",
  time: new Date(0).toISOString(),
};

export default function CommandCenterPage() {
  return (
    <div className="stack">
      <section aria-labelledby="cc-heading">
        <h1 id="cc-heading">Command center</h1>
        <p className="muted">
          Platform-wide operations overview. API status:{" "}
          <Badge tone={apiHealth.status === "ok" ? "success" : "danger"}>
            {apiHealth.service} · {apiHealth.status}
          </Badge>
        </p>
        <nav aria-label="Admin sections" className="nav-actions">
          <a className="gg-button gg-button--secondary" href="/login">
            Sign in
          </a>
          <a className="gg-button gg-button--secondary" href="/admin/users">
            Users & roles
          </a>
          <a className="gg-button gg-button--secondary" href="/admin/guides">
            Guide directory
          </a>
          <a className="gg-button gg-button--secondary" href="/admin/tours">
            Operations board
          </a>
          <a className="gg-button gg-button--secondary" href="/admin/incidents">
            Safety desk
          </a>
          <a className="gg-button gg-button--secondary" href="/admin/quality">
            Quality &amp; retraining
          </a>
          <a className="gg-button gg-button--secondary" href="/admin/finance">
            Finance &amp; payouts
          </a>
          <a className="gg-button gg-button--secondary" href="/admin/training">
            Training
          </a>
          <a className="gg-button gg-button--secondary" href="/admin/reports">
            Reports
          </a>
          <a className="gg-button gg-button--secondary" href="/admin/settings">
            Configuration
          </a>
          <a className="gg-button gg-button--secondary" href="/admin/audit">
            Audit trail
          </a>
        </nav>
      </section>

      <div className="grid grid--cols-4" aria-label="Platform stats">
        <Card title="Active tours">
          <p className="stat">0</p>
          Tours currently in progress.
        </Card>
        <Card title="Online guides">
          <p className="stat">0</p>
          Guides available for dispatch.
        </Card>
        <Card title="Pending verifications">
          <p className="stat">0</p>
          Applications awaiting review.
        </Card>
        <Card title="Open incidents">
          <p className="stat">0</p>
          Safety desk items needing action.
        </Card>
      </div>

      <div className="grid grid--cols-2">
        <Card title="Verification queue">
          New guide applications and document reviews will be triaged here.
          Empty state — nothing pending.
        </Card>
        <Card title="Finance & payouts">
          Revenue, tourism levy and payout batches. Empty state — no batches
          yet.
        </Card>
      </div>
    </div>
  );
}
