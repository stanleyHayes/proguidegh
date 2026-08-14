/**
 * Shared marketing chrome and the credential card that the whole site is
 * built around. Server components — this site ships no client JS beyond what
 * Next needs, because nothing here is interactive except native `<details>`.
 */
import Link from "next/link";
import type { ReactNode } from "react";
import { APP_URL, type MarketingContent } from "../lib/content";
import { Logo } from "./logo";
import { ActiveLink } from "./active-link";

export function Rule({ onField = false }: { onField?: boolean }) {
  return <hr aria-hidden className={onField ? "rule rule--onField" : "rule"} />;
}

export function Nav() {
  return (
    <nav className="nav" aria-label="Primary">
      <div className="wrap nav__inner">
        <Link className="nav__brand" href="/">
          <Logo />
          <span className="nav__brand-copy">Certified journeys<small>Ghana, guided better</small></span>
        </Link>
        {/* Links collapse on small screens; the primary action never does —
            most of this audience arrives on a phone. */}
        <div className="nav__links">
          <ActiveLink href="/destinations">Destinations</ActiveLink>
          <ActiveLink href="/safety">Safety</ActiveLink>
          <ActiveLink href="/pricing">Pricing</ActiveLink>
          <ActiveLink href="/become-a-guide">Become a guide</ActiveLink>
        </div>
        <div className="nav__actions">
          <a className="nav__signin" href={`${APP_URL}/login`}>Sign in</a>
          <a className="btn btn--gold btn--sm" href={`${APP_URL}/search`}><span>Find a guide</span><b aria-hidden="true">↗</b></a>
        </div>
      </div>
    </nav>
  );
}

export function Footer({ content }: { content: MarketingContent }) {
  return (
    <footer className="foot">
      <div className="wrap">
        <div className="foot__cta">
          <div><p className="eyebrow">Your Ghana story starts locally</p><h2>Meet the guide who makes the place make sense.</h2></div>
          <a className="btn btn--gold" href={`${APP_URL}/search`}>Find your guide <b aria-hidden="true">↗</b></a>
        </div>
        <div className="foot__grid">
          <div className="foot__brand">
            <Logo />
            <p>
              Certified local knowledge for safer, richer travel across Ghana.
            </p>
            <span className="foot__status"><i /> Live in Accra, Cape Coast &amp; Kumasi</span>
          </div>
          <div>
            <h4>Explore</h4>
            <ul>
              <li>
                <a href={`${APP_URL}/search`}>Find a guide</a>
              </li>
              <li>
                <ActiveLink href="/destinations">Destinations</ActiveLink>
              </li>
              <li>
                <ActiveLink href="/safety">Safety</ActiveLink>
              </li>
              <li>
                <ActiveLink href="/pricing">Pricing</ActiveLink>
              </li>
              <li>
                <ActiveLink href="/become-a-guide">Become a guide</ActiveLink>
              </li>
            </ul>
          </div>
          <div>
            <h4>Support & policy</h4>
            <ul>
              <li>
                <ActiveLink href="/about">About</ActiveLink>
              </li>
              <li>
                <ActiveLink href="/contact">Contact</ActiveLink>
              </li>
              <li>
                <ActiveLink href="/faq">Common questions</ActiveLink>
              </li>
              <li>
                <ActiveLink href="/legal/terms">Terms of service</ActiveLink>
              </li>
              <li>
                <ActiveLink href="/legal/privacy">Privacy policy</ActiveLink>
              </li>
              <li>
                <a href={`mailto:${content.contact.supportEmail}`}>Contact support</a>
              </li>
            </ul>
          </div>
          <div>
            <h4>For guides</h4>
            <ul><li><ActiveLink href="/become-a-guide">Become certified</ActiveLink></li><li><a href={`${APP_URL}/login`}>Guide sign in</a></li><li><ActiveLink href="/contact">Partner with us</ActiveLink></li></ul>
          </div>
        </div>

        <div className="foot__bottom">
          <span>
            © {new Date().getFullYear()} ProGuideGH · {content.contact.address}
          </span>
          <span>Certified guides. Transparent prices. Tracked tours.</span>
        </div>
      </div>
    </footer>
  );
}

export interface CredentialProps {
  name: string;
  licence: string;
  region: string;
  languages: string[];
  specialties: string[];
  rating: string;
  tours: string;
  since: string;
  /** Renders the card unfilled, as an invitation on the recruitment page. */
  blank?: boolean;
  tilt?: boolean;
  note?: string;
}

/**
 * The site's signature object. Everywhere else a marketplace would show a
 * photograph of scenery, ProGuideGH shows the credential — because the
 * credential is the thing being sold.
 */
export function Credential({
  name,
  licence,
  region,
  languages,
  specialties,
  rating,
  tours,
  since,
  blank = false,
  tilt = false,
  note,
}: CredentialProps) {
  return (
    <div>
      <article
        className={`cred${tilt ? " cred--tilt" : ""}${blank ? " cred--blank" : ""}`}
      >
        <div aria-hidden className="cred__strip" />
        <div className="cred__body">
          <div className="cred__top">
            <span className="cred__issuer">ProGuideGH · Certified guide</span>
            <span className="cred__status">{blank ? "Pending" : "Active"}</span>
          </div>

          <h3 className="cred__name">{name}</h3>
          <p className="cred__licence">Licence {licence}</p>

          <dl className="cred__rows">
            <div className="cred__row">
              <dt className="cred__key">Region</dt>
              <dd className="cred__val" style={{ margin: 0 }}>
                {region}
              </dd>
            </div>
            <div className="cred__row">
              <dt className="cred__key">Languages</dt>
              <dd className="cred__val" style={{ margin: 0 }}>
                {languages.join(" · ")}
              </dd>
            </div>
            <div className="cred__row">
              <dt className="cred__key">Specialties</dt>
              <dd className="cred__val" style={{ margin: 0 }}>
                {specialties.join(" · ")}
              </dd>
            </div>
          </dl>

          <div className="cred__foot">
            <div className="cred__metric">
              <b>{rating}</b>
              <span>Rating</span>
            </div>
            <div className="cred__metric">
              <b>{tours}</b>
              <span>Tours</span>
            </div>
            <div className="cred__metric">
              <b>{since}</b>
              <span>Guiding since</span>
            </div>
          </div>
        </div>
      </article>
      {note ? <p className="cred__note">{note}</p> : null}
    </div>
  );
}

export function SectionHead({
  eyebrow,
  title,
  children,
}: {
  eyebrow?: string;
  title: string;
  children?: ReactNode;
}) {
  return (
    <header className="section-head">
      {eyebrow ? <p className="eyebrow">{eyebrow}</p> : null}
      <h2 className="h2">{title}</h2>
      {children ? <p className="lede" style={{ marginTop: "1rem" }}>{children}</p> : null}
    </header>
  );
}
