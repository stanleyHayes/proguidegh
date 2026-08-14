import type { Metadata } from "next";
import { APP_URL } from "../lib/content";
import { Rule, SectionHead } from "../components/site";

export const metadata: Metadata = {
  title: "Pricing",
  description:
    "How ProGuideGH prices tours: server-set rates, an itemised receipt, a published commission and the tourism levy.",
  alternates: { canonical: "/pricing" },
};

export default function PricingPage() {
  return (
    <>
      <section className="band band--field">
        <div className="wrap">
          <p className="eyebrow">Pricing</p>
          <h1 className="h2" style={{ maxWidth: "20ch" }}>
            The price is set before you pay, and itemised after.
          </h1>
          <p className="lede" style={{ marginTop: "1rem" }}>
            Haggling at the meeting point is the single most common complaint about guided
            tours anywhere. We removed the mechanism rather than the symptom: guides cannot
            set or change the price.
          </p>
        </div>
      </section>
      <Rule />

      <section className="band band--surface">
        <div className="wrap">
          <SectionHead eyebrow="For travellers" title="What you pay, and what it covers">
            A quote is calculated by ProGuideGH from the published rate for that tour
            package, the date, and the number of guests. It holds while you complete the
            booking.
          </SectionHead>

          <div className="grid grid--3">
            {[
              [
                "The tour",
                "The guide's fee for the package and duration you chose. This is the bulk of what you pay and the bulk of what the guide receives.",
              ],
              [
                "Platform commission",
                "ProGuideGH's share, which funds certification, the operations room, and the payment rails. Shown as its own line on the receipt.",
              ],
              [
                "Tourism levy",
                "Collected and remitted as required for tourism services in Ghana. Also its own line — we do not fold it into the headline price.",
              ],
            ].map(([title, detail]) => (
              <article className="card" key={title}>
                <h3 style={{ fontSize: "var(--step-0)", fontWeight: 700 }}>{title}</h3>
                <p style={{ marginTop: "0.35rem" }}>{detail}</p>
              </article>
            ))}
          </div>

          <div className="prose" style={{ marginTop: "2.5rem" }}>
            <p>
              <strong style={{ color: "var(--ink)" }}>No tipping expected.</strong> Guides
              are paid properly for the work. A tip is welcome and never assumed.
            </p>
            <p>
              <strong style={{ color: "var(--ink)" }}>No surge pricing.</strong> Rates are
              effective-dated rules. If a rate changes, bookings already made keep the
              price they were quoted.
            </p>
            <p>
              <strong style={{ color: "var(--ink)" }}>Refunds go back to source.</strong>{" "}
              Refunds are issued against the original payment as reversing ledger entries,
              not as platform credit.
            </p>
          </div>
        </div>
      </section>

      <section className="band band--paper" id="guides">
        <div className="wrap">
          <SectionHead eyebrow="For guides" title="What you earn, and when">
            The commission is published, applied identically to every guide, and shown on
            every booking before you accept it.
          </SectionHead>
          <div className="seq">
            {[
              [
                "You see the fee before accepting",
                "Every dispatched offer shows what the job pays. Declining costs nothing and does not affect your standing.",
              ],
              [
                "Earnings clear after completion",
                "Money becomes eligible for payout after the tour completes and a short hold passes, which protects both sides while a dispute could still be raised.",
              ],
              [
                "Weekly Mobile Money payouts",
                "Paid in a weekly batch to a payout account verified in your own name. Retries are idempotent — a failed transfer is never paid twice.",
              ],
              [
                "A statement behind every number",
                "Your wallet shows available, pending and paid, and each figure traces to ledger entries you can inspect.",
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
        </div>
      </section>
      <Rule />

      <section className="band band--surface">
        <div className="wrap">
          <SectionHead title="Rates" />
          <p className="prose">
            Current commission and levy percentages, cancellation windows and refund terms
            are published in the terms of service and shown on every quote and receipt.
            They are configuration, not code, so when they change the change is dated and
            applies from that date forward.
          </p>
          <div style={{ marginTop: "2rem", display: "flex", gap: "0.75rem", flexWrap: "wrap" }}>
            <a className="btn btn--green" href={`${APP_URL}/search`}>
              Get a quote
            </a>
            <a className="btn btn--outline" href="/legal/terms">
              Read the terms
            </a>
          </div>
        </div>
      </section>
    </>
  );
}
