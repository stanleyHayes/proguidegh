import type { Metadata } from "next";
import Link from "next/link";
import { getContent } from "../lib/content";
import { Rule, SectionHead } from "../components/site";

export const metadata: Metadata = {
  title: "Safety",
  description:
    "How ProGuideGH vets guides, tracks active tours, handles SOS, and protects your data.",
  alternates: { canonical: "/safety" },
};

export default async function SafetyPage() {
  const content = await getContent();

  return (
    <>
      <section className="band band--field">
        <div className="wrap">
          <p className="eyebrow">Safety</p>
          <h1 className="h2" style={{ maxWidth: "20ch" }}>
            A marketplace that sends a stranger to meet you owes you more than a review
            score.
          </h1>
          <p className="lede" style={{ marginTop: "1rem" }}>
            This page describes what actually happens, not what we intend to do. If any of
            it stops being true, this page is wrong and we want to hear about it.
          </p>
        </div>
      </section>
      <Rule />

      <section className="band band--surface">
        <div className="wrap">
          <SectionHead eyebrow="Before you meet" title="Who gets to be on the platform" />
          <div className="grid grid--2">
            {[
              [
                "Government ID, verified",
                "Every guide submits a Ghana Card or passport. Documents go to private storage and are reachable only through short-lived links — they are never public files.",
              ],
              [
                "Background check",
                "Completed before certification, not after the first booking.",
              ],
              [
                "Ghana Tourism Authority registration",
                "The GTA licenses and regulates tour guides. We require evidence of it, and we record the reference on the guide's profile.",
              ],
              [
                "Expiry is enforced by the system",
                "Certification documents carry expiry dates. When one lapses the guide stops appearing in search that day. Nobody has to remember.",
              ],
            ].map(([title, detail]) => (
              <article className="card" key={title}>
                <h3 style={{ fontSize: "var(--step-0)", fontWeight: 700 }}>{title}</h3>
                <p style={{ marginTop: "0.35rem" }}>{detail}</p>
              </article>
            ))}
          </div>
        </div>
      </section>

      <section className="band band--paper">
        <div className="wrap">
          <SectionHead eyebrow="During the tour" title="What you can see, and what we can see">
            Tracking exists to make a meeting happen and to make an emergency answerable.
            It is deliberately narrow.
          </SectionHead>
          <div className="seq">
            {[
              [
                "Your guide's location is shared — yours is not",
                "The traveller is never tracked. Only the guide's position is shared, so you can find each other.",
              ],
              [
                "Only while the tour is live",
                "Sharing starts when your guide sets off and stops when the tour completes. Not before, not between tours, never when they are offline.",
              ],
              [
                "Only with the people involved",
                "You, for your booking, and our operations team. Guides' historical movements are never shown to travellers.",
              ],
              [
                "SOS reaches a person",
                "The SOS button on an active booking creates a high-priority incident with the freshest location, and it is acknowledged, escalated and closed by named staff — with every step on the record.",
              ],
            ].map(([title, detail]) => (
              <div className="seq__item" key={title}>
                <div>
                  <h3 className="h3">{title}</h3>
                  <p style={{ color: "var(--muted)", marginTop: "0.4rem" }}>{detail}</p>
                </div>
              </div>
            ))}
          </div>
          <p className="callout" style={{ marginTop: "2rem" }}>
            If you are in immediate danger, contact the Ghana Police Service on 191 or the
            National Ambulance Service on 193 first. The SOS button alerts ProGuideGH
            operations; it is not an emergency service.
          </p>
        </div>
      </section>
      <Rule />

      <section className="band band--surface">
        <div className="wrap">
          <SectionHead eyebrow="Your money and your data" title="The quieter half of safety" />
          <div className="grid grid--2">
            {[
              [
                "We never see your card",
                "Payment happens on Paystack's hosted page. Card and Mobile Money details never pass through ProGuideGH, and there is no screen in our apps that asks for them.",
              ],
              [
                "Every cedi is on an immutable ledger",
                "Payments, refunds, commission and the tourism levy are recorded as double-entry transactions that are never edited. Balances are derived from them, so they cannot silently drift.",
              ],
              [
                "You can take your data, or delete it",
                "Export everything we hold about you, or delete your account outright, from inside the app or from this website. Deletion is permanent.",
              ],
              [
                "Privileged actions are audited",
                "When a member of staff changes a role, resolves an incident or issues a refund, it is written to an append-only audit log with who did it and when.",
              ],
            ].map(([title, detail]) => (
              <article className="card" key={title}>
                <h3 style={{ fontSize: "var(--step-0)", fontWeight: 700 }}>{title}</h3>
                <p style={{ marginTop: "0.35rem" }}>{detail}</p>
              </article>
            ))}
          </div>

          <div style={{ marginTop: "2.5rem", display: "flex", gap: "0.75rem", flexWrap: "wrap" }}>
            <Link className="btn btn--outline" href="/legal/privacy">
              Privacy policy
            </Link>
            <Link className="btn btn--outline" href="/legal/location">
              Location sharing policy
            </Link>
            <Link className="btn btn--outline" href="/account/delete">
              Delete your account
            </Link>
          </div>

          <p style={{ marginTop: "2rem", color: "var(--muted)" }}>
            Something wrong, or a safety concern about a guide? Write to{" "}
            <a href={`mailto:${content.contact.supportEmail}`} style={{ color: "var(--green)" }}>
              {content.contact.supportEmail}
            </a>
            . Reports about an active tour are treated as incidents, not tickets.
          </p>
        </div>
      </section>
    </>
  );
}
