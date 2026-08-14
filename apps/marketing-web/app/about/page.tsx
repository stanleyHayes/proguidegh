import type { Metadata } from "next";
import { getContent } from "../lib/content";
import { Rule, SectionHead } from "../components/site";

export const metadata: Metadata = {
  title: "About",
  description:
    "ProGuideGH is a certified-guide marketplace for Ghana, built to raise service standards and put tourism income into the hands of young Ghanaians.",
  alternates: { canonical: "/about" },
};

export default async function AboutPage() {
  const content = await getContent();

  return (
    <>
      <section className="band band--field">
        <div className="wrap">
          <p className="eyebrow">About</p>
          <h1 className="h2" style={{ maxWidth: "22ch" }}>
            Ghana does not have a tourism demand problem. It has a trust problem.
          </h1>
          <p className="lede" style={{ marginTop: "1rem" }}>
            Over 1.3 million people arrived in 2025. The question was never whether they
            would come — it was whether the person who met them at the castle gate was
            qualified, fairly paid, and accountable to anyone.
          </p>
        </div>
      </section>
      <Rule />

      <section className="band band--surface">
        <div className="wrap split">
          <div>
            <SectionHead eyebrow="What we are" title="A marketplace with a licence check in the middle of it" />
            <div className="prose">
              <p>
                ProGuideGH connects travellers with tour guides who have been identity
                verified, background checked, registered with the Ghana Tourism Authority
                and trained in the specialties they work in. Guides are dispatched to jobs,
                paid weekly, and rated only by travellers who actually took the tour.
              </p>
              <p>
                Everything else in the product exists to make that arrangement trustworthy:
                server-set pricing so nobody haggles, live tracking so nobody is lost, an
                immutable ledger so no cedi is unaccounted for, and an audit trail on every
                privileged action our own staff take.
              </p>
            </div>
          </div>
          <div className="grid" style={{ gap: "1rem" }}>
            {[
              ["Guide knowledge gaps", "Certification, structured training, specialist and language profiles, and a review loop that feeds back into quality."],
              ["Unprofessional service", "Standard pricing, verified profiles, receipts, ratings and certification records."],
              ["Tourist safety", "Identity and background checks, active-tour tracking, an SOS workflow and staffed incident response."],
              ["Youth unemployment", "A transparent marketplace and dispatch engine that matches certified guides to paid work."],
            ].map(([problem, response]) => (
              <div className="swap" key={problem} style={{ paddingTop: "1rem" }}>
                <p className="swap__problem" style={{ fontSize: "var(--step-0)" }}>{problem}</p>
                <p className="swap__response">{response}</p>
              </div>
            ))}
          </div>
        </div>
      </section>

      <section className="band band--paper">
        <div className="wrap">
          <SectionHead eyebrow="Why it matters" title="Guiding as a career, not a hustle">
            The most valuable thing we can hand a young Ghanaian guide is not a booking.
            It is a verifiable record.
          </SectionHead>
          <div className="prose">
            <p>
              A guide who has completed three hundred tours at 4.9 stars, with certification
              in heritage interpretation and accessible tourism, has something that should
              open doors — at a hotel, a tour operator, or the Authority itself. Until now
              that record mostly lived in the memories of satisfied strangers.
            </p>
            <p>
              Every rating on ProGuideGH is attached to a completed, paid booking. That is a
              deliberate constraint: it makes the number harder to inflate, and it makes the
              record worth carrying.
            </p>
          </div>
        </div>
      </section>
      <Rule />

      <section className="band band--surface">
        <div className="wrap">
          <SectionHead eyebrow="Working with" title="Partners" />
          <div className="prose">
            <p>
              ProGuideGH is being built in alignment with the Ghana Tourism Authority, which
              licenses and regulates tour guides and tourism enterprises in Ghana. Guide
              certification on the platform requires GTA registration; we do not issue a
              parallel credential of our own invention.
            </p>
            <p>
              Partner and programme details are confirmed ahead of public launch. We would
              rather name nobody than imply an endorsement we have not been given.
            </p>
          </div>
          <p style={{ marginTop: "2rem", color: "var(--muted)" }}>
            Press or partnership enquiries:{" "}
            <a href={`mailto:${content.contact.supportEmail}`} style={{ color: "var(--green)" }}>
              {content.contact.supportEmail}
            </a>
          </p>
        </div>
      </section>
    </>
  );
}
