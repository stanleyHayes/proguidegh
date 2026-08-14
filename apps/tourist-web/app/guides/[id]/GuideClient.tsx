"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import {
  Alert,
  Badge,
  Button,
  Card,
  DateTimeField,
  Input,
  Select,
  Textarea,
} from "@proguidegh/ui";
import { api, ApiError, errorMessage, unwrap } from "../../lib/api";
import ReviewsSection from "./ReviewsSection";
import {
  formatPrice,
  packageName,
  parsePackages,
  type TourPackage,
} from "../../lib/catalog";
import {
  guideName,
  labelOf,
  labelsOf,
  localToIso,
  type Booking,
  type Guide,
  type Quote,
} from "../../lib/bookings";

type LoadState = "loading" | "not-found" | "error" | "ready";
type QuoteState = "idle" | "loading" | "error" | "ready";

export default function GuideClient({ guideId }: { guideId: string }) {
  const router = useRouter();

  const [state, setState] = useState<LoadState>("loading");
  const [loadError, setLoadError] = useState<string | null>(null);
  const [guide, setGuide] = useState<Guide | null>(null);

  const [packages, setPackages] = useState<TourPackage[]>([]);

  const [packageId, setPackageId] = useState("");
  const [startsAt, setStartsAt] = useState("");
  const [guests, setGuests] = useState("2");
  const [meetingPoint, setMeetingPoint] = useState("");
  const [notes, setNotes] = useState("");

  const [quoteState, setQuoteState] = useState<QuoteState>("idle");
  const [quoteError, setQuoteError] = useState<string | null>(null);
  const [quote, setQuote] = useState<Quote | null>(null);

  const [submitting, setSubmitting] = useState(false);
  const [submitError, setSubmitError] = useState<string | null>(null);
  const [unauthenticated, setUnauthenticated] = useState(false);

  // One Idempotency-Key per booking draft; retries of the same submit reuse
  // it, and it is only regenerated after a booking is successfully created.
  const idempotencyKeyRef = useRef<string | null>(null);

  const load = useCallback(async () => {
    setState("loading");
    setLoadError(null);
    try {
      const data = await api<unknown>(`/guides/${guideId}`);
      setGuide(unwrap<Guide>(data, "guide"));
      setState("ready");
    } catch (err) {
      if (err instanceof ApiError && err.status === 404) {
        setState("not-found");
      } else {
        setLoadError(
          errorMessage(err, "Could not load this guide. Please retry."),
        );
        setState("error");
      }
    }
  }, [guideId]);

  useEffect(() => {
    void load();
    void api<unknown>("/tour-packages")
      .then((data) => parsePackages(data))
      .then(setPackages)
      .catch(() => {
        // Booking form shows an empty package select with an error on quote.
      });
  }, [load]);

  // Debounced server quote whenever package/date/guests are complete.
  useEffect(() => {
    const iso = localToIso(startsAt);
    const guestCount = Number(guests);
    if (!packageId || !iso || !Number.isInteger(guestCount) || guestCount < 1) {
      setQuoteState("idle");
      setQuote(null);
      setQuoteError(null);
      return;
    }
    setQuoteState("loading");
    setQuoteError(null);
    const timer = setTimeout(() => {
      void api<unknown>("/bookings/quote", {
        method: "POST",
        body: { package_id: packageId, starts_at: iso, guests: guestCount },
      })
        .then((data) => {
          setQuote(unwrap<Quote>(data, "quote"));
          setQuoteState("ready");
        })
        .catch((err: unknown) => {
          if (err instanceof ApiError && err.status === 401) {
            setUnauthenticated(true);
          }
          setQuote(null);
          setQuoteError(errorMessage(err, "Could not get a quote right now."));
          setQuoteState("error");
        });
    }, 500);
    return () => clearTimeout(timer);
  }, [packageId, startsAt, guests]);

  async function onConfirm(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const iso = localToIso(startsAt);
    if (!iso) return;
    if (!idempotencyKeyRef.current) {
      idempotencyKeyRef.current = crypto.randomUUID();
    }
    setSubmitting(true);
    setSubmitError(null);
    try {
      const data = await api<unknown>("/bookings", {
        method: "POST",
        headers: { "Idempotency-Key": idempotencyKeyRef.current },
        body: {
          package_id: packageId,
          guide_id: guideId,
          starts_at: iso,
          meeting_point: meetingPoint,
          guests: Number(guests),
          notes: notes || undefined,
        },
      });
      const booking = unwrap<Booking>(data, "booking");
      idempotencyKeyRef.current = null; // draft consumed
      router.push(`/checkout/${booking.id}`);
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        setUnauthenticated(true);
      }
      // 409 (overlap) and 422 messages are shown verbatim from the envelope.
      setSubmitError(
        errorMessage(err, "Could not create your booking. Please try again."),
      );
    } finally {
      setSubmitting(false);
    }
  }

  if (state === "loading") {
    return (
      <div className="stack" aria-busy="true" aria-label="Loading guide profile">
        <div className="gg-skeleton" style={{ height: "2rem", width: "40%" }} />
        <div className="gg-skeleton" style={{ height: "8rem" }} />
        <div className="gg-skeleton" style={{ height: "16rem" }} />
      </div>
    );
  }

  if (state === "not-found") {
    return (
      <div className="stack">
        <h1>Guide unavailable</h1>
        <Alert tone="info" title="This guide is not available">
          <p>
            This guide could not be found or is not currently bookable. Only
            certified, active guides appear on ProGuideGH.
          </p>
        </Alert>
        <p>
          <Link className="gg-button gg-button--secondary" href="/search">
            Back to search
          </Link>
        </p>
      </div>
    );
  }

  if (state === "error" || !guide) {
    return (
      <div className="stack">
        <h1>Guide profile</h1>
        <Alert tone="error" title="Something went wrong">
          <p>{loadError}</p>
        </Alert>
        <div>
          <Button type="button" onClick={() => void load()}>
            Retry
          </Button>
        </div>
      </div>
    );
  }

  const languages = labelsOf(guide.languages);
  const specialties = labelsOf(guide.specialties);
  const region = labelOf(guide.region);
  const quoteCurrency = quote?.currency;

  return (
    <div className="stack">
      <section aria-labelledby="guide-heading">
        <div className="badge-row">
          <Badge tone="success">Verified</Badge>
          {guide.elite_status ? <Badge tone="warning">Elite guide</Badge> : null}
        </div>
        <h1 id="guide-heading">{guideName(guide)}</h1>
        <p className="stars" aria-label={`Rated ${guide.rating_avg?.toFixed(1) ?? "—"} out of 5`}>
          <span aria-hidden="true">★</span>{" "}
          {guide.rating_avg && guide.rating_avg > 0
            ? guide.rating_avg.toFixed(1)
            : "—"}{" "}
          <span className="muted">
            ({guide.rating_count ?? 0} reviews · {guide.completed_tours ?? 0}{" "}
            completed tours)
          </span>
        </p>
        {region ? <p className="muted">{region}</p> : null}
      </section>

      <section aria-label="About this guide" className="stack">
        {guide.bio ? <p>{guide.bio}</p> : null}
        {languages.length > 0 ? (
          <p>
            <strong>Languages:</strong> {languages.join(", ")}
          </p>
        ) : null}
        {specialties.length > 0 ? (
          <div className="badge-row" aria-label="Specialties">
            {specialties.map((specialty) => (
              <Badge key={specialty} tone="neutral">
                {specialty}
              </Badge>
            ))}
          </div>
        ) : null}
      </section>

      <ReviewsSection guideId={guideId} />

      <Card title="Book this guide">
        {unauthenticated ? (
          <Alert tone="info" title="Sign in required">
            <p>
              You need to{" "}
              <Link href="/login">sign in</Link> to request a quote and book a
              tour.
            </p>
          </Alert>
        ) : null}

        <form className="stack" onSubmit={onConfirm}>
          <Select
            label="Tour package"
            name="package_id"
            required
            value={packageId}
            onChange={(e) => setPackageId(e.target.value)}
            disabled={submitting}
          >
            <option value="">Select a package</option>
            {packages.map((pkg) => (
              <option key={pkg.id} value={pkg.id}>
                {packageName(pkg)}
                {pkg.base_price !== undefined
                  ? ` — ${formatPrice(pkg.base_price, pkg.currency)}`
                  : ""}
              </option>
            ))}
          </Select>
          <DateTimeField
            label="Date & time"
            name="starts_at"
            required
            value={startsAt}
            onChange={setStartsAt}
          />
          <Input
            label="Guests"
            name="guests"
            type="number"
            min={1}
            max={20}
            required
            value={guests}
            onChange={(e) => setGuests(e.target.value)}
            disabled={submitting}
          />
          <Input
            label="Meeting point"
            name="meeting_point"
            type="text"
            required
            placeholder="e.g. Cape Coast Castle main entrance"
            value={meetingPoint}
            onChange={(e) => setMeetingPoint(e.target.value)}
            disabled={submitting}
          />
          <Textarea
            label="Notes (optional)"
            name="notes"
            hint="Anything your guide should know — interests, pace, accessibility."
            value={notes}
            onChange={(e) => setNotes(e.target.value)}
            disabled={submitting}
          />

          {quoteState === "loading" ? (
            <div
              className="gg-skeleton"
              style={{ height: "6rem" }}
              aria-busy="true"
              aria-label="Calculating quote"
            />
          ) : null}

          {quoteState === "error" ? (
            <Alert tone="error" title="Quote failed">
              <p>{quoteError}</p>
            </Alert>
          ) : null}

          {quoteState === "ready" && quote ? (
            <div className="quote-panel" aria-live="polite">
              <p className="quote-panel__title">Server-calculated quote</p>
              <dl className="quote-rows">
                <div className="quote-row">
                  <dt>Tour price</dt>
                  <dd>{formatPrice(quote.amount, quoteCurrency)}</dd>
                </div>
                <div className="quote-row">
                  <dt>Platform fee</dt>
                  <dd>{formatPrice(quote.platform_fee, quoteCurrency)}</dd>
                </div>
                <div className="quote-row">
                  <dt>Tourism levy</dt>
                  <dd>{formatPrice(quote.tourism_levy, quoteCurrency)}</dd>
                </div>
              </dl>
              <p className="muted">
                Final pricing is always confirmed by the server — never by your
                browser.
              </p>
            </div>
          ) : null}

          {submitError ? (
            <Alert tone="error" title="Booking failed">
              <p>{submitError}</p>
            </Alert>
          ) : null}

          <div>
            <Button
              type="submit"
              disabled={submitting || quoteState !== "ready"}
            >
              {submitting ? "Confirming…" : "Confirm booking"}
            </Button>
          </div>
        </form>
      </Card>

      <p>
        <Link href="/search">← Back to search</Link>
      </p>
    </div>
  );
}
