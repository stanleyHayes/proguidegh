"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { Alert, Badge, Button, Card } from "@proguidegh/ui";
import { api, ApiError, errorMessage } from "../../lib/api";
import { Unauthorized } from "../../components/Unauthorized";
import { useWebSocket } from "../../lib/useWebSocket";

/**
 * Safety desk (spec §12, P6-03) — the incident dashboard.
 *
 * Every workflow action (acknowledge, note, escalate, assign, resolve,
 * close) hits the API and re-reads the incident: the append-only trail on
 * the server is the source of truth, never local state. The
 * /ws/admin/operations feed (sos.triggered / incident.updated) triggers a
 * refetch; polling covers socket outages.
 */

interface Incident {
  id: string;
  booking_id?: string | null;
  type: string;
  severity: string;
  status: string;
  reported_by?: string | null;
  assigned_to?: string | null;
  occurred_at: string;
}

interface IncidentEvent {
  id: string;
  kind: string;
  body?: string | null;
  actor_id?: string | null;
  created_at: string;
}

type LoadState = "loading" | "unauthorized" | "forbidden" | "error" | "ready";

const STATUS_FILTERS = ["", "open", "acknowledged", "in_progress", "resolved", "closed"];
const SEVERITY_TONE: Record<string, "neutral" | "warning" | "danger"> = {
  low: "neutral",
  medium: "warning",
  high: "danger",
  critical: "danger",
};

function asRecord(value: unknown): Record<string, unknown> | null {
  return value !== null && typeof value === "object"
    ? (value as Record<string, unknown>)
    : null;
}

function parseIncidents(data: unknown): Incident[] {
  const list = asRecord(data)?.incidents;
  if (!Array.isArray(list)) return [];
  return list
    .map((entry) => asRecord(entry))
    .filter((r): r is Record<string, unknown> => r !== null && typeof r.id === "string")
    .map((r) => ({
      id: r.id as string,
      booking_id: (r.booking_id as string | null) ?? null,
      type: String(r.type ?? ""),
      severity: String(r.severity ?? "low"),
      status: String(r.status ?? "open"),
      reported_by: (r.reported_by as string | null) ?? null,
      assigned_to: (r.assigned_to as string | null) ?? null,
      occurred_at: String(r.occurred_at ?? ""),
    }));
}

function parseEvents(data: unknown): IncidentEvent[] {
  const list = asRecord(data)?.events;
  if (!Array.isArray(list)) return [];
  return list
    .map((entry) => asRecord(entry))
    .filter((r): r is Record<string, unknown> => r !== null && typeof r.id === "string")
    .map((r) => ({
      id: r.id as string,
      kind: String(r.kind ?? ""),
      body: (r.body as string | null) ?? null,
      actor_id: (r.actor_id as string | null) ?? null,
      created_at: String(r.created_at ?? ""),
    }));
}

function fmt(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString(undefined, {
    day: "numeric",
    month: "short",
    hour: "2-digit",
    minute: "2-digit",
  });
}

export default function IncidentsPage() {
  const [state, setState] = useState<LoadState>("loading");
  const [statusFilter, setStatusFilter] = useState("open");
  const [incidents, setIncidents] = useState<Incident[]>([]);
  const [selected, setSelected] = useState<Incident | null>(null);
  const [events, setEvents] = useState<IncidentEvent[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [note, setNote] = useState("");
  const [assignee, setAssignee] = useState("");

  const loadList = useCallback(async () => {
    try {
      const query = statusFilter ? `?status=${encodeURIComponent(statusFilter)}` : "";
      const data = await api(`/admin/incidents${query}`);
      setIncidents(parseIncidents(data));
      setState("ready");
      setError(null);
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        setState("unauthorized");
      } else if (err instanceof ApiError && err.status === 403) {
        setState("forbidden");
      } else {
        setError(errorMessage(err, "Could not load incidents."));
        setState("error");
      }
    }
  }, [statusFilter]);

  const loadDetail = useCallback(async (id: string) => {
    try {
      const data = await api(`/admin/incidents/${id}`);
      const record = asRecord(data);
      const inc = asRecord(record?.incident);
      if (inc) {
        setSelected({
          id: String(inc.id),
          booking_id: (inc.booking_id as string | null) ?? null,
          type: String(inc.type ?? ""),
          severity: String(inc.severity ?? "low"),
          status: String(inc.status ?? "open"),
          reported_by: (inc.reported_by as string | null) ?? null,
          assigned_to: (inc.assigned_to as string | null) ?? null,
          occurred_at: String(inc.occurred_at ?? ""),
        });
      }
      setEvents(parseEvents(data));
    } catch (err) {
      setActionError(errorMessage(err, "Could not load the incident."));
    }
  }, []);

  useEffect(() => {
    void loadList();
  }, [loadList]);

  // Live alert: an SOS anywhere in the system refetches the desk instantly;
  // the socket is an enhancement over plain polling.
  const liveStatus = useWebSocket({
    path: "/ws/admin/operations",
    onMessage: useCallback(
      (data: unknown) => {
        const type = asRecord(data)?.type;
        if (type === "sos.triggered" || type === "incident.updated") {
          void loadList();
        }
      },
      [loadList],
    ),
    onPoll: useCallback(() => void loadList(), [loadList]),
  });

  const act = useCallback(
    async (path: string, body?: unknown) => {
      if (!selected) return;
      setBusy(true);
      setActionError(null);
      try {
        await api(`/admin/incidents/${selected.id}${path}`, {
          method: "POST",
          body: body ?? {},
        });
        await loadDetail(selected.id);
        await loadList();
      } catch (err) {
        setActionError(errorMessage(err, "The action could not be completed."));
      } finally {
        setBusy(false);
      }
    },
    [selected, loadDetail, loadList],
  );

  const terminal = useMemo(
    () => selected?.status === "closed" || selected?.status === "resolved",
    [selected],
  );

  if (state === "unauthorized" || state === "forbidden") {
    return <Unauthorized />;
  }

  return (
    <div className="stack">
      <section aria-labelledby="incidents-heading">
        <h1 id="incidents-heading">Safety desk</h1>
        <p className="muted">
          Incidents and SOS alerts. Every action is timestamped and audited.
          Feed: {liveStatus === "live" ? "live" : "polling"}.
        </p>
        <div className="nav-actions" role="group" aria-label="Filter by status">
          {STATUS_FILTERS.map((s) => (
            <Button
              key={s || "all"}
              variant={statusFilter === s ? "primary" : "secondary"}
              onClick={() => setStatusFilter(s)}
            >
              {s === "" ? "All" : s.replace("_", " ")}
            </Button>
          ))}
        </div>
      </section>

      {state === "error" && error ? <Alert tone="error">{error}</Alert> : null}

      <div className="grid grid--cols-2">
        <Card title={`Incidents (${incidents.length})`}>
          {incidents.length === 0 ? (
            <p className="muted">No {statusFilter || ""} incidents. Empty state.</p>
          ) : (
            <ul className="stack" aria-label="Incident list">
              {incidents.map((inc) => (
                <li key={inc.id}>
                  <button
                    type="button"
                    className="gg-button gg-button--secondary"
                    aria-pressed={selected?.id === inc.id}
                    onClick={() => {
                      setSelected(inc);
                      setActionError(null);
                      void loadDetail(inc.id);
                    }}
                  >
                    <Badge tone={SEVERITY_TONE[inc.severity] ?? "neutral"}>
                      {inc.severity}
                    </Badge>{" "}
                    {inc.type} · {inc.status} · {fmt(inc.occurred_at)}
                  </button>
                </li>
              ))}
            </ul>
          )}
        </Card>

        <Card title={selected ? `Incident ${selected.id.slice(0, 8)}` : "Select an incident"}>
          {!selected ? (
            <p className="muted">Choose an incident to see its trail and actions.</p>
          ) : (
            <div className="stack">
              <p>
                <Badge tone={SEVERITY_TONE[selected.severity] ?? "neutral"}>
                  {selected.severity}
                </Badge>{" "}
                <Badge tone="neutral">{selected.status}</Badge>
              </p>
              <p className="muted">
                Type {selected.type} · occurred {fmt(selected.occurred_at)}
                {selected.booking_id ? ` · booking ${selected.booking_id.slice(0, 8)}` : ""}
              </p>

              {actionError ? <Alert tone="error">{actionError}</Alert> : null}

              {!terminal ? (
                <div className="nav-actions" aria-label="Workflow actions">
                  {selected.status === "open" ? (
                    <Button disabled={busy} onClick={() => void act("/acknowledge")}>
                      Acknowledge
                    </Button>
                  ) : null}
                  <Button
                    variant="secondary"
                    disabled={busy}
                    onClick={() => void act("/escalate")}
                  >
                    Escalate
                  </Button>
                  <Button
                    variant="secondary"
                    disabled={busy}
                    onClick={() => {
                      const resolution = window.prompt("Resolution note (required)");
                      if (resolution) void act("/resolve", { note: resolution });
                    }}
                  >
                    Resolve
                  </Button>
                  <Button
                    variant="secondary"
                    disabled={busy}
                    onClick={() => void act("/close", {})}
                  >
                    Close
                  </Button>
                </div>
              ) : null}

              {!terminal ? (
                <form
                  className="stack"
                  onSubmit={(e) => {
                    e.preventDefault();
                    if (note.trim()) {
                      void act("/notes", { body: note.trim() });
                      setNote("");
                    }
                  }}
                >
                  <label htmlFor="incident-note">Add note</label>
                  <input
                    id="incident-note"
                    value={note}
                    onChange={(e) => setNote(e.target.value)}
                    placeholder="Timestamped case note"
                  />
                  <Button type="submit" variant="secondary" disabled={busy || !note.trim()}>
                    Add note
                  </Button>
                </form>
              ) : null}

              {!terminal ? (
                <form
                  className="stack"
                  onSubmit={(e) => {
                    e.preventDefault();
                    if (assignee.trim()) {
                      void act("/assign", { user_id: assignee.trim() });
                      setAssignee("");
                    }
                  }}
                >
                  <label htmlFor="incident-assign">Assign to user id</label>
                  <input
                    id="incident-assign"
                    value={assignee}
                    onChange={(e) => setAssignee(e.target.value)}
                    placeholder="Operations user UUID"
                  />
                  <Button type="submit" variant="secondary" disabled={busy || !assignee.trim()}>
                    Assign
                  </Button>
                </form>
              ) : null}

              <section aria-label="Audit trail">
                <h2>Trail</h2>
                {events.length === 0 ? (
                  <p className="muted">No trail entries yet.</p>
                ) : (
                  <ol className="stack">
                    {events.map((ev) => (
                      <li key={ev.id}>
                        <strong>{ev.kind}</strong> · {fmt(ev.created_at)}
                        {ev.actor_id ? ` · by ${ev.actor_id.slice(0, 8)}` : ""}
                        {ev.body ? <p className="muted">{ev.body}</p> : null}
                      </li>
                    ))}
                  </ol>
                )}
              </section>
            </div>
          )}
        </Card>
      </div>
    </div>
  );
}
