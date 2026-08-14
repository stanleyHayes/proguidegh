"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import Link from "next/link";
import { Alert, Badge, Button, Card } from "@proguidegh/ui";
import { api, ApiError, errorMessage } from "../../../lib/api";
import {
  formatDateTime,
  formatPrice,
  formatStatus,
  isLiveTourStatus,
  nextTourAction,
  parseAssignedBookingDetail,
  TOUR_FLOW,
  type AssignedBooking,
} from "../../../lib/dispatch";

type LoadState = "loading" | "unauthenticated" | "not-found" | "error" | "ready";
type ShareState = "inactive" | "starting" | "sharing" | "denied" | "unsupported";

const LOCATION_POST_INTERVAL_MS = 10_000;

function StatusStepper({ status }: { status?: string }) {
  const currentIndex = TOUR_FLOW.indexOf(
    status as (typeof TOUR_FLOW)[number],
  );
  return (
    <ol className="timeline" aria-label="Tour progress">
      {TOUR_FLOW.map((step, index) => {
        const done = index < currentIndex;
        const current = index === currentIndex;
        return (
          <li
            key={step}
            className={`timeline__item${done ? " timeline__item--done" : ""}${
              current ? " timeline__item--current" : ""
            }`}
            aria-current={current ? "step" : undefined}
          >
            <span className="timeline__marker" aria-hidden="true" />
            <span className={done || current ? undefined : "timeline__label--pending"}>
              {formatStatus(step)}
            </span>
            {current ? <Badge tone="warning">Current</Badge> : null}
          </li>
        );
      })}
    </ol>
  );
}

/**
 * Live location sharing (spec §11): while the tour is in a live status the
 * browser watches the device position and posts a compact update roughly
 * every 10 seconds. Coordinates stay server-side for the active booking
 * window only — see the privacy copy rendered next to the indicator.
 *
 * Note: background GPS while the screen is off needs the native wrapper
 * planned in spec §34; this web build only shares while the page is open.
 */
function useLocationSharing(bookingId: string, status?: string): ShareState {
  const [shareState, setShareState] = useState<ShareState>("inactive");
  const lastPostRef = useRef(0);

  useEffect(() => {
    if (!isLiveTourStatus(status)) {
      setShareState("inactive");
      return undefined;
    }
    if (!("geolocation" in navigator)) {
      setShareState("unsupported");
      return undefined;
    }

    setShareState("starting");
    let cancelled = false;

    const post = (position: GeolocationPosition) => {
      const now = Date.now();
      if (now - lastPostRef.current < LOCATION_POST_INTERVAL_MS) return;
      lastPostRef.current = now;
      const { coords } = position;
      void api(`/bookings/${bookingId}/location`, {
        method: "POST",
        body: {
          latitude: coords.latitude,
          longitude: coords.longitude,
          accuracy_m: coords.accuracy ?? undefined,
          heading: coords.heading ?? undefined,
          speed_mps: coords.speed ?? undefined,
          captured_at: new Date(position.timestamp).toISOString(),
        },
      }).catch(() => {
        // A missed ping is fine — the next watch callback retries.
      });
    };

    const watchId = navigator.geolocation.watchPosition(
      (position) => {
        if (cancelled) return;
        setShareState("sharing");
        post(position);
      },
      (error) => {
        if (cancelled) return;
        setShareState(
          error.code === error.PERMISSION_DENIED ? "denied" : "unsupported",
        );
      },
      { enableHighAccuracy: true, maximumAge: 5_000 },
    );

    return () => {
      cancelled = true;
      navigator.geolocation.clearWatch(watchId);
    };
  }, [bookingId, status]);

  return shareState;
}

export default function TourClient({ bookingId }: { bookingId: string }) {
  const [state, setState] = useState<LoadState>("loading");
  const [loadError, setLoadError] = useState<string | null>(null);
  const [booking, setBooking] = useState<AssignedBooking | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    setLoadError(null);
    try {
      const data = await api<unknown>(`/bookings/${bookingId}`);
      const parsed = parseAssignedBookingDetail(data);
      if (!parsed) throw new Error("unexpected-shape");
      setBooking(parsed);
      setState("ready");
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        setState("unauthenticated");
      } else if (err instanceof ApiError && err.status === 404) {
        setState("not-found");
      } else {
        setLoadError(
          errorMessage(err, "Could not load this tour. Please retry."),
        );
        setState("error");
      }
    }
  }, [bookingId]);

  useEffect(() => {
    void load();
  }, [load]);

  const shareState = useLocationSharing(bookingId, booking?.status);

  async function runAction(endpoint: string, confirmation: string) {
    if (!window.confirm(confirmation)) return;
    setBusy(true);
    setActionError(null);
    try {
      await api(`/bookings/${bookingId}${endpoint}`, { method: "POST" });
      await load();
    } catch (err) {
      if (err instanceof ApiError && err.status === 409) {
        setActionError(
          "This action is no longer valid for the tour's current status. The page has been refreshed.",
        );
        await load();
      } else {
        setActionError(errorMessage(err, "Action failed. Please retry."));
      }
    } finally {
      setBusy(false);
    }
  }

  if (state === "loading") {
    return (
      <div className="stack" aria-busy="true" aria-label="Loading tour">
        <div className="gg-skeleton" style={{ height: "2rem", width: "40%" }} />
        <div className="gg-skeleton" style={{ height: "10rem" }} />
        <div className="gg-skeleton" style={{ height: "10rem" }} />
      </div>
    );
  }

  if (state === "unauthenticated") {
    return (
      <div className="stack">
        <h1>Tour</h1>
        <Alert tone="info" title="Sign in required">
          <p>Sign in with your guide account to manage this tour.</p>
        </Alert>
        <p>
          <Link className="gg-button gg-button--primary" href="/login">
            Sign in
          </Link>
        </p>
      </div>
    );
  }

  if (state === "not-found" || state === "error" || !booking) {
    return (
      <div className="stack">
        <h1>Tour</h1>
        <Alert
          tone={state === "not-found" ? "info" : "error"}
          title={state === "not-found" ? "Tour not found" : "Something went wrong"}
        >
          <p>
            {state === "not-found"
              ? "We could not find this tour on your account."
              : loadError}
          </p>
        </Alert>
        <p className="nav-actions">
          <Button type="button" onClick={() => void load()}>
            Retry
          </Button>
          <Link className="gg-button gg-button--secondary" href="/guide/tours">
            My tours
          </Link>
        </p>
      </div>
    );
  }

  const action = nextTourAction(booking.status);

  return (
    <div className="stack">
      <section aria-labelledby="tour-heading">
        <h1 id="tour-heading">
          Tour {booking.reference ?? booking.id}
        </h1>
        <p>
          <Badge tone={isLiveTourStatus(booking.status) ? "success" : "neutral"}>
            {formatStatus(booking.status)}
          </Badge>
        </p>
      </section>

      {actionError ? (
        <Alert tone="error" title="Action failed">
          <p>{actionError}</p>
        </Alert>
      ) : null}

      <Card title="Progress">
        <StatusStepper status={booking.status} />
        {action ? (
          <p style={{ marginBottom: 0 }}>
            <Button
              type="button"
              disabled={busy}
              onClick={() => void runAction(action.endpoint, action.confirm)}
            >
              {busy ? "Working…" : action.label}
            </Button>
          </p>
        ) : (
          <p className="muted" style={{ marginBottom: 0 }}>
            {booking.status === "COMPLETED"
              ? "This tour is complete. Nice work."
              : "No action available for the current status."}
          </p>
        )}
      </Card>

      {isLiveTourStatus(booking.status) ? (
        <Card title="Live location">
          {shareState === "sharing" || shareState === "starting" ? (
            <p className="live-indicator">
              <span className="live-indicator__dot" aria-hidden="true" />
              Sharing your live location with the tourist and operations.
            </p>
          ) : null}
          {shareState === "denied" ? (
            <Alert tone="error" title="Location permission needed">
              <p>
                Live location is required while a tour is active so the tourist
                can follow your arrival. Enable location access for this site
                in your browser settings, then reload the page.
              </p>
            </Alert>
          ) : null}
          {shareState === "unsupported" ? (
            <Alert tone="info" title="Location unavailable">
              <p>
                This device could not provide a position. Move to an area with
                better signal and keep this page open.
              </p>
            </Alert>
          ) : null}
          <p className="muted">
            Location is only collected while this tour is active and is shared
            only with your assigned tourist and the operations team. Keep this
            page open — sharing pauses if you close it.
          </p>
        </Card>
      ) : null}

      <Card title="Tour details">
        <dl className="quote-rows">
          <div className="quote-row">
            <dt>Package</dt>
            <dd>{booking.package_name ?? "Tour package"}</dd>
          </div>
          <div className="quote-row">
            <dt>Tourist</dt>
            <dd>{booking.tourist_name ?? "—"}</dd>
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
          <div className="quote-row">
            <dt>Payout</dt>
            <dd>{formatPrice(booking.amount, booking.currency)}</dd>
          </div>
        </dl>
        {booking.notes ? (
          <p>
            <strong>Tourist notes:</strong> {booking.notes}
          </p>
        ) : null}
      </Card>

      <p>
        <Link href="/guide/tours">← Back to my tours</Link>
      </p>
    </div>
  );
}
