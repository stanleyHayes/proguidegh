import type { Metadata } from "next";
import Link from "next/link";
import { getContent } from "../lib/content";
import { Rule } from "../components/site";

export const metadata: Metadata = {
  title: "Destinations",
  description:
    "Certified guides in Accra, Cape Coast and Kumasi — what each city offers and what a local guide adds.",
  alternates: { canonical: "/destinations" },
};

export default async function DestinationsPage() {
  const { destinations } = await getContent();

  return (
    <>
      <section className="band band--field">
        <div className="wrap">
          <p className="eyebrow">Where we operate</p>
          <h1 className="h2" style={{ maxWidth: "18ch" }}>
            Three cities, covered properly.
          </h1>
          <p className="lede" style={{ marginTop: "1rem" }}>
            We would rather certify enough guides in three places to be reliable than list
            sixteen regions we cannot staff. Each city below has guides across several
            specialties and languages.
          </p>
        </div>
      </section>
      <Rule />

      {destinations.map((d, index) => (
        <section
          className={`band ${index % 2 === 0 ? "band--surface" : "band--paper"}`}
          key={d.slug}
          id={d.slug}
        >
          <div className="wrap split">
            <div>
              <p className="eyebrow">{d.region}</p>
              <h2 className="h2">{d.city}</h2>
              <p className="lede" style={{ marginTop: "0.75rem" }}>{d.tagline}</p>
              <p className="prose" style={{ marginTop: "1.25rem" }}>{d.blurb}</p>
              <ul className="tags">
                {d.bestFor.map((tag) => (
                  <li className="tag" key={tag}>
                    {tag}
                  </li>
                ))}
              </ul>
              <p style={{ marginTop: "1.5rem" }}>
                <Link className="btn btn--outline" href={`/destinations/${d.slug}`}>
                  More on {d.city}
                </Link>
              </p>
            </div>
            <div className="grid" style={{ gap: "1rem" }}>
              {d.highlights.map((h) => (
                <article className="card" key={h.name}>
                  <h3 style={{ fontSize: "var(--step-0)", fontWeight: 700 }}>{h.name}</h3>
                  <p style={{ marginTop: "0.35rem" }}>{h.detail}</p>
                </article>
              ))}
            </div>
          </div>
        </section>
      ))}
    </>
  );
}
