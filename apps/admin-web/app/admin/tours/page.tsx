"use client";

import { useCallback, useEffect, useState } from "react";
import { Alert, Badge, Button, Card } from "@proguidegh/ui";
import { api, ApiError, errorMessage } from "../../lib/api";
import { Unauthorized } from "../../components/Unauthorized";
import { useWebSocket } from "../../lib/useWebSocket";

/**
 * Operations board (spec §18.4).
 *
 * The active-tours table fed by REST is the primary representation of
 * operational state; the /ws/admin/operations event feed is a live
 * enhancement, and the board keeps polling every 5s when the socket is
 * unavailable.
 */

/** Assumed shape of GET /admin/bookings?status=active entries. */
interface ActiveTour {
  id: string;
  reference?: string;
  status?: string;
  guide?: string;
  tourist?: string;
  updated_at?: string;
}

interface OpsEvent {
  id: string;
  label: string;
  at: string;
}

type LoadState =
  | "loading"
  | "unauthorized"
  | "forbidden"
  | "pending"
  | "error"
  | "ready";

function asRecord(value: unknown): Record<string, unknown> | null {
  return value !== null && typeof value === "object"
    ? (value as Record<string, unknown>)
    : null;
}

function asString(value: unknown): string | undefined {
  return typeof value === "string" && value ? value : undefined;
}

function nameOf(value: unknown): string | undefined {
  const record = asRecord(value);
  if (!record) return asString(value);
  return (
    asString(record.name) ??
    asString(record.public_name) ??
    asString(record.full_name)
  );
}

function parseTours(data: unknown): ActiveTour[] {
  const list = Array.isArray(data)
    ? data
    : (["bookings", "tours", "items", "results"] as const)
        .map((key) => asRecord(data)?.[key])
        .find(Array.isArray) ?? [];
  return list
    .map((entry): ActiveTour | null => {
      const record = asRecord(entry);
      if (!record || typeof record.id !== "string") return null;
      return {
        id: record.id,
        reference: asString(record.reference),
        status: asString(record.status),
        guide: nameOf(record.guide),
        tourist: nameOf(record.tourist) ?? nameOf(record.customer),
        updated_at:
          asString(record.updated_at) ??
          asString(record.updatedAt) ??
          asString(record.last_update) ??
          asString(record.lastUpdate),
      };
    })
    .filter((tour): tour is ActiveTour => tour !== null);
}

function describeEvent(data: unknown, index: number): OpsEvent {
  const record = asRecord(data);
  const type = asString(record?.type) ?? asString(record?.event) ?? "event";
  const booking =
    asString(record?.reference) ??
    asString(asRecord(record?.booking)?.reference) ??
    asString(record?.booking_id) ??
    "";
  const at =
    asString(record?.at) ??
    asString(record?.timestamp) ??
    asString(record?.captured_at) ??
    new Date().toISOString();
  return {
    id: `${at}-${index}-${type}`,
    label: booking ? `${type} — ${booking}` : type,
    at,
  };
}

function formatTime(value?: string): string {
  if (!value) return "—";
  const date = new Date(value);
  return Number.isNaN(date.getTime())
    ? value
    : date.toLocaleString(undefined, { dateStyle: "short", timeStyle: "medium" });
}

function statusTone(status?: string): "success" | "warning" | "neutral" {
  switch (status) {
    case "GUIDE_EN_ROUTE":
    case "GUIDE_ARRIVED":
    case "IN_PROGRESS":
      return "success";
    case "CONFIRMED":
      return "warning";
    default:
      return "neutral";
  }
}

export default function AdminToursPage() {
  const [state, setState] = useState<LoadState>("loading");
  const [loadError, setLoadError] = useState<string | null>(null);
  const [tours, setTours] = useState<ActiveTour[]>([]);
  const [events, setEvents] = useState<OpsEvent[]>([]);

  const load = useCallback(async () => {
    setLoadError(null);
    try {
      const data = await api<unknown>("/admin/bookings?status=active");
      setTours(parseTours(data));
      setState("ready");
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        setState("unauthorized");
      } else if (err instanceof ApiError && err.status === 403) {
        setState("forbidden");
      } else if (err instanceof ApiError && err.status === 404) {
        // Endpoint ships with the backend in this phase; hold the shell.
        setState("pending");
      } else {
        setLoadError(
          errorMessage(err, "Could not load active tours. Please retry."),
        );
        setState("error");
      }
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const handleEvent = useCallback((data: unknown) => {
    setEvents((current) =>
      [describeEvent(data, current.length), ...current].slice(0, 50),
    );
  }, []);

  const liveStatus = useWebSocket({
    path: "/ws/admin/operations",
    enabled: state === "ready" || state === "pending",
    onMessage: handleEvent,
    onPoll: () => void load(),
  });

  if (state === "unauthorized" || state === "forbidden") {
    return (
      <div className="stack">
        <h1>Tour operations</h1>
        <Unauthorized forbidden={state === "forbidden"} />
      </div>
    );
  }

  return (
    <div className="stack">
      <section aria-labelledby="tours-heading">
        <h1 id="tours-heading">Tour operations</h1>
        <p className="muted">
          Active tours across the platform. The table below is the primary
          view; the live event feed is an enhancement.
        </p>
        <Badge tone={liveStatus === "live" ? "success" : "neutral"}>
          {liveStatus === "live" ? "Live feed connected" : "Refreshing every 5s"}
        </Badge>
      </section>

      {state === "loading" ? (
        <div className="stack" aria-busy="true" aria-label="Loading active tours">
          {Array.from({ length: 4 }, (_, i) => (
            <div key={i} className="gg-skeleton" style={{ height: "3rem" }} />
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

      {state === "pending" ? (
        <Alert tone="info" title="Endpoint pending">
          <p>
            The active-tours endpoint is being built in the current backend
            phase. This board will populate as soon as it ships — live events
            still appear in the feed below when the socket is available.
          </p>
        </Alert>
      ) : null}

      {state === "ready" && tours.length === 0 ? (
        <Alert tone="info" title="No active tours">
          <p>There are no tours in progress right now.</p>
        </Alert>
      ) : null}

      {state === "ready" && tours.length > 0 ? (
        <div className="gg-table-scroll">
          <table className="gg-table">
            <thead>
              <tr>
                <th scope="col">Booking</th>
                <th scope="col">Guide</th>
                <th scope="col">Tourist</th>
                <th scope="col">Status</th>
                <th scope="col">Last update</th>
              </tr>
            </thead>
            <tbody>
              {tours.map((tour) => (
                <tr key={tour.id}>
                  <td>{tour.reference ?? tour.id}</td>
                  <td>{tour.guide ?? "—"}</td>
                  <td>{tour.tourist ?? "—"}</td>
                  <td>
                    <Badge tone={statusTone(tour.status)}>
                      {tour.status ?? "unknown"}
                    </Badge>
                  </td>
                  <td>{formatTime(tour.updated_at)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : null}

      <Card title="Live event feed">
        {events.length === 0 ? (
          <p className="muted">
            No events yet. Dispatch, status and location events appear here as
            they arrive.
          </p>
        ) : (
          <ul className="pipeline-list" aria-label="Operations events">
            {events.map((event) => (
              <li key={event.id}>
                <span>{event.label}</span>
                <span className="muted">{formatTime(event.at)}</span>
              </li>
            ))}
          </ul>
        )}
      </Card>
    </div>
  );
}
