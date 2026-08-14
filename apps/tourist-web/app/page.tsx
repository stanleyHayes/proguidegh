import { Badge, Button, Card } from "@proguidegh/ui";

export default function HomePage() {
  return (
    <div className="stack">
      <section aria-labelledby="hero-heading">
        <Badge tone="success">Certified guides only</Badge>
        <h1 id="hero-heading">Explore Ghana with a certified local guide</h1>
        <p className="muted">
          Search vetted guides and tour packages nationwide — with transparent
          pricing and official receipts.
        </p>
        <nav aria-label="Account" className="nav-actions">
          <a className="gg-button gg-button--secondary" href="/login">
            Sign in
          </a>
          <a className="gg-button gg-button--secondary" href="/register">
            Create an account
          </a>
          <a className="gg-button gg-button--secondary" href="/profile">
            My profile
          </a>
          <a className="gg-button gg-button--secondary" href="/search">
            Find a guide
          </a>
          <a className="gg-button gg-button--secondary" href="/bookings">
            My bookings
          </a>
        </nav>
      </section>

      <form
        role="search"
        aria-label="Search guides and tours"
        className="stack"
        action="/search"
      >
        <div className="field">
          <label htmlFor="destination">Destination</label>
          <input
            id="destination"
            name="destination"
            type="text"
            placeholder="e.g. Cape Coast, Mole, Kumasi"
            autoComplete="off"
          />
        </div>
        <div className="field">
          <label htmlFor="date">Tour date</label>
          <span className="brand-date-input"><svg aria-hidden="true" viewBox="0 0 24 24" fill="none"><rect x="3" y="5" width="18" height="16" rx="2"/><path d="M8 3v4M16 3v4M3 10h18"/></svg><input id="date" name="date" type="text" inputMode="numeric" placeholder="YYYY-MM-DD" pattern="[0-9]{4}-[0-9]{2}-[0-9]{2}" /></span>
        </div>
        <div>
          <Button type="submit">Search guides</Button>
        </div>
      </form>

      <div className="grid grid--cols-3 home-benefits">
        <Card title="Verified & certified">
          <span className="benefit-icon" aria-hidden="true"><svg viewBox="0 0 24 24" fill="none"><path d="M12 3 5 6v5c0 4.6 2.8 8.1 7 10 4.2-1.9 7-5.4 7-10V6l-7-3Z"/><path d="m9 12 2 2 4-5"/></svg></span>
          Every guide is vetted and certified through the official verification
          pipeline before appearing in search.
        </Card>
        <Card title="Safe payments">
          <span className="benefit-icon" aria-hidden="true"><svg viewBox="0 0 24 24" fill="none"><rect x="3" y="6" width="18" height="13" rx="2"/><path d="M3 10h18M7 15h3"/></svg></span>
          Pay by MoMo or card with escrowed funds and instant digital receipts.
        </Card>
        <Card title="Live tour tracking">
          <span className="benefit-icon" aria-hidden="true"><svg viewBox="0 0 24 24" fill="none"><path d="M12 21s6-5.2 6-11a6 6 0 1 0-12 0c0 5.8 6 11 6 11Z"/><circle cx="12" cy="10" r="2"/></svg></span>
          Follow your tour in real time and reach support whenever you need it.
        </Card>
      </div>
    </div>
  );
}
