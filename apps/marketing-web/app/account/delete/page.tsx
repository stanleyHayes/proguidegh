import type { Metadata } from "next";
import Link from "next/link";
import { APP_URL, getContent } from "../../lib/content";
import { Rule } from "../../components/site";

/**
 * Public account-deletion information page.
 *
 * Google Play requires an account-deletion route reachable without installing
 * the app; this is the URL submitted on the Data Safety form. The actual
 * deletion happens behind sign-in (identity has to be proven before we erase
 * anything), so this page's job is to state plainly what deletion does, what
 * survives it and why, and route the reader to somewhere they can act.
 */

export const metadata: Metadata = {
  title: "Delete your account",
  description:
    "How to permanently delete your ProGuideGH account and personal data, what is removed, and what is retained.",
  alternates: { canonical: "/account/delete" },
};

export default async function DeleteAccountInfoPage() {
  const content = await getContent();

  return (
    <>
      <section className="band band--field">
        <div className="wrap">
          <p className="eyebrow">Your data</p>
          <h1 className="h2" style={{ maxWidth: "20ch" }}>
            Delete your ProGuideGH account
          </h1>
          <p className="lede" style={{ marginTop: "1rem" }}>
            You can delete your account and personal data at any time, from the app or
            from the web. It is permanent and we do not ask you to email anyone to do it.
          </p>
        </div>
      </section>
      <Rule />

      <section className="band band--surface">
        <div className="wrap">
          <div className="grid grid--2">
            <article className="card">
              <h3>In the app</h3>
              <p>
                ProGuideGH or ProGuideGH&nbsp;Guide → <strong>Privacy &amp; data</strong> →{" "}
                <strong>Delete my account</strong>. In the traveller app, Privacy &amp;
                data sits under Profile; in the guide app it is on the dashboard.
              </p>
            </article>
            <article className="card">
              <h3>On the web</h3>
              <p>
                Sign in and open the deletion page. We verify it is your account before
                erasing anything — that check is the reason this page cannot simply have a
                button on it.
              </p>
              <p style={{ marginTop: "1rem" }}>
                <a className="btn btn--green" href={`${APP_URL}/account/delete`}>
                  Sign in and delete
                </a>
              </p>
            </article>
          </div>

          <h2 className="h3" style={{ marginTop: "3rem" }}>
            What is removed
          </h2>
          <ul className="prose" style={{ paddingLeft: "1.25rem", marginTop: "1rem" }}>
            <li>Your name, email address, phone number and password</li>
            <li>Your emergency contact details</li>
            <li>Any verification documents you uploaded, including the stored files</li>
            <li>Your payout account details</li>
            <li>Your location history</li>
            <li>Every active session — you are signed out immediately, everywhere</li>
          </ul>

          <h2 className="h3" style={{ marginTop: "2.5rem" }}>
            What is kept, and why
          </h2>
          <div className="prose" style={{ marginTop: "1rem" }}>
            <p>
              Payment records, receipts and ledger entries are retained. Ghanaian tax and
              tourism-levy obligations require us to keep them, and that obligation does
              not disappear because an account is closed. They reference an account
              identifier that, once your account is anonymised, no longer identifies you.
            </p>
            <p>
              Reviews you wrote stay visible, because other travellers rely on them when
              choosing a guide — but they are no longer linked to your name.
            </p>
          </div>

          <h2 className="h3" style={{ marginTop: "2.5rem" }}>
            When we ask you to wait
          </h2>
          <div className="prose" style={{ marginTop: "1rem" }}>
            <p>
              Deletion is refused, temporarily and with the reason shown, if you have a
              booking that has not finished or — for guides — earnings that have not been
              paid out yet. Complete or cancel the booking, or wait for the payout to
              settle, and the option becomes available.
            </p>
          </div>

          <h2 className="h3" style={{ marginTop: "2.5rem" }}>
            If you cannot sign in
          </h2>
          <div className="prose" style={{ marginTop: "1rem" }}>
            <p>
              Write to{" "}
              <a href={`mailto:${content.contact.privacyEmail}`} style={{ color: "var(--green)" }}>
                {content.contact.privacyEmail}
              </a>{" "}
              from the email address on the account. If you have lost access to that
              address too, say so and we will verify you another way before deleting
              anything.
            </p>
          </div>

          <div style={{ marginTop: "2.5rem", display: "flex", gap: "0.75rem", flexWrap: "wrap" }}>
            <Link className="btn btn--outline" href="/legal/privacy">
              Privacy policy
            </Link>
            <Link className="btn btn--outline" href="/safety">
              How we handle your data
            </Link>
          </div>
        </div>
      </section>
    </>
  );
}
