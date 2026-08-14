"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { Alert, Badge, Button, Card } from "@proguidegh/ui";
import { api, ApiError, errorMessage, unwrap } from "../../lib/api";
import { formatPrice } from "../../lib/catalog";
import {
  bookingPackageName,
  formatDateTime,
  formatStatus,
  guideName,
  isPaidStatus,
  parsePaymentIntent,
  statusTone,
  type Booking,
  type PaymentIntent,
} from "../../lib/bookings";

type LoadState = "loading" | "unauthenticated" | "not-found" | "error" | "ready";

type PayState =
  | "idle"
  | "starting"
  | "redirecting"
  | "polling"
  | "poll-timeout";

const POLL_INTERVAL_MS = 2000;
const POLL_MAX_ATTEMPTS = 15; // ~30 seconds of polling

/**
 * Payment section for PAYMENT_PENDING / PAYMENT_FAILED bookings.
 *
 * Flow: POST payment-intent (one Idempotency-Key per booking, reused across
 * retries) → hand off to the provider-hosted authorization_url. The provider
 * (mock or Paystack) may redirect back with ?reference=…, but its shape is
 * still being built in parallel — so confirmation never trusts the URL: we
 * poll GET /bookings/{id} until the webhook moves the status, whichever way
 * the user returns here.
 */
function PaymentSection({
  booking,
  onBookingChange,
}: {
  booking: Booking;
  onBookingChange: (booking: Booking) => void;
}) {
  const searchParams = useSearchParams();
  const [payState, setPayState] = useState<PayState>("idle");
  const [payError, setPayError] = useState<string | null>(null);
  const [intent, setIntent] = useState<PaymentIntent | null>(null);
  const idempotencyKeyRef = useRef<string | null>(null);

  const startPayment = useCallback(async () => {
    if (!idempotencyKeyRef.current) {
      idempotencyKeyRef.current = crypto.randomUUID();
    }
    setPayState("starting");
    setPayError(null);
    try {
      const data = await api<unknown>(
        `/bookings/${booking.id}/payment-intent`,
        {
          method: "POST",
          headers: { "Idempotency-Key": idempotencyKeyRef.current },
        },
      );
      const parsed = parsePaymentIntent(data);
      if (!parsed.authorization_url) {
        setPayError(
          "The payment provider did not return a checkout link. Please try again.",
        );
        setPayState("idle");
        return;
      }
      setIntent(parsed);
      setPayState("redirecting");
    } catch (err) {
      setPayError(
        errorMessage(err, "Could not start the payment. Please try again."),
      );
      setPayState("idle");
    }
  }, [booking.id]);

  // Hand off to the provider-hosted page once the intent exists.
  useEffect(() => {
    if (payState === "redirecting" && intent?.authorization_url) {
      window.location.assign(intent.authorization_url);
    }
  }, [payState, intent]);

  const checkStatus = useCallback(async () => {
    try {
      const data = await api<unknown>(`/bookings/${booking.id}`);
      const latest = unwrap<Booking>(data, "booking");
      if (latest.status && latest.status !== "PAYMENT_PENDING") {
        onBookingChange(latest);
        setPayState("idle");
        return true;
      }
    } catch {
      // Transient errors are fine — polling retries, manual check can too.
    }
    return false;
  }, [booking.id, onBookingChange]);

  // After returning from the provider (mock redirects back with ?reference=…;
  // Paystack may do the same), poll until the webhook confirms or fails the
  // payment. If the user just navigates back without the param, the Pay now
  // button simply re-shows — the intent is idempotent, so that is safe too.
  const returnedReference = searchParams.get("reference");
  useEffect(() => {
    if (!returnedReference) return;
    setPayState("polling");
    let attempts = 0;
    const timer = window.setInterval(() => {
      attempts += 1;
      void checkStatus().then((moved) => {
        if (attempts >= POLL_MAX_ATTEMPTS && !moved) {
          window.clearInterval(timer);
          setPayState("poll-timeout");
        }
      });
    }, POLL_INTERVAL_MS);
    return () => window.clearInterval(timer);
  }, [returnedReference, checkStatus]);

  const amountLabel =
    booking.amount !== undefined
      ? ` ${formatPrice(booking.amount, booking.currency)}`
      : "";
  const payLabel =
    payState === "starting"
      ? "Starting payment…"
      : booking.status === "PAYMENT_FAILED"
        ? `Retry payment${amountLabel}`
        : `Pay${amountLabel} now`;

  return (
    <Card title="Payment">
      <div className="stack" style={{ gap: "var(--gg-space-4)" }}>
        {booking.status === "PAYMENT_FAILED" ? (
          <Alert tone="error" title="Payment failed">
            <p>
              The payment for this booking did not go through. No charge was
              made — you can try again below.
            </p>
          </Alert>
        ) : null}

        {payError ? (
          <Alert tone="error" title="Payment error">
            <p>{payError}</p>
          </Alert>
        ) : null}

        {payState === "redirecting" && intent ? (
          <Alert tone="info" title="Redirecting to secure checkout">
            <p>
              {intent.provider === "mock" ? (
                <>
                  <Badge tone="warning">Test payment — no real charge</Badge>{" "}
                </>
              ) : null}
              Taking you to the payment page…
            </p>
            <p>
              <a
                className="gg-button gg-button--primary"
                href={intent.authorization_url}
              >
                Continue to payment
              </a>
            </p>
          </Alert>
        ) : null}

        {payState === "polling" ? (
          <Alert tone="info" title="Confirming your payment">
            <p>
              We are waiting for the payment provider to confirm. This usually
              takes a few seconds — we keep checking for up to 30 seconds, so
              please stay on this page.
            </p>
          </Alert>
        ) : null}

        {payState === "poll-timeout" ? (
          <Alert tone="info" title="Still waiting for confirmation">
            <p>
              The provider has not confirmed the payment yet. It may still be
              processing — check the status again, or start the payment flow
              over (you will not be charged twice).
            </p>
          </Alert>
        ) : null}

        <div className="nav-actions">
          <Button
            type="button"
            variant="primary"
            disabled={payState === "starting"}
            onClick={() => void startPayment()}
          >
            {payLabel}
          </Button>
          {payState === "poll-timeout" ? (
            <Button type="button" onClick={() => void checkStatus()}>
              Check status
            </Button>
          ) : null}
        </div>
      </div>
    </Card>
  );
}

export default function CheckoutClient({ bookingId }: { bookingId: string }) {
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
      <div className="stack" aria-busy="true" aria-label="Loading your booking">
        <div className="gg-skeleton" style={{ height: "2rem", width: "40%" }} />
        <div className="gg-skeleton" style={{ height: "12rem" }} />
      </div>
    );
  }

  if (state === "unauthenticated") {
    return (
      <div className="stack">
        <h1>Checkout</h1>
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
        <h1>Checkout</h1>
        <Alert tone="info" title="Booking not found">
          <p>We could not find this booking on your account.</p>
        </Alert>
        <p>
          <Link className="gg-button gg-button--secondary" href="/bookings">
            View my bookings
          </Link>
        </p>
      </div>
    );
  }

  if (state === "error" || !booking) {
    return (
      <div className="stack">
        <h1>Checkout</h1>
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

  const awaitingPayment =
    booking.status === "PAYMENT_PENDING" || booking.status === "PAYMENT_FAILED";

  return (
    <div className="stack">
      <section aria-labelledby="checkout-heading">
        <h1 id="checkout-heading">
          Booking {booking.reference ?? booking.id}
        </h1>
        <p>
          <Badge tone={statusTone(booking.status)}>
            {formatStatus(booking.status)}
          </Badge>
        </p>
      </section>

      {booking.status === "CONFIRMED" ? (
        <Alert tone="success" title="Payment confirmed">
          <p>
            Your booking{booking.reference ? ` ${booking.reference}` : ""} is
            confirmed. A receipt is now available, and your guide will be in
            touch before the tour.
          </p>
        </Alert>
      ) : null}

      <Card title="Booking summary">
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
      </Card>

      {awaitingPayment ? (
        <PaymentSection booking={booking} onBookingChange={setBooking} />
      ) : null}

      <nav aria-label="Booking actions" className="nav-actions">
        {isPaidStatus(booking.status) ? (
          <Link
            className="gg-button gg-button--primary"
            href={`/receipts/${booking.id}`}
          >
            View receipt
          </Link>
        ) : null}
        <Link
          className="gg-button gg-button--secondary"
          href={`/bookings/${booking.id}`}
        >
          Booking details
        </Link>
        <Link className="gg-button gg-button--secondary" href="/bookings">
          My bookings
        </Link>
        <Link className="gg-button gg-button--secondary" href="/search">
          Back to search
        </Link>
      </nav>
    </div>
  );
}
