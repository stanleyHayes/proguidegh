import Link from "next/link";
import { APP_URL, getContent, SITE_URL } from "./lib/content";
import { Credential, Rule, SectionHead } from "./components/site";

/**
 * JSON-LD is injected as raw HTML, and the FAQ text is admin-editable, so an
 * editor could otherwise close the script tag from inside a string. Escaping
 * `<` blocks that; JSON.stringify handles quoting for everything else.
 */
function jsonLd(data: unknown): string {
  return JSON.stringify(data).replace(/</g, "\\u003c");
}

/**
 * Home. One argument, made in order: you are booking a credentialed person
 * (hero) → here is what that fixes (problems) → here is how it runs (sequence)
 * → here is where (destinations) → here is what happens if it goes wrong
 * (safety) → and if you're Ghanaian, here is a job (guides).
 *
 * The problem/response pairs are the platform's own stated purpose from the
 * build spec §2.1, not invented marketing claims.
 */

const PROBLEMS: { problem: string; response: string }[] = [
  {
    problem: "You cannot tell who is qualified.",
    response:
      "Every guide completes identity verification, a background check and Ghana Tourism Authority registration before they are bookable. Their licence, languages and specialties are on their profile before you pay — and a guide whose certification lapses leaves search the same day.",
  },
  {
    problem: "The price changes when you arrive.",
    response:
      "ProGuideGH sets the price, not the guide. It is fixed before payment and itemised on your receipt, down to the platform fee and the tourism levy. No negotiation at the meeting point, no expected tip.",
  },
  {
    problem: "Nobody knows where you are.",
    response:
      "From the moment your guide sets off you can see them approaching, and an SOS button on the active booking reaches our operations team with a location. Tracking covers the guide, during the tour, and stops when it ends.",
  },
  {
    problem: "Good guides have no way to prove it.",
    response:
      "Ratings attach to completed, paid bookings — only the person who took the tour can review it. Guides build a record that travels with them, get paid weekly by Mobile Money, and can train into higher-value specialties.",
  },
];

const STEPS: { title: string; body: string }[] = [
  {
    title: "Search for what you actually want",
    body: "Filter by city, language, and specialty — heritage, food, photography, business support, accessible tourism. You are choosing a person, not a package.",
  },
  {
    title: "See the price before you commit",
    body: "The quote is calculated server-side from published rates and holds while you book. What you see is what is charged.",
  },
  {
    title: "Pay on Paystack, not in the app",
    body: "Card or Mobile Money on Paystack's secure page. ProGuideGH never sees your card details. Your booking confirms when the payment does.",
  },
  {
    title: "Meet your guide",
    body: "You get their name, photo and licence in advance, and you can watch them travel to the meeting point.",
  },
  {
    title: "Take the tour — tracked",
    body: "The tour moves through clear stages, and the SOS button is there for the whole of it. Our operations room is staffed while tours are running.",
  },
  {
    title: "Get your receipt, leave a review",
    body: "An itemised receipt lands when the tour completes. Your review is tied to that booking, which is why the ratings mean something.",
  },
];

export default async function HomePage() {
  const content = await getContent();
  const { hero, sampleCredential, stats, destinations, faq } = content;

  // Surfacing the FAQ as structured data: these are the exact questions people
  // type into a search engine before booking a guide abroad.
  const faqJsonLd = {
    "@context": "https://schema.org",
    "@type": "FAQPage",
    mainEntity: faq.slice(0, 6).map((item) => ({
      "@type": "Question",
      name: item.question,
      acceptedAnswer: { "@type": "Answer", text: item.answer },
    })),
  };

  const orgJsonLd = {
    "@context": "https://schema.org",
    "@type": "Organization",
    name: "ProGuideGH",
    url: SITE_URL,
    description:
      "Marketplace for certified tourist guides in Ghana, covering Accra, Cape Coast and Kumasi.",
    areaServed: { "@type": "Country", name: "Ghana" },
    email: content.contact.supportEmail,
  };

  return (
    <>
      <script
        type="application/ld+json"
        dangerouslySetInnerHTML={{ __html: jsonLd(orgJsonLd) }}
      />
      <script
        type="application/ld+json"
        dangerouslySetInnerHTML={{ __html: jsonLd(faqJsonLd) }}
      />

      {/* ---------------------------------------------------------- hero */}
      <section className="hero">
        <div className="wrap hero__grid">
          <div className="reveal">
            <p className="eyebrow">{hero.eyebrow}</p>
            <h1 className="hero__headline">{hero.headline}</h1>
            <p className="hero__sub">{hero.subhead}</p>
            <div className="hero__actions">
              <a className="btn btn--gold" href={hero.primaryCta.href}>
                {hero.primaryCta.label}
              </a>
              <Link className="btn btn--ghost" href={hero.secondaryCta.href}>
                {hero.secondaryCta.label}
              </Link>
            </div>
          </div>

          <div className="reveal" style={{ animationDelay: "120ms" }}>
            <Credential
              tilt
              name={sampleCredential.name}
              licence={sampleCredential.licence}
              region={sampleCredential.region}
              languages={sampleCredential.languages}
              specialties={sampleCredential.specialties}
              rating={sampleCredential.rating}
              tours={sampleCredential.tours}
              since={sampleCredential.since}
              note="Illustrative credential. Every guide's real licence is on their profile."
            />
          </div>
        </div>
      </section>
      <Rule />

      {/* ------------------------------------------------------ problems */}
      <section className="band band--surface">
        <div className="wrap">
          <SectionHead eyebrow="Why we exist" title="Four things that go wrong, and what we did about them">
            Ghana had over 1.3 million international arrivals in 2025. The gap has never
            been demand — it has been the distance between a good guide and a traveller
            who can tell.
          </SectionHead>

          <div>
            {PROBLEMS.map((item) => (
              <div className="swap" key={item.problem}>
                <p className="swap__problem">{item.problem}</p>
                <p className="swap__response">{item.response}</p>
              </div>
            ))}
          </div>
        </div>
      </section>

      {/* ---------------------------------------------------------- stats */}
      <section className="band band--field">
        <div className="wrap">
          <SectionHead eyebrow="Context" title="The market we are building into">
            {stats.verified
              ? "Our numbers, updated from the platform."
              : "Ghana's tourism sector, from the national statistics. We publish our own numbers once they are audited — not before."}
          </SectionHead>
          <div className="grid grid--4">
            {stats.items.map((stat) => (
              <div className="stat" key={stat.label}>
                <b>{stat.value}</b>
                <span>{stat.label}</span>
                {stat.source ? <cite>{stat.source}</cite> : null}
              </div>
            ))}
          </div>
        </div>
      </section>
      <Rule />

      {/* ------------------------------------------------------- sequence */}
      <section className="band band--paper">
        <div className="wrap">
          <SectionHead eyebrow="How a booking works" title="Six steps, and you can see all of them">
            Numbered because it is genuinely a sequence — each step depends on the one
            before it, and the money only moves at step three.
          </SectionHead>
          <div className="seq">
            {STEPS.map((step) => (
              <div className="seq__item" key={step.title}>
                <div>
                  <h3 className="h3">{step.title}</h3>
                  <p style={{ color: "var(--muted)", marginTop: "0.4rem" }}>{step.body}</p>
                </div>
              </div>
            ))}
          </div>
        </div>
      </section>

      {/* --------------------------------------------------- destinations */}
      <section className="band band--surface">
        <div className="wrap">
          <SectionHead eyebrow="Where we operate" title="Three cities to begin with">
            We launch where we can certify enough guides to be reliable, rather than
            listing the whole country and hoping. More regions follow the same standard.
          </SectionHead>

          <div className="grid grid--3">
            {destinations.map((d) => (
              <article className="card" key={d.slug}>
                <p className="eyebrow" style={{ marginBottom: "0.5rem" }}>{d.region}</p>
                <h3>{d.city}</h3>
                <p style={{ marginBottom: "0.75rem" }}>{d.tagline}</p>
                <ul className="tags">
                  {d.bestFor.slice(0, 3).map((tag) => (
                    <li className="tag" key={tag}>
                      {tag}
                    </li>
                  ))}
                </ul>
                <p style={{ marginTop: "1.1rem" }}>
                  <Link href={`/destinations/${d.slug}`} style={{ color: "var(--green)", fontWeight: 600 }}>
                    What a guide adds in {d.city} →
                  </Link>
                </p>
              </article>
            ))}
          </div>
        </div>
      </section>
      <Rule />

      {/* --------------------------------------------------------- safety */}
      <section className="band band--paper">
        <div className="wrap split">
          <div>
            <SectionHead eyebrow="Safety" title="The part we take most seriously">
              A marketplace that sends a stranger to meet you has an obligation that ends
              only when the tour does.
            </SectionHead>
            <p className="callout">
              Every active tour has an SOS button. Pressing it creates an incident with
              your location and a named responder on our side — not a support ticket in a
              queue.
            </p>
            <p style={{ marginTop: "1.5rem" }}>
              <Link className="btn btn--outline" href="/safety">
                How safety works
              </Link>
            </p>
          </div>
          <div className="grid" style={{ gap: "1rem" }}>
            {[
              ["Verified identity", "Government ID and a background check before certification."],
              ["Live tour tracking", "You see your guide approaching, and during the tour."],
              ["Operations response", "Incidents are acknowledged, escalated and closed by staff."],
              ["Audited money", "Every payment, refund and payout is written to an immutable ledger."],
            ].map(([title, detail]) => (
              <div className="card" key={title}>
                <h3 style={{ fontSize: "var(--step-0)", fontWeight: 700 }}>{title}</h3>
                <p style={{ marginTop: "0.35rem" }}>{detail}</p>
              </div>
            ))}
          </div>
        </div>
      </section>

      {/* --------------------------------------------------------- guides */}
      <section className="band band--field">
        <div className="wrap split">
          <div>
            <p className="eyebrow">For Ghanaian guides</p>
            <h2 className="h2">This card is a job.</h2>
            <p className="lede" style={{ marginTop: "1rem" }}>
              Certification, training into specialties that pay more, dispatched work you
              accept in one tap, and Mobile Money payouts every week. You keep the rating
              you earn.
            </p>
            <div className="hero__actions">
              <Link className="btn btn--gold" href="/become-a-guide">
                What it takes
              </Link>
              <a className="btn btn--ghost" href={`${APP_URL}/register`}>
                Apply
              </a>
            </div>
          </div>
          <div>
            <Credential
              blank
              name="Your name"
              licence="GTA-TG-——"
              region="Your region"
              languages={["Your languages"]}
              specialties={["Your specialties"]}
              rating="—"
              tours="0"
              since="2026"
              note="Issued when you complete certification."
            />
          </div>
        </div>
      </section>
      <Rule />

      {/* ------------------------------------------------------------ faq */}
      <section className="band band--surface">
        <div className="wrap">
          <SectionHead eyebrow="Before you book" title="Questions people actually ask" />
          <div>
            {faq.slice(0, 5).map((item) => (
              <details className="faq" key={item.question}>
                <summary>{item.question}</summary>
                <p>{item.answer}</p>
              </details>
            ))}
          </div>
          <p style={{ marginTop: "2rem" }}>
            <Link className="btn btn--outline" href="/faq">
              All questions
            </Link>
          </p>
        </div>
      </section>
    </>
  );
}
