"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { Alert, Badge, Button, Card } from "@proguidegh/ui";
import { api, ApiError, errorMessage } from "../../lib/api";
import {
  formatDateTime,
  formatPrice,
  formatStatus,
  isLiveTourStatus,
  isPastTour,
  parseAssignedBookings,
  type AssignedBooking,
} from "../../lib/dispatch";

type LoadState = "loading" | "unauthenticated" | "error" | "ready";

function tourTone(booking: AssignedBooking): "success" | "warning" | "neutral" {
  if (isLiveTourStatus(booking.status)) return "success";
  if (booking.status === "CONFIRMED") return "warning";
  return "neutral";
}

function TourRow({ booking }: { booking: AssignedBooking }) {
  return (
    <li className="pipeline-list__row">
      <div>
        <strong>
          {booking.package_name ?? "Tour"} —{" "}
          {booking.reference ?? booking.id}
        </strong>
        <p className="muted" style={{ margin: 0 }}>
          {formatDateTime(booking.starts_at)}
          {booking.meeting_point ? ` · ${booking.meeting_point}` : ""}
          {booking.amount !== undefined
            ? ` · ${formatPrice(booking.amount, booking.currency)}`
            : ""}
        </p>
      </div>
      <div className="nav-actions" style={{ alignItems: "center" }}>
        <Badge tone={tourTone(booking)}>{formatStatus(booking.status)}</Badge>
        <Link
          className="gg-button gg-button--secondary"
          href={`/guide/tours/${booking.id}`}
        >
          Open
        </Link>
      </div>
    </li>
  );
}

export default function GuideToursPage() {
  const [state, setState] = useState<LoadState>("loading");
  const [loadError, setLoadError] = useState<string | null>(null);
  const [bookings, setBookings] = useState<AssignedBooking[]>([]);

  async function load() {
    setState("loading");
    setLoadError(null);
    try {
      // Assumed endpoint for the guide's assigned bookings (spec §13).
      const data = await api<unknown>("/me/guide/bookings");
      setBookings(parseAssignedBookings(data));
      setState("ready");
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        setState("unauthenticated");
      } else {
        setLoadError(
          errorMessage(err, "Could not load your tours. Please retry."),
        );
        setState("error");
      }
    }
  }

  useEffect(() => {
    void load();
  }, []);

  if (state === "unauthenticated") {
    return (
      <div className="stack">
        <h1>My tours</h1>
        <Alert tone="info" title="Sign in required">
          <p>Sign in with your guide account to see your assigned tours.</p>
        </Alert>
        <p>
          <Link className="gg-button gg-button--primary" href="/login">
            Sign in
          </Link>
        </p>
      </div>
    );
  }

  if (state === "error") {
    return (
      <div className="stack">
        <h1>My tours</h1>
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

  const upcoming = bookings.filter((booking) => !isPastTour(booking));
  const past = bookings.filter(isPastTour);

  return (
    <div className="stack">
      <section aria-labelledby="tours-heading">
        <h1 id="tours-heading">My tours</h1>
        <p className="muted">
          Your assigned bookings — upcoming tours first, history below.
        </p>
      </section>

      {state === "loading" ? (
        <div className="stack" aria-busy="true" aria-label="Loading your tours">
          {Array.from({ length: 3 }, (_, i) => (
            <div key={i} className="gg-skeleton" style={{ height: "4rem" }} />
          ))}
        </div>
      ) : null}

      {state === "ready" && bookings.length === 0 ? (
        <Alert tone="info" title="No tours yet">
          <p>
            Accept a job offer and it will appear here.{" "}
            <Link href="/guide/jobs">Check open offers</Link>.
          </p>
        </Alert>
      ) : null}

      {upcoming.length > 0 ? (
        <Card title="Upcoming">
          <ul className="pipeline-list" aria-label="Upcoming tours">
            {upcoming.map((booking) => (
              <TourRow key={booking.id} booking={booking} />
            ))}
          </ul>
        </Card>
      ) : null}

      {past.length > 0 ? (
        <Card title="Past">
          <ul className="pipeline-list" aria-label="Past tours">
            {past.map((booking) => (
              <TourRow key={booking.id} booking={booking} />
            ))}
          </ul>
        </Card>
      ) : null}
    </div>
  );
}
