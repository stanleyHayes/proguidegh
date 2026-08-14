"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { Alert, Badge, Button, Card } from "@proguidegh/ui";
import { api, ApiError, errorMessage } from "../lib/api";
import { formatPrice } from "../lib/catalog";
import {
  bookingPackageName,
  formatDateTime,
  formatStatus,
  guideName,
  isPaidStatus,
  parseBookings,
  statusTone,
  type Booking,
} from "../lib/bookings";

type LoadState = "loading" | "unauthenticated" | "error" | "ready";

export default function BookingsPage() {
  const [state, setState] = useState<LoadState>("loading");
  const [loadError, setLoadError] = useState<string | null>(null);
  const [bookings, setBookings] = useState<Booking[]>([]);

  const load = useCallback(async () => {
    setState("loading");
    setLoadError(null);
    try {
      const data = await api<unknown>("/me/bookings");
      setBookings(parseBookings(data));
      setState("ready");
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        setState("unauthenticated");
      } else {
        setLoadError(
          errorMessage(err, "Could not load your bookings. Please retry."),
        );
        setState("error");
      }
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  if (state === "loading") {
    return (
      <div className="stack" aria-busy="true" aria-label="Loading your bookings">
        <div className="gg-skeleton" style={{ height: "2rem", width: "40%" }} />
        {Array.from({ length: 3 }, (_, i) => (
          <div key={i} className="gg-skeleton" style={{ height: "8rem" }} />
        ))}
      </div>
    );
  }

  if (state === "unauthenticated") {
    return (
      <div className="stack">
        <h1>My bookings</h1>
        <Alert tone="info" title="Sign in required">
          <p>You need to sign in to view your booking history.</p>
        </Alert>
        <p>
          <Link className="gg-button gg-button--primary" href="/login">
            Sign in
          </Link>{" "}
          <Link className="gg-button gg-button--secondary" href="/register">
            Create an account
          </Link>
        </p>
      </div>
    );
  }

  if (state === "error") {
    return (
      <div className="stack">
        <h1>My bookings</h1>
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
      <section aria-labelledby="bookings-heading">
        <h1 id="bookings-heading">My bookings</h1>
        <p className="muted">
          Your booking history — receipts, status and meeting details.
        </p>
      </section>

      {bookings.length === 0 ? (
        <>
          <Alert tone="info" title="No bookings yet">
            <p>
              You have not booked a tour yet. Find a certified guide and make
              your first booking.
            </p>
          </Alert>
          <p>
            <Link className="gg-button gg-button--primary" href="/search">
              Find a guide
            </Link>
          </p>
        </>
      ) : (
        <div className="grid grid--cols-2" aria-label="Your bookings">
          {bookings.map((booking) => (
            <Card
              key={booking.id}
              title={booking.reference ?? bookingPackageName(booking)}
            >
              <div className="stack" style={{ gap: "var(--gg-space-3)" }}>
                <div className="badge-row">
                  <Badge tone={statusTone(booking.status)}>
                    {formatStatus(booking.status)}
                  </Badge>
                </div>
                <p>
                  <strong>{bookingPackageName(booking)}</strong>
                  <br />
                  <span className="muted">with {guideName(booking.guide)}</span>
                </p>
                <p className="muted">{formatDateTime(booking.starts_at)}</p>
                {booking.amount !== undefined ? (
                  <p>
                    <strong>
                      {formatPrice(booking.amount, booking.currency)}
                    </strong>
                  </p>
                ) : null}
                <p className="nav-actions">
                  <Link
                    className="gg-button gg-button--secondary"
                    href={`/bookings/${booking.id}`}
                  >
                    View booking
                  </Link>
                  {isPaidStatus(booking.status) ? (
                    <Link
                      className="gg-button gg-button--secondary"
                      href={`/receipts/${booking.id}`}
                    >
                      Receipt
                    </Link>
                  ) : null}
                </p>
              </div>
            </Card>
          ))}
        </div>
      )}
    </div>
  );
}
