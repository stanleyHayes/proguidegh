/**
 * Marketing content model (Phase M, M-25).
 *
 * Content lives in `system_settings` under the key `marketing.site` and is
 * edited from admin-web. This file holds the shape, the defaults the site
 * falls back to when nothing has been published yet, and the fetcher.
 *
 * Why defaults live in code rather than a seed migration: the site must render
 * correctly on a cold environment with no database row, and a marketing page
 * that 500s because a settings key is missing is worse than one showing the
 * launch copy.
 *
 * IMPORTANT — the `stats` block ships with `verified: false`. Spec Appendix D
 * forbids unearned claims and §33 requires synthetic data out of production.
 * The figures in the spec (2,140 guides, 8,420 tours/month) are stated
 * baselines and Y1 targets, not measured traction, so the site does not print
 * them as achievements until someone sets `verified: true` in admin having
 * checked them. Until then the stats band renders the Ghana-tourism context
 * figures, which are externally sourced and cited.
 */

export interface Stat {
  value: string;
  label: string;
  /** Shown as a source note; required for any externally sourced figure. */
  source?: string;
}

export interface Destination {
  slug: string;
  city: string;
  region: string;
  tagline: string;
  blurb: string;
  highlights: { name: string; detail: string }[];
  bestFor: string[];
}

export interface FaqItem {
  question: string;
  answer: string;
}

export interface MarketingContent {
  hero: {
    eyebrow: string;
    headline: string;
    subhead: string;
    primaryCta: { label: string; href: string };
    secondaryCta: { label: string; href: string };
  };
  /** The credential card rendered in the hero. Illustrative, not a real guide. */
  sampleCredential: {
    name: string;
    licence: string;
    region: string;
    languages: string[];
    specialties: string[];
    rating: string;
    tours: string;
    since: string;
  };
  stats: {
    verified: boolean;
    items: Stat[];
  };
  destinations: Destination[];
  faq: FaqItem[];
  contact: {
    supportEmail: string;
    privacyEmail: string;
    phone: string;
    address: string;
  };
}

/** Launch content. Editable in admin → Content. */
export const DEFAULT_CONTENT: MarketingContent = {
  hero: {
    eyebrow: "Accra · Cape Coast · Kumasi",
    headline: "Every guide is licensed, checked, and tracked for the whole tour.",
    subhead:
      "ProGuideGH books you a certified Ghanaian guide — not a stranger at the gate. You see who they are before you pay, and our operations room sees where they are until you're safely done.",
    primaryCta: { label: "Find a guide", href: "https://app.proguidegh.com/search" },
    secondaryCta: { label: "Become a guide", href: "/become-a-guide" },
  },
  sampleCredential: {
    name: "Ama Owusu",
    licence: "GTA-TG-0000",
    region: "Central Region",
    languages: ["English", "Twi", "Fante"],
    specialties: ["Heritage & History", "Photography Tours"],
    rating: "4.9",
    tours: "312",
    since: "2024",
  },
  stats: {
    // Flip to true only after Marketing/Finance verify our own numbers.
    verified: false,
    items: [
      {
        value: "1.31M",
        label: "international arrivals in Ghana, 2025",
        source: "Ghana Tourism Authority 2025 Tourism Report",
      },
      {
        value: "7,109",
        label: "licensed tourism enterprises, 2025",
        source: "Ghana Tourism Authority 2025 Tourism Report",
      },
      {
        value: "31%",
        label: "of arrivals travel for business",
        source: "Ghana Tourism Authority 2025 Tourism Report",
      },
      {
        value: "13",
        label: "guide specialties, from heritage to accessible tourism",
      },
    ],
  },
  destinations: [
    {
      slug: "accra",
      city: "Accra",
      region: "Greater Accra",
      tagline: "The capital, and the country's front door",
      blurb:
        "Most visits to Ghana begin here. Accra rewards a guide who knows which market to enter from which side, and which stretch of the coast is worth your afternoon. It is also where business travel lands — nearly a third of arrivals to Ghana come for work, and a half-day with a guide turns a gap between meetings into the reason you remember the trip.",
      highlights: [
        {
          name: "Jamestown",
          detail:
            "Ghana's oldest district: the lighthouse, the boxing gyms, the fishing harbour at the hour the boats come in.",
        },
        {
          name: "Makola Market",
          detail:
            "The city's commercial heart. Overwhelming alone; navigable and generous with someone who trades there.",
        },
        {
          name: "Kwame Nkrumah Memorial Park",
          detail:
            "Where independence was declared. The context is the point, and context is what a guide is for.",
        },
      ],
      bestFor: ["City Tours", "Food & Culinary", "Business/Conference Support", "Nightlife"],
    },
    {
      slug: "cape-coast",
      city: "Cape Coast",
      region: "Central Region",
      tagline: "Where the history is heaviest",
      blurb:
        "Cape Coast Castle and Elmina are not sights you tick off. They are places where the guide standing next to you decides what the visit becomes. Our guides here are certified in heritage interpretation, and many are from the communities that have lived beside these walls for generations. Kakum's rainforest is an hour inland, which makes a single day here span the hardest history and the highest canopy in West Africa.",
      highlights: [
        {
          name: "Cape Coast Castle",
          detail:
            "A UNESCO World Heritage site and a central place in the transatlantic slave trade. Guided tours take about an hour through the dungeons.",
        },
        {
          name: "Elmina Castle",
          detail:
            "The oldest European building south of the Sahara, a short drive along the coast.",
        },
        {
          name: "Kakum National Park",
          detail:
            "360 km² of rainforest with a 350-metre canopy walkway on seven bridges, 27 metres up.",
        },
      ],
      bestFor: ["Heritage & History", "Nature & Ecotourism", "Family Tours", "Photography Tours"],
    },
    {
      slug: "kumasi",
      city: "Kumasi",
      region: "Ashanti Region",
      tagline: "The Ashanti capital, still very much a capital",
      blurb:
        "Kumasi is a living seat of power, not a museum of one. The distinction matters, and it is the difference a guide makes: knowing when the Manhyia Palace is ceremonial and when it is operational, how to move through Kejetia, and what is expected of you if you are invited to a funeral — which, in Kumasi, is a genuine possibility on a Saturday.",
      highlights: [
        {
          name: "Manhyia Palace Museum",
          detail: "Seat of the Asantehene and the record of the Ashanti Kingdom.",
        },
        {
          name: "Kejetia Market",
          detail:
            "Around 12 hectares and one of the largest markets in West Africa. Go with someone.",
        },
        {
          name: "Craft villages",
          detail:
            "Bonwire for kente weaving, Ntonso for adinkra printing, Ahwiaa for woodcarving.",
        },
      ],
      bestFor: ["Culture & Arts", "Heritage & History", "City Tours", "Multi-region Tours"],
    },
  ],
  faq: [
    {
      question: "What does “certified” actually mean?",
      answer:
        "A guide only becomes bookable after they complete our certification pipeline: identity verification, a background check, proof of Ghana Tourism Authority registration, and the training modules for their specialties. Documents are re-checked when they expire. A guide whose certification lapses stops appearing in search that day — not at the end of the month.",
    },
    {
      question: "How do I know the price is the real price?",
      answer:
        "The price is calculated by ProGuideGH, not by the guide, and it is fixed before you pay. Your receipt breaks out the tour cost, the platform fee and the tourism levy. There is no cash negotiation at the meeting point, and tipping is never expected.",
    },
    {
      question: "What happens during the tour?",
      answer:
        "Once your guide starts travelling to meet you, you can see them approaching. During the tour you have an SOS button that reaches our operations team with your location. When the tour is complete you get a receipt and can leave a review that only you, as the person who took the tour, are able to write.",
    },
    {
      question: "Is my location tracked?",
      answer:
        "No. Only the guide's location is shared, and only while they are online or on an active tour, so that you can find each other and so we can reach them in an emergency. It stops when the tour ends.",
    },
    {
      question: "How do I pay?",
      answer:
        "On Paystack's secure checkout page, by card or Mobile Money. ProGuideGH never sees or stores your card or MoMo details — we only receive confirmation that payment succeeded.",
    },
    {
      question: "Can I book a guide for business travel?",
      answer:
        "Yes. Business/Conference Support is one of our guide specialties: airport transfers, meeting logistics, translation, and something worth doing with a free afternoon.",
    },
    {
      question: "What if something goes wrong?",
      answer:
        "Raise it from the booking in the app and it becomes an incident with a named responder on our side. Refunds follow a published policy and are issued against the original payment, not as credit.",
    },
  ],
  contact: {
    supportEmail: "support@proguidegh.com",
    privacyEmail: "privacy@proguidegh.com",
    phone: "+233 30 000 0000",
    address: "Accra, Ghana",
  },
};

const API_BASE = (process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080").replace(
  /\/+$/,
  "",
);

/**
 * Published content, falling back to the defaults above.
 *
 * Deliberately fail-soft: a marketing page that renders launch copy is better
 * than one that 500s because the API is briefly unreachable. Revalidated
 * rather than cached forever so an admin edit appears without a redeploy.
 */
export async function getContent(): Promise<MarketingContent> {
  try {
    const res = await fetch(`${API_BASE}/api/v1/content/marketing`, {
      next: { revalidate: 300 },
    });
    if (!res.ok) return DEFAULT_CONTENT;
    const body: unknown = await res.json();
    const published = (body as { content?: Partial<MarketingContent> })?.content;
    if (!published || typeof published !== "object") return DEFAULT_CONTENT;
    // Shallow merge by section: a partially published document still renders,
    // and a section nobody has touched keeps its default.
    return { ...DEFAULT_CONTENT, ...published };
  } catch {
    return DEFAULT_CONTENT;
  }
}

export interface LegalDocument {
  document: string;
  version: string;
  url: string;
  /** One-line description, shown as the page lede and meta description. */
  summary?: string;
  /** Markdown body. Absent for legacy rows seeded before the text existed. */
  body?: string;
  /** False until counsel signs off; drives the draft banner and noindex. */
  approved: boolean;
  approved_at?: string;
  published_at?: string;
}

/** Published policy versions, straight from the API that the apps also use. */
export async function getPolicies(): Promise<LegalDocument[]> {
  try {
    const res = await fetch(`${API_BASE}/api/v1/legal/policies`, {
      next: { revalidate: 300 },
    });
    if (!res.ok) return [];
    const body: unknown = await res.json();
    const policies = (body as { policies?: LegalDocument[] })?.policies;
    return Array.isArray(policies) ? policies : [];
  } catch {
    return [];
  }
}

export const SITE_URL = (
  process.env.NEXT_PUBLIC_SITE_URL ?? "https://proguidegh.com"
).replace(/\/+$/, "");

export const APP_URL = (
  process.env.NEXT_PUBLIC_APP_URL ?? "https://app.proguidegh.com"
).replace(/\/+$/, "");
