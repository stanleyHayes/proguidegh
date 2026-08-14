import type { Metadata } from "next";
import Link from "next/link";
import Image from "next/image";
import { notFound } from "next/navigation";
import { APP_URL, getContent, SITE_URL } from "../../lib/content";
import { destinationImages } from "../../lib/images";
import { Rule, SectionHead } from "../../components/site";

interface Params {
  params: Promise<{ slug: string }>;
}

/** Pre-render the launch cities; new destinations render on demand. */
export async function generateStaticParams() {
  const { destinations } = await getContent();
  return destinations.map((d) => ({ slug: d.slug }));
}

export async function generateMetadata({ params }: Params): Promise<Metadata> {
  const { slug } = await params;
  const { destinations } = await getContent();
  const d = destinations.find((entry) => entry.slug === slug);
  if (!d) return { title: "Destination" };
  return {
    title: `Certified tour guides in ${d.city}`,
    description: `${d.tagline}. Book a licensed, background-checked guide in ${d.city}, ${d.region}.`,
    alternates: { canonical: `/destinations/${d.slug}` },
  };
}

export default async function DestinationPage({ params }: Params) {
  const { slug } = await params;
  const { destinations } = await getContent();
  const d = destinations.find((entry) => entry.slug === slug);
  if (!d) notFound();

  const others = destinations.filter((entry) => entry.slug !== slug);

  // Breadcrumbs give search engines the site hierarchy and earn the
  // Home › Destinations › City trail in results.
  const breadcrumbs = {
    "@context": "https://schema.org",
    "@type": "BreadcrumbList",
    itemListElement: [
      { "@type": "ListItem", position: 1, name: "Home", item: SITE_URL },
      { "@type": "ListItem", position: 2, name: "Destinations", item: `${SITE_URL}/destinations` },
      { "@type": "ListItem", position: 3, name: d.city, item: `${SITE_URL}/destinations/${d.slug}` },
    ],
  };

  // A city page is a place page; naming the attractions gives the entities
  // something to attach to.
  const place = {
    "@context": "https://schema.org",
    "@type": "TouristDestination",
    name: d.city,
    description: d.blurb,
    address: { "@type": "PostalAddress", addressRegion: d.region, addressCountry: "GH" },
    touristType: d.bestFor,
    includesAttraction: d.highlights.map((h) => ({
      "@type": "TouristAttraction",
      name: h.name,
      description: h.detail,
    })),
  };

  return (
    <>
      <script
        type="application/ld+json"
        dangerouslySetInnerHTML={{
          __html: JSON.stringify(breadcrumbs).replace(/</g, "\\u003c"),
        }}
      />
      <script
        type="application/ld+json"
        dangerouslySetInnerHTML={{
          __html: JSON.stringify(place).replace(/</g, "\\u003c"),
        }}
      />
      <section className="band band--field">
        <div className="wrap">
          <p className="eyebrow">{d.region}</p>
          <h1 className="display" style={{ color: "var(--surface)", maxWidth: "14ch" }}>
            {d.city}
          </h1>
          <p className="lede" style={{ marginTop: "1rem" }}>{d.tagline}</p>
          <div className="hero__actions">
            <a className="btn btn--gold" href={`${APP_URL}/search?region=${d.slug}`}>
              Find a guide in {d.city}
            </a>
          </div>
        </div>
      </section>
      <Rule />

      <section className="band band--surface">
        <div className="wrap split">
          <div>
            <div className="destination-feature__media"><Image src={destinationImages[d.slug]!.src} alt={destinationImages[d.slug]!.alt} fill priority sizes="(min-width: 768px) 48vw, 100vw" /></div>
            <SectionHead eyebrow="What a guide adds" title={`Going with someone who knows ${d.city}`} />
            <p className="prose">{d.blurb}</p>
            <ul className="tags">
              {d.bestFor.map((tag) => (
                <li className="tag" key={tag}>
                  {tag}
                </li>
              ))}
            </ul>
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

      <section className="band band--paper">
        <div className="wrap">
          <SectionHead eyebrow="Also on ProGuideGH" title="Other cities" />
          <div className="grid grid--2">
            {others.map((o) => (
              <article className="card" key={o.slug}>
                <p className="eyebrow" style={{ marginBottom: "0.4rem" }}>{o.region}</p>
                <h3>{o.city}</h3>
                <p>{o.tagline}</p>
                <p style={{ marginTop: "0.9rem" }}>
                  <Link href={`/destinations/${o.slug}`} style={{ color: "var(--green)", fontWeight: 600 }}>
                    Guides in {o.city} →
                  </Link>
                </p>
              </article>
            ))}
          </div>
        </div>
      </section>
    </>
  );
}
