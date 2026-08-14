import type { Metadata } from "next";
import Link from "next/link";
import { getContent } from "../lib/content";
import { Rule } from "../components/site";

export const metadata: Metadata = {
  title: "Questions",
  description:
    "Common questions about booking a certified guide in Ghana: certification, pricing, safety, payment and data.",
  alternates: { canonical: "/faq" },
};

export default async function FaqPage() {
  const content = await getContent();

  return (
    <>
      <section className="band band--field">
        <div className="wrap">
          <p className="eyebrow">Before you book</p>
          <h1 className="h2" style={{ maxWidth: "18ch" }}>
            Questions people actually ask.
          </h1>
        </div>
      </section>
      <Rule />

      <section className="band band--surface">
        <div className="wrap">
          {content.faq.map((item) => (
            <details className="faq" key={item.question}>
              <summary>{item.question}</summary>
              <p>{item.answer}</p>
            </details>
          ))}

          <div style={{ marginTop: "2.5rem", display: "flex", gap: "0.75rem", flexWrap: "wrap" }}>
            <Link className="btn btn--outline" href="/safety">
              How safety works
            </Link>
            <Link className="btn btn--outline" href="/pricing">
              How pricing works
            </Link>
            <Link className="btn btn--outline" href="/contact">
              Ask us something else
            </Link>
          </div>
        </div>
      </section>
    </>
  );
}
