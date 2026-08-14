import Link from "next/link";
import { Rule } from "./components/site";

export default function NotFound() {
  return (
    <>
      <section className="band band--field">
        <div className="wrap">
          <p className="eyebrow">404</p>
          <h1 className="h2" style={{ maxWidth: "18ch" }}>
            That page is not here.
          </h1>
          <p className="lede" style={{ marginTop: "1rem" }}>
            It may have moved, or the link may be wrong. These are the pages people
            usually want.
          </p>
          <div className="hero__actions">
            <Link className="btn btn--gold" href="/destinations">
              Destinations
            </Link>
            <Link className="btn btn--ghost" href="/become-a-guide">
              Become a guide
            </Link>
          </div>
        </div>
      </section>
      <Rule />
    </>
  );
}
