"use client";

import { useCallback, useEffect, useState } from "react";
import { Alert, Badge, Card } from "@proguidegh/ui";
import { api } from "../../lib/api";
import { useWebSocket, type LiveStatus } from "../../lib/useWebSocket";

/**
 * Live guide panel (spec §11, §18.4).
 *
 * Shows the assigned guide's latest position as plain coordinates and text —
 * the map is never the sole representation of operational state, and a Google
 * Maps embed is deliberately out of scope here. Updates arrive over
 * /ws/booking/{id}; when the socket is unavailable the hook falls back to
 * polling GET /bookings/{id} every 5 seconds.
 */

/** Statuses during which the tourist may see the guide position (spec §11.2). */
const LIVE_STATUSES = ["GUIDE_EN_ROUTE", "GUIDE_ARRIVED", "IN_PROGRESS"];

interface GuidePosition {
  latitude: number;
  longitude: number;
  accuracy_m?: number;
  captured_at?: string;
}

function asRecord(value: unknown): Record<string, unknown> | null {
  return value !== null && typeof value === "object"
    ? (value as Record<string, unknown>)
    : null;
}

/** Tolerant parse of a position from a WS message or booking detail. */
function parsePosition(data: unknown): GuidePosition | null {
  const root = asRecord(data);
  if (!root) return null;
  const candidates: unknown[] = [
    root.position,
    root.location,
    root.guide_position,
    asRecord(root.booking)?.position,
    asRecord(root.booking)?.guide_position,
    root,
  ];
  for (const candidate of candidates) {
    const record = asRecord(candidate);
    if (!record) continue;
    if (
      typeof record.latitude === "number" &&
      typeof record.longitude === "number"
    ) {
      const capturedAt = record.captured_at ?? record.capturedAt;
      return {
        latitude: record.latitude,
        longitude: record.longitude,
        accuracy_m:
          typeof record.accuracy_m === "number" ? record.accuracy_m : undefined,
        captured_at: typeof capturedAt === "string" ? capturedAt : undefined,
      };
    }
  }
  return null;
}

/** Tolerant parse of the status from a WS message or booking detail. */
function parseStatus(data: unknown): string | undefined {
  const root = asRecord(data);
  if (!root) return undefined;
  const booking = asRecord(root.booking);
  if (typeof root.status === "string") return root.status;
  if (typeof booking?.status === "string") return booking.status;
  return undefined;
}

function formatAgo(capturedAt: string | undefined, now: number): string {
  if (!capturedAt) return "just now";
  const captured = new Date(capturedAt).getTime();
  if (Number.isNaN(captured)) return "just now";
  const seconds = Math.max(0, Math.round((now - captured) / 1000));
  if (seconds < 5) return "just now";
  if (seconds < 60) return `${seconds}s ago`;
  return `${Math.floor(seconds / 60)}m ${seconds % 60}s ago`;
}

export default function LivePanel({
  bookingId,
  status: initialStatus,
}: {
  bookingId: string;
  status?: string;
}) {
  const [status, setStatus] = useState(initialStatus);
  const [position, setPosition] = useState<GuidePosition | null>(null);
  const [now, setNow] = useState(() => Date.now());

  const live = LIVE_STATUSES.includes(status ?? "");

  const refresh = useCallback(async () => {
    try {
      const data = await api<unknown>(`/bookings/${bookingId}`);
      const nextStatus = parseStatus(data);
      if (nextStatus) setStatus(nextStatus);
      const nextPosition = parsePosition(data);
      if (nextPosition) setPosition(nextPosition);
    } catch {
      // A missed poll is fine — the next tick retries.
    }
  }, [bookingId]);

  const handleMessage = useCallback((data: unknown) => {
    const nextStatus = parseStatus(data);
    if (nextStatus) setStatus(nextStatus);
    const nextPosition = parsePosition(data);
    if (nextPosition) setPosition(nextPosition);
  }, []);

  const liveStatus: LiveStatus = useWebSocket({
    path: `/ws/booking/${bookingId}`,
    enabled: live,
    onMessage: handleMessage,
    onPoll: refresh,
  });

  // Tick the "updated Xs ago" label.
  useEffect(() => {
    if (!live) return undefined;
    const timer = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(timer);
  }, [live]);

  if (!live) return null;

  return (
    <Card title="Live — your guide">
      <p>
        <Badge tone={liveStatus === "live" ? "success" : "neutral"}>
          {liveStatus === "live" ? "Live updates" : "Refreshing every 5s"}
        </Badge>
      </p>
      {position ? (
        <dl className="quote-rows">
          <div className="quote-row">
            <dt>Latitude</dt>
            <dd>{position.latitude.toFixed(5)}</dd>
          </div>
          <div className="quote-row">
            <dt>Longitude</dt>
            <dd>{position.longitude.toFixed(5)}</dd>
          </div>
          {position.accuracy_m !== undefined ? (
            <div className="quote-row">
              <dt>Accuracy</dt>
              <dd>±{Math.round(position.accuracy_m)} m</dd>
            </div>
          ) : null}
          <div className="quote-row">
            <dt>Updated</dt>
            <dd>{formatAgo(position.captured_at, now)}</dd>
          </div>
        </dl>
      ) : (
        <Alert tone="info" title="Waiting for the first position">
          <p>
            Your guide is on the way. Their position appears here as soon as
            the first update arrives.
          </p>
        </Alert>
      )}
      <p className="muted">
        Location is only shared while your tour is active, and only with you
        and the operations team.
      </p>
    </Card>
  );
}
