"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { Alert, Badge, Button } from "@proguidegh/ui";
import { api, ApiError, errorMessage } from "../../lib/api";
import { useWebSocket, type LiveStatus } from "../../lib/useWebSocket";
import {
  formatDateTime,
  formatPrice,
  isOfferExpired,
  parseOffers,
  secondsRemaining,
  type Offer,
} from "../../lib/dispatch";

type LoadState = "loading" | "unauthenticated" | "error" | "ready";

function liveStatusLabel(status: LiveStatus): string {
  switch (status) {
    case "live":
      return "Live updates";
    case "polling":
      return "Refreshing every 5s";
    default:
      return "Connecting…";
  }
}

function OfferCard({
  offer,
  now,
  onAccept,
  onDecline,
  busy,
}: {
  offer: Offer;
  now: number;
  onAccept: (offer: Offer) => void;
  onDecline: (offer: Offer) => void;
  busy: boolean;
}) {
  const expired = isOfferExpired(offer, now);
  const remaining = secondsRemaining(offer, now);

  return (
    <li className={`offer-card${expired ? " offer-card--expired" : ""}`}>
      <div className="offer-card__body">
        <div className="offer-card__header">
          <strong>{offer.booking.package_name ?? "Tour package"}</strong>
          {expired ? (
            <Badge tone="neutral">Expired</Badge>
          ) : (
            <Badge tone={remaining <= 10 ? "danger" : "warning"}>
              {remaining}s left
            </Badge>
          )}
        </div>
        <dl className="quote-rows">
          <div className="quote-row">
            <dt>Starts</dt>
            <dd>{formatDateTime(offer.booking.starts_at)}</dd>
          </div>
          <div className="quote-row">
            <dt>Meeting point</dt>
            <dd>{offer.booking.meeting_point ?? "—"}</dd>
          </div>
          <div className="quote-row">
            <dt>Guests</dt>
            <dd>{offer.booking.guests ?? "—"}</dd>
          </div>
          <div className="quote-row">
            <dt>Payout</dt>
            <dd>{formatPrice(offer.booking.amount, offer.booking.currency)}</dd>
          </div>
        </dl>
      </div>
      {!expired ? (
        <div className="nav-actions">
          <Button
            type="button"
            disabled={busy}
            onClick={() => onAccept(offer)}
          >
            Accept
          </Button>
          <Button
            type="button"
            variant="secondary"
            disabled={busy}
            onClick={() => onDecline(offer)}
          >
            Decline
          </Button>
        </div>
      ) : null}
    </li>
  );
}

export default function GuideJobsPage() {
  const [state, setState] = useState<LoadState>("loading");
  const [loadError, setLoadError] = useState<string | null>(null);
  const [offers, setOffers] = useState<Offer[]>([]);
  const [notice, setNotice] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [acceptedBookingId, setAcceptedBookingId] = useState<string | null>(
    null,
  );
  const [busyOfferId, setBusyOfferId] = useState<string | null>(null);
  // Client-side clock for the expiry countdown (spec §10.3 TTLs).
  const [now, setNow] = useState(() => Date.now());

  const load = useCallback(async () => {
    setLoadError(null);
    try {
      const data = await api<unknown>("/me/guide/offers");
      setOffers(parseOffers(data));
      setState("ready");
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        setState("unauthenticated");
      } else {
        setLoadError(
          errorMessage(err, "Could not load job offers. Please retry."),
        );
        setState("error");
      }
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  // Tick the countdown once per second.
  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(timer);
  }, []);

  // Live pushes are an upgrade over the REST list; polling is the fallback.
  const liveStatus = useWebSocket({
    path: "/ws/guide",
    enabled: state === "ready",
    onMessage: () => void load(),
    onPoll: () => void load(),
  });

  async function accept(offer: Offer) {
    if (
      !window.confirm(
        `Accept this job — ${offer.booking.package_name ?? "tour"} on ${formatDateTime(
          offer.booking.starts_at,
        )}? The booking is assigned to the first guide who accepts.`,
      )
    ) {
      return;
    }
    setBusyOfferId(offer.id);
    setActionError(null);
    setNotice(null);
    try {
      await api(`/offers/${offer.id}/accept`, { method: "POST" });
      setOffers((current) => current.filter((entry) => entry.id !== offer.id));
      setAcceptedBookingId(offer.booking.id);
      setNotice("Job accepted. The tour has been added to your schedule.");
    } catch (err) {
      if (err instanceof ApiError && err.status === 409) {
        setActionError(
          "This offer is no longer available — another guide accepted it or it expired. Refreshing the list.",
        );
      } else {
        setActionError(
          errorMessage(err, "Could not accept this offer. Please retry."),
        );
      }
      await load();
    } finally {
      setBusyOfferId(null);
    }
  }

  async function decline(offer: Offer) {
    if (!window.confirm("Decline this job offer?")) return;
    setBusyOfferId(offer.id);
    setActionError(null);
    setNotice(null);
    try {
      await api(`/offers/${offer.id}/decline`, { method: "POST" });
      setOffers((current) => current.filter((entry) => entry.id !== offer.id));
    } catch (err) {
      if (err instanceof ApiError && err.status === 409) {
        // Already expired or taken — just drop it locally.
        setOffers((current) =>
          current.filter((entry) => entry.id !== offer.id),
        );
      } else {
        setActionError(
          errorMessage(err, "Could not decline this offer. Please retry."),
        );
      }
    } finally {
      setBusyOfferId(null);
    }
  }

  if (state === "unauthenticated") {
    return (
      <div className="stack">
        <h1>Job offers</h1>
        <Alert tone="info" title="Sign in required">
          <p>Sign in with your guide account to receive job offers.</p>
        </Alert>
        <p>
          <Link className="gg-button gg-button--primary" href="/login">
            Sign in
          </Link>
        </p>
      </div>
    );
  }

  const activeOffers = offers.filter((offer) => !isOfferExpired(offer, now));

  return (
    <div className="stack">
      <section aria-labelledby="jobs-heading">
        <h1 id="jobs-heading">Job offers</h1>
        <p className="muted">
          New dispatch offers appear here with a short countdown. The first
          guide to accept wins the booking.
        </p>
        <Badge tone={liveStatus === "live" ? "success" : "neutral"}>
          {liveStatusLabel(liveStatus)}
        </Badge>
      </section>

      {actionError ? (
        <Alert tone="error" title="Offer action failed">
          <p>{actionError}</p>
        </Alert>
      ) : null}

      {notice ? (
        <Alert tone="success" title="Done">
          <p>{notice}</p>
          {acceptedBookingId ? (
            <p>
              <Link
                className="gg-button gg-button--primary"
                href={`/guide/tours/${acceptedBookingId}`}
              >
                Open the tour
              </Link>
            </p>
          ) : null}
        </Alert>
      ) : null}

      {state === "loading" ? (
        <div className="stack" aria-busy="true" aria-label="Loading job offers">
          {Array.from({ length: 3 }, (_, i) => (
            <div key={i} className="gg-skeleton" style={{ height: "10rem" }} />
          ))}
        </div>
      ) : null}

      {state === "error" ? (
        <>
          <Alert tone="error" title="Something went wrong">
            <p>{loadError}</p>
          </Alert>
          <div>
            <Button type="button" onClick={() => void load()}>
              Retry
            </Button>
          </div>
        </>
      ) : null}

      {state === "ready" && offers.length === 0 ? (
        <Alert tone="info" title="No open offers">
          <p>
            You have no job offers right now. Keep this page open — new offers
            appear automatically.
          </p>
        </Alert>
      ) : null}

      {state === "ready" && offers.length > 0 ? (
        <ul className="offer-list" aria-label="Open job offers">
          {offers.map((offer) => (
            <OfferCard
              key={offer.id}
              offer={offer}
              now={now}
              busy={busyOfferId === offer.id}
              onAccept={(entry) => void accept(entry)}
              onDecline={(entry) => void decline(entry)}
            />
          ))}
        </ul>
      ) : null}

      {state === "ready" && offers.length > 0 && activeOffers.length === 0 ? (
        <p className="muted">
          All current offers have expired. New ones will appear automatically.
        </p>
      ) : null}
    </div>
  );
}
