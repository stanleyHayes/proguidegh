import type { Metadata } from "next";
import { getContent } from "../lib/content";
import { Rule, SectionHead } from "../components/site";

export const metadata: Metadata = {
  title: "Contact",
  description: "How to reach ProGuideGH — support, safety, privacy and partnerships.",
  alternates: { canonical: "/contact" },
};

export default async function ContactPage() {
  const { contact } = await getContent();

  return (
    <>
      <section className="band band--field">
        <div className="wrap">
          <p className="eyebrow">Contact</p>
          <h1 className="h2" style={{ maxWidth: "18ch" }}>
            Reach a person, not a queue.
          </h1>
        </div>
      </section>
      <Rule />

      <section className="band band--surface">
        <div className="wrap">
          <SectionHead title="Where to write" />
          <div className="grid grid--3">
            <article className="card">
              <h3>Support</h3>
              <p>Bookings, payments, receipts and anything about your account.</p>
              <p style={{ marginTop: "0.75rem" }}>
                <a href={`mailto:${contact.supportEmail}`} style={{ color: "var(--green)", fontWeight: 600 }}>
                  {contact.supportEmail}
                </a>
              </p>
            </article>
            <article className="card">
              <h3>Privacy</h3>
              <p>Data access requests, deletion help, and questions about what we hold.</p>
              <p style={{ marginTop: "0.75rem" }}>
                <a href={`mailto:${contact.privacyEmail}`} style={{ color: "var(--green)", fontWeight: 600 }}>
                  {contact.privacyEmail}
                </a>
              </p>
            </article>
            <article className="card">
              <h3>Phone</h3>
              <p>Office hours, West Africa Time.</p>
              <p style={{ marginTop: "0.75rem" }}>
                <a href={`tel:${contact.phone.replace(/\s/g, "")}`} style={{ color: "var(--green)", fontWeight: 600 }}>
                  {contact.phone}
                </a>
              </p>
            </article>
          </div>

          <p className="callout" style={{ marginTop: "2.5rem" }}>
            During an active tour, use the SOS button in the app rather than email — it
            reaches our operations team immediately with your guide&apos;s location. In an
            emergency, contact the Ghana Police Service on 191 or the National Ambulance
            Service on 193 first.
          </p>

          <p style={{ marginTop: "2rem", color: "var(--muted)" }}>{contact.address}</p>
        </div>
      </section>
    </>
  );
}
