import { Badge, Card } from "@proguidegh/ui";

export default function HomePage() {
  return (
    <div className="stack">
      <section aria-labelledby="hero-heading">
        <Badge tone="success">Guide with ProGuideGH</Badge>
        <h1 id="hero-heading">Turn your local knowledge into certified work</h1>
        <p className="muted">
          Apply, get certified and receive dispatched tour jobs with weekly
          MoMo payouts.
        </p>
        <nav aria-label="Guide account" className="nav-actions">
          <a className="gg-button gg-button--secondary" href="/login">
            Sign in
          </a>
          <a className="gg-button gg-button--secondary" href="/register">
            Register as a guide
          </a>
          <a className="gg-button gg-button--secondary" href="/guide">
            Dashboard
          </a>
        </nav>
      </section>

      <div className="grid grid--cols-3">
        <Card title="Apply">
          Tell us where you guide and in which languages.
          <p>
            <a className="gg-button gg-button--secondary" href="/guide/apply">
              Start application
            </a>
          </p>
        </Card>
        <Card title="Get verified">
          Upload your documents and track every certification stage.
          <p>
            <a
              className="gg-button gg-button--secondary"
              href="/guide/verification"
            >
              Verification status
            </a>
          </p>
        </Card>
        <Card title="Start earning">
          Certified guides receive nearby job offers and weekly payouts.
        </Card>
      </div>
    </div>
  );
}
