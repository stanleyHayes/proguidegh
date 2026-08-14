import type { Metadata } from "next";
import { APP_URL, getContent } from "../lib/content";
import { Credential, Rule, SectionHead } from "../components/site";

export const metadata: Metadata = {
  title: "Become a certified guide",
  description:
    "Get certified, take dispatched tours, and get paid weekly by Mobile Money. What ProGuideGH certification requires and what it pays.",
  alternates: { canonical: "/become-a-guide" },
};

const STAGES: { stage: string; detail: string }[] = [
  {
    stage: "Apply",
    detail:
      "Create an account and start a guide application. You need a Ghana Card or passport, a phone number, and a bank or Mobile Money account in your own name.",
  },
  {
    stage: "Verify",
    detail:
      "We check your identity and run a background check. You upload your Ghana Tourism Authority registration and any qualifications. Documents are stored privately and are never public.",
  },
  {
    stage: "Train",
    detail:
      "Complete the modules for the specialties you want to work in. Heritage interpretation, accessible tourism and business support each unlock different work.",
  },
  {
    stage: "Get certified",
    detail:
      "An administrator reviews the case and activates you. From that moment your profile is visible in search and you can receive dispatched jobs.",
  },
  {
    stage: "Work",
    detail:
      "Go online when you want jobs. Offers arrive with the fee, the meeting point and the time, and you accept in one tap. You cannot be double-booked.",
  },
  {
    stage: "Get paid",
    detail:
      "Earnings clear after the tour completes and are paid out weekly to your Mobile Money account. Your wallet shows available, pending and paid, with a statement behind every figure.",
  },
];

export default async function BecomeAGuidePage() {
  const content = await getContent();

  return (
    <>
      <section className="hero">
        <div className="wrap hero__grid">
          <div>
            <p className="eyebrow">For Ghanaian guides</p>
            <h1 className="hero__headline">Get certified. Get dispatched. Get paid weekly.</h1>
            <p className="hero__sub">
              ProGuideGH exists partly to solve a supply problem: there are excellent
              guides in Ghana with no way to prove it to a stranger, and travellers who
              would happily pay more for someone qualified. Certification closes that gap,
              and the record you build belongs to you.
            </p>
            <div className="hero__actions">
              <a className="btn btn--gold" href={`${APP_URL}/register`}>
                Start your application
              </a>
              <a className="btn btn--ghost" href="#certification">
                What it takes
              </a>
            </div>
          </div>
          <div>
            <Credential
              tilt
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

      <section className="band band--surface" id="certification">
        <div className="wrap">
          <SectionHead eyebrow="Certification" title="Six stages, and you can see where you are">
            Nothing here is a black box. Your application shows the stage you are at and
            what is outstanding, and an administrator has to give a reason to reject
            anything.
          </SectionHead>
          <div className="seq">
            {STAGES.map((s) => (
              <div className="seq__item" key={s.stage}>
                <div>
                  <h3 className="h3">{s.stage}</h3>
                  <p style={{ color: "var(--muted)", marginTop: "0.4rem" }}>{s.detail}</p>
                </div>
              </div>
            ))}
          </div>
        </div>
      </section>

      <section className="band band--paper">
        <div className="wrap">
          <SectionHead eyebrow="What you get" title="The terms, stated plainly">
            We would rather you read this before applying than discover it afterwards.
          </SectionHead>
          <div className="grid grid--2">
            {[
              [
                "A published commission, not a mystery",
                "ProGuideGH takes a platform commission and remits the tourism levy. Both are itemised on every booking, and both are set as effective-dated rules — if they change, existing bookings keep the rate they were priced at.",
              ],
              [
                "Weekly Mobile Money payouts",
                "Earnings become eligible after a short hold once the tour completes, then pay out on a weekly batch. Retries are idempotent, so a failed transfer never pays you twice or loses the money.",
              ],
              [
                "Work you choose",
                "Going online is a switch you control. Offers show the fee before you accept, and declining costs you nothing.",
              ],
              [
                "A rating that is hard to fake",
                "Only a traveller who completed and paid for a tour with you can review it. That is why a 4.9 here means something.",
              ],
              [
                "Training that pays",
                "Specialties are not badges. Heritage interpretation, accessible tourism and business support attract different work at different rates.",
              ],
              [
                "Support when a tour goes wrong",
                "If a traveller raises an SOS or a dispute, a named person on our operations team handles it. You are not left to argue with a customer alone.",
              ],
            ].map(([title, detail]) => (
              <article className="card" key={title}>
                <h3>{title}</h3>
                <p>{detail}</p>
              </article>
            ))}
          </div>
        </div>
      </section>
      <Rule />

      <section className="band band--field">
        <div className="wrap">
          <SectionHead eyebrow="Honest about the bar" title="Who we turn down">
            Certification only means something if it can be withheld.
          </SectionHead>
          <div className="prose" style={{ color: "var(--muted-invert)" }}>
            <p>
              We decline applications that fail the background check, that cannot evidence
              Ghana Tourism Authority registration, or where identity documents do not
              match. We also remove certified guides whose documents expire and are not
              renewed — that happens automatically, on the day.
            </p>
            <p>
              If a guide's rating falls below our quality threshold they are flagged for
              retraining rather than quietly dropped, and there is a route back. Serious
              safety findings are different, and end the relationship.
            </p>
          </div>
          <p style={{ marginTop: "2rem" }}>
            <a className="btn btn--gold" href={`${APP_URL}/register`}>
              Apply to be certified
            </a>
          </p>
        </div>
      </section>

      <section className="band band--surface">
        <div className="wrap">
          <SectionHead title="Questions from guides" />
          {[
            {
              q: "Do I need to already be registered with the Ghana Tourism Authority?",
              a: "You need it to complete certification. If you are mid-process you can still apply and upload the registration when it arrives — your case waits at the verification stage rather than being rejected.",
            },
            {
              q: "Does it cost anything to join?",
              a: "There is no fee to apply or to be certified. ProGuideGH earns a commission on completed bookings, which means we only make money when you do.",
            },
            {
              q: "Can I still take my own private clients?",
              a: "Yes. ProGuideGH is not exclusive. Bookings you take here are tracked and insured through the platform; bookings you arrange privately are yours to manage.",
            },
            {
              q: "What happens to my location data?",
              a: "It is collected only while you are online or on an active tour, shared only with that tour's traveller and our operations team, and it stops when the tour ends. You can see the full policy before you turn it on.",
            },
          ].map((item) => (
            <details className="faq" key={item.q}>
              <summary>{item.q}</summary>
              <p>{item.a}</p>
            </details>
          ))}
          <p style={{ marginTop: "2rem", color: "var(--muted)" }}>
            Still deciding? Write to{" "}
            <a href={`mailto:${content.contact.supportEmail}`} style={{ color: "var(--green)" }}>
              {content.contact.supportEmail}
            </a>
            .
          </p>
        </div>
      </section>
    </>
  );
}
