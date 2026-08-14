# ProGuideGH — marketing site

The public website at `proguidegh.com`. Next.js 16, statically prerendered,
content editable from admin-web without a redeploy.

## Why this app exists separately

Three reasons it is not a route in `tourist-web`:

1. **It owns the apex domain.** `legal_documents` points the mobile apps at
   `https://proguidegh.com/legal/*`, and the app stores require those to
   resolve. Store submission is blocked while they 404.
2. **Different audience, different job.** `tourist-web` serves people who have
   already decided; this serves people deciding — and Ghanaian guides deciding
   whether to apply.
3. **Different rendering profile.** Everything here is static or ISR with a
   five-minute revalidate. It should stay up and fast even if the API is down.

## Design

The site makes one argument: you are booking a **credentialed person**, not a
listing. So the signature object is a guide credential card — a licence, not a
photo of a beach — and it recurs on the recruitment page as a blank card the
reader is invited to fill.

Everything around the card stays quiet. Section seams are a 1px hairline —
low-opacity gold on the dark field, border-grey on light. Two brand colours
carry the page, deep green and a single gold accent; **red is reserved for
genuine danger states and appears nowhere decorative**.

The mark is a compass needle over a gold waypoint: wayfinding is the literal
job, and a needle still reads at 16px. It is defined once in
`components/logo.tsx`, and the favicon, Apple touch icon and Open Graph card
are all generated from it via `next/og` — no binary asset to drift.

Type: **Outfit** at two weights for display and body, plus **IBM Plex Mono**
for record data only (licence numbers, credential field labels, citations).
Self-hosted by `next/font`, so no runtime request leaves the page.

Brand colours are the same values as `@proguidegh/tokens`; the deep field tones
are marketing-only extensions.

## Content and the CMS

Content lives in `system_settings` under the key `marketing.site`, edited at
**admin-web → Content**, and read publicly from `GET /api/v1/content/marketing`.

- Reusing `system_settings` rather than adding CMS tables means every edit to
  public copy goes through the same audited, permission-gated endpoint as a
  policy change.
- `app/lib/content.ts` holds the shape **and the launch defaults**. If nothing
  is published, or the API is unreachable, the site renders the defaults rather
  than erroring.
- Published content is merged over the defaults section by section, so a
  partially edited document still renders.

### The stats band is deliberately conservative

`stats.verified` ships `false`. The site then shows externally sourced Ghana
tourism figures with citations, not our own traction. The numbers in the build
spec (2,140 guides, 8,420 tours/month) are stated baselines and Y1 targets, not
measured results — publishing them as achievements would be the kind of
unearned claim Appendix D treats as a launch blocker. Flip `verified` in admin
only once someone has checked real figures.

## Running it

```bash
pnpm --filter @proguidegh/marketing-web dev      # http://localhost:3003
pnpm --filter @proguidegh/marketing-web build
pnpm --filter @proguidegh/marketing-web typecheck
pnpm --filter @proguidegh/marketing-web lint
```

Environment: `NEXT_PUBLIC_API_URL`, `NEXT_PUBLIC_SITE_URL`,
`NEXT_PUBLIC_APP_URL` — see [`infra/vercel/README.md`](../../infra/vercel/README.md).

## Routes

| Route | Notes |
|---|---|
| `/` | Hero, problem/response, stats, booking sequence, destinations, safety, guide CTA, FAQ |
| `/destinations`, `/destinations/[slug]` | Accra, Cape Coast, Kumasi — prerendered from content |
| `/become-a-guide` | Supply side: certification stages, terms, who we turn down |
| `/safety` | Vetting, tracking limits, SOS, money and data |
| `/pricing` | Traveller and guide pricing |
| `/about`, `/faq`, `/contact` | |
| `/legal/[document]` | terms, privacy, location — **these URLs unblock store submission** |
| `/account/delete` | Play's required deletion route, reachable without the app |
| `/sitemap.xml`, `/robots.txt` | |

## Outstanding

The legal pages currently render an outline plus a clearly marked notice that
the document is with counsel. **They must carry the real approved text before
launch** (M-24) — a store reviewer following the link needs a policy, not a
table of contents.
