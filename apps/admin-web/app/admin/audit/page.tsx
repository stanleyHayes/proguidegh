"use client";

import { useCallback, useEffect, useState } from "react";
import { Alert, Badge, Button, Card } from "@proguidegh/ui";
import { api, ApiError, errorMessage } from "../../lib/api";
import { Unauthorized } from "../../components/Unauthorized";

/**
 * Audit viewer (P8-04, spec §1.2): the append-only trail of privileged and
 * financially significant actions, filterable by actor, action prefix,
 * entity and date range.
 */

interface AuditEntry {
  id: string;
  actor_id?: string | null;
  actor_email?: string | null;
  action: string;
  entity_type: string;
  entity_id?: string | null;
  after?: unknown;
  created_at: string;
}

type LoadState = "loading" | "unauthorized" | "forbidden" | "error" | "ready";

function asRecord(value: unknown): Record<string, unknown> | null {
  return value !== null && typeof value === "object"
    ? (value as Record<string, unknown>)
    : null;
}

function fmt(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString();
}

export default function AuditPage() {
  const [state, setState] = useState<LoadState>("loading");
  const [entries, setEntries] = useState<AuditEntry[]>([]);
  const [total, setTotal] = useState(0);
  const [offset, setOffset] = useState(0);
  const [actionFilter, setActionFilter] = useState("");
  const [entityTypeFilter, setEntityTypeFilter] = useState("");
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      const params = new URLSearchParams();
      if (actionFilter) params.set("action", actionFilter);
      if (entityTypeFilter) params.set("entity_type", entityTypeFilter);
      params.set("limit", "50");
      params.set("offset", String(offset));
      const data = await api(`/admin/audit-logs?${params.toString()}`);
      const list = asRecord(data)?.entries;
      setEntries(Array.isArray(list) ? (list as AuditEntry[]) : []);
      setTotal(Number(asRecord(data)?.total ?? 0));
      setState("ready");
      setError(null);
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        setState("unauthorized");
      } else if (err instanceof ApiError && err.status === 403) {
        setState("forbidden");
      } else {
        setError(errorMessage(err, "Could not load the audit trail."));
        setState("error");
      }
    }
  }, [actionFilter, entityTypeFilter, offset]);

  useEffect(() => {
    void load();
  }, [load]);

  if (state === "unauthorized" || state === "forbidden") {
    return <Unauthorized />;
  }

  return (
    <div className="stack">
      <section aria-labelledby="audit-heading">
        <h1 id="audit-heading">Audit trail</h1>
        <p className="muted">
          Append-only record of privileged actions (§1.2). {total} entries
          match the current filters.
        </p>
        <div className="nav-actions" role="group" aria-label="Audit filters">
          <input
            value={actionFilter}
            onChange={(e) => {
              setActionFilter(e.target.value);
              setOffset(0);
            }}
            placeholder="Action prefix (e.g. payout.)"
            aria-label="Filter by action prefix"
          />
          <input
            value={entityTypeFilter}
            onChange={(e) => {
              setEntityTypeFilter(e.target.value);
              setOffset(0);
            }}
            placeholder="Entity type (e.g. payout)"
            aria-label="Filter by entity type"
          />
        </div>
      </section>

      {state === "error" && error ? <Alert tone="error">{error}</Alert> : null}

      <Card title={`Entries (${entries.length})`}>
        {entries.length === 0 ? (
          <p className="muted">{state === "loading" ? "Loading…" : "No entries match."}</p>
        ) : (
          <ul className="stack" aria-label="Audit entries">
            {entries.map((entry) => (
              <li key={entry.id}>
                <p>
                  <Badge tone="neutral">{entry.action}</Badge>{" "}
                  <strong>{entry.actor_email ?? entry.actor_id ?? "system"}</strong> ·{" "}
                  {entry.entity_type}
                  {entry.entity_id ? ` ${entry.entity_id.slice(0, 8)}` : ""} ·{" "}
                  {fmt(entry.created_at)}
                </p>
                {entry.after != null ? (
                  <p className="muted">
                    <code>{JSON.stringify(entry.after)}</code>
                  </p>
                ) : null}
              </li>
            ))}
          </ul>
        )}
        <p className="nav-actions">
          <Button
            variant="secondary"
            disabled={offset === 0}
            onClick={() => setOffset(Math.max(0, offset - 50))}
          >
            Newer
          </Button>
          <Button
            variant="secondary"
            disabled={offset + entries.length >= total}
            onClick={() => setOffset(offset + 50)}
          >
            Older
          </Button>
        </p>
      </Card>
    </div>
  );
}
