import type { Metadata } from "next";
import Link from "next/link";
import { notFound } from "next/navigation";
import { getContent, getPolicies } from "../../lib/content";
import { Rule } from "../../components/site";
import { Markdown } from "../../components/markdown";

/**
 * Legal documents (Phase M, M-24).
 *
 * These URLs are load-bearing: `legal_documents` points the mobile apps here,
 * and both app stores require a reachable privacy policy before they will
 * accept a submission.
 *
 * The text comes from the database, not this file, so it can be revised in
 * admin without a deploy — and the version shown here is the same version
 * recorded against a user's consent. While a version is unapproved the page
 * says so in a banner; that is driven by the `approved` column rather than by
 * anyone remembering to add a note.
 */

const TITLES = {
  terms: "Terms of service",
  privacy: "Privacy policy",
  location: "Location sharing policy",
} as const;

type DocumentKey = keyof typeof TITLES;

interface Params {
  params: Promise<{ document: string }>;
}

export function generateStaticParams() {
  return Object.keys(TITLES).map((document) => ({ document }));
}

export async function generateMetadata({ params }: Params): Promise<Metadata> {
  const { document } = await params;
  if (!(document in TITLES)) return { title: "Legal" };
  const policies = await getPolicies();
  const published = policies.find((p) => p.document === document);
  return {
    title: TITLES[document as DocumentKey],
    description: published?.summary ?? TITLES[document as DocumentKey],
    alternates: { canonical: `/legal/${document}` },
    // An unapproved draft should not be the canonical answer in search.
    robots: published?.approved ? undefined : { index: false, follow: true },
  };
}

export default async function LegalPage({ params }: Params) {
  const { document } = await params;
  if (!(document in TITLES)) notFound();
  const key = document as DocumentKey;

  const [policies, content] = await Promise.all([getPolicies(), getContent()]);
  const published = policies.find((p) => p.document === document);

  return (
    <>
      <section className="band band--field">
        <div className="wrap">
          <p className="eyebrow">Legal</p>
          <h1 className="h2" style={{ maxWidth: "18ch" }}>
            {TITLES[key]}
          </h1>
          {published?.summary ? (
            <p className="lede" style={{ marginTop: "1rem" }}>{published.summary}</p>
          ) : null}
          {published ? (
            <p className="doc__version">
              VERSION {published.version}
              {published.approved ? "" : " · DRAFT"}
            </p>
          ) : null}
        </div>
      </section>
      <Rule onField />

      <section className="band band--surface">
        <div className="wrap">
          {published && !published.approved ? (
            <div className="callout" style={{ marginBottom: "2.5rem" }}>
              <strong style={{ color: "var(--ink)" }}>Draft — pending legal review.</strong>{" "}
              This version describes how ProGuideGH actually works today, but it has not
              yet been approved by our legal counsel and is not the final published
              policy. Questions in the meantime go to{" "}
              <a href={`mailto:${content.contact.privacyEmail}`} style={{ color: "var(--green)" }}>
                {content.contact.privacyEmail}
              </a>
              .
            </div>
          ) : null}

          {published?.body ? (
            <Markdown source={published.body} />
          ) : (
            <div className="callout">
              <strong style={{ color: "var(--ink)" }}>This document is not published yet.</strong>{" "}
              Write to{" "}
              <a href={`mailto:${content.contact.privacyEmail}`} style={{ color: "var(--green)" }}>
                {content.contact.privacyEmail}
              </a>{" "}
              with any question and we will answer it directly.
            </div>
          )}

          <div
            style={{
              marginTop: "3rem",
              paddingTop: "2rem",
              borderTop: "1px solid var(--border)",
              display: "flex",
              gap: "0.75rem",
              flexWrap: "wrap",
            }}
          >
            {(Object.keys(TITLES) as DocumentKey[])
              .filter((other) => other !== key)
              .map((other) => (
                <Link className="btn btn--outline" href={`/legal/${other}`} key={other}>
                  {TITLES[other]}
                </Link>
              ))}
            <Link className="btn btn--outline" href="/account/delete">
              Delete your account
            </Link>
          </div>
        </div>
      </section>
    </>
  );
}
