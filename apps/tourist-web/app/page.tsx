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
          <input id="date" name="date" type="date" />
        </div>
        <div>
          <Button type="submit">Search guides</Button>
        </div>
      </form>

      <div className="grid grid--cols-3">
        <Card title="Verified & certified">
          Every guide is vetted and certified through the official verification
          pipeline before appearing in search.
        </Card>
        <Card title="Safe payments">
          Pay by MoMo or card with escrowed funds and instant digital receipts.
        </Card>
        <Card title="Live tour tracking">
          Follow your tour in real time and reach support whenever you need it.
        </Card>
      </div>
    </div>
  );
}
