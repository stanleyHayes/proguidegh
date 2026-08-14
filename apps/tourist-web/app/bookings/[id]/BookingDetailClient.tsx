"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { Alert, Badge, Button, Card } from "@proguidegh/ui";
import { api, ApiError, errorMessage, unwrap } from "../../lib/api";
import { formatPrice } from "../../lib/catalog";
import LivePanel from "./LivePanel";
import SosPanel from "./SosPanel";
import ReviewPanel from "./ReviewPanel";
import {
  BOOKING_EXCEPTIONS,
  BOOKING_FLOW,
  bookingPackageName,
  formatDateTime,
  formatStatus,
  guideName,
  isPaidStatus,
  statusTone,
  type Booking,
} from "../../lib/bookings";

type LoadState = "loading" | "unauthenticated" | "not-found" | "error" | "ready";

function StatusTimeline({ status }: { status?: string }) {
  const isException = (BOOKING_EXCEPTIONS as readonly string[]).includes(
    status ?? "",
  );
  const currentIndex = BOOKING_FLOW.indexOf(
    status as (typeof BOOKING_FLOW)[number],
  );

  return (
    <div className="stack" style={{ gap: "var(--gg-space-4)" }}>
      {isException ? (
        <Alert
          tone={statusTone(status) === "danger" ? "error" : "info"}
          title={formatStatus(status)}
        >
          <p>
            This booking left the standard flow. Contact support if you are
            unsure what this means for your tour.
          </p>
        </Alert>
      ) : null}
      <ol className="timeline" aria-label="Booking status timeline">
        {BOOKING_FLOW.map((step, index) => {
          const done = !isException && index < currentIndex;
          const current = !isException && index === currentIndex;
          return (
            <li
              key={step}
              className={`timeline__item${done ? " timeline__item--done" : ""}${
                current ? " timeline__item--current" : ""
              }`}
              aria-current={current ? "step" : undefined}
            >
              <span className="timeline__marker" aria-hidden="true" />
              <span
                className={
                  done || current ? undefined : "timeline__label--pending"
                }
              >
                {formatStatus(step)}
              </span>
              {current ? (
                <Badge tone={statusTone(status)}>Current</Badge>
              ) : null}
            </li>
          );
        })}
      </ol>
    </div>
  );
}

export default function BookingDetailClient({ bookingId }: { bookingId: string }) {
  const [state, setState] = useState<LoadState>("loading");
  const [loadError, setLoadError] = useState<string | null>(null);
  const [booking, setBooking] = useState<Booking | null>(null);

  const load = useCallback(async () => {
    setState("loading");
    setLoadError(null);
    try {
      const data = await api<unknown>(`/bookings/${bookingId}`);
      setBooking(unwrap<Booking>(data, "booking"));
      setState("ready");
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        setState("unauthenticated");
      } else if (err instanceof ApiError && err.status === 404) {
        setState("not-found");
      } else {
        setLoadError(
          errorMessage(err, "Could not load this booking. Please retry."),
        );
        setState("error");
      }
    }
  }, [bookingId]);

  useEffect(() => {
    void load();
  }, [load]);

  if (state === "loading") {
    return (
      <div className="stack" aria-busy="true" aria-label="Loading booking details">
        <div className="gg-skeleton" style={{ height: "2rem", width: "40%" }} />
        <div className="gg-skeleton" style={{ height: "10rem" }} />
        <div className="gg-skeleton" style={{ height: "10rem" }} />
      </div>
    );
  }

  if (state === "unauthenticated") {
    return (
      <div className="stack">
        <h1>Booking details</h1>
        <Alert tone="info" title="Sign in required">
          <p>You need to sign in to view this booking.</p>
        </Alert>
        <p>
          <Link className="gg-button gg-button--primary" href="/login">
            Sign in
          </Link>
        </p>
      </div>
    );
  }

  if (state === "not-found") {
    return (
      <div className="stack">
        <h1>Booking details</h1>
        <Alert tone="info" title="Booking not found">
          <p>We could not find this booking on your account.</p>
        </Alert>
        <p>
          <Link className="gg-button gg-button--secondary" href="/bookings">
            My bookings
          </Link>
        </p>
      </div>
    );
  }

  if (state === "error" || !booking) {
    return (
      <div className="stack">
        <h1>Booking details</h1>
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

  return (
    <div className="stack">
      <section aria-labelledby="booking-heading">
        <h1 id="booking-heading">
          Booking {booking.reference ?? booking.id}
        </h1>
        <p>
          <Badge tone={statusTone(booking.status)}>
            {formatStatus(booking.status)}
          </Badge>
        </p>
      </section>

      <Card title="Status">
        <StatusTimeline status={booking.status} />
      </Card>

      <LivePanel bookingId={booking.id} status={booking.status} />

      <SosPanel bookingId={booking.id} status={booking.status} />

      <ReviewPanel bookingId={booking.id} status={booking.status} />

      <Card title="Tour details">
        <dl className="quote-rows">
          <div className="quote-row">
            <dt>Package</dt>
            <dd>{bookingPackageName(booking)}</dd>
          </div>
          <div className="quote-row">
            <dt>Guide</dt>
            <dd>{guideName(booking.guide)}</dd>
          </div>
          <div className="quote-row">
            <dt>Starts</dt>
            <dd>{formatDateTime(booking.starts_at)}</dd>
          </div>
          <div className="quote-row">
            <dt>Ends</dt>
            <dd>{formatDateTime(booking.ends_at)}</dd>
          </div>
          <div className="quote-row">
            <dt>Guests</dt>
            <dd>{booking.guests ?? "—"}</dd>
          </div>
          <div className="quote-row">
            <dt>Meeting point</dt>
            <dd>{booking.meeting_point ?? "—"}</dd>
          </div>
          {booking.amount !== undefined ? (
            <div className="quote-row">
              <dt>Amount</dt>
              <dd>{formatPrice(booking.amount, booking.currency)}</dd>
            </div>
          ) : null}
        </dl>
        {booking.notes ? (
          <p>
            <strong>Notes for your guide:</strong> {booking.notes}
          </p>
        ) : null}
      </Card>

      {booking.status === "PAYMENT_PENDING" ||
      booking.status === "PAYMENT_FAILED" ? (
        <p>
          <Link
            className="gg-button gg-button--primary"
            href={`/checkout/${booking.id}`}
          >
            Go to checkout
          </Link>
        </p>
      ) : null}

      {isPaidStatus(booking.status) ? (
        <p>
          <Link
            className="gg-button gg-button--secondary"
            href={`/receipts/${booking.id}`}
          >
            View receipt
          </Link>
        </p>
      ) : null}

      <p>
        <Link href="/bookings">← Back to my bookings</Link>
      </p>
    </div>
  );
}
