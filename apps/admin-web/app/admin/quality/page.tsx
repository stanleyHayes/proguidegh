"use client";

import { useCallback, useEffect, useState } from "react";
import { Alert, Badge, Button, Card } from "@proguidegh/ui";
import { api, ApiError, errorMessage } from "../../lib/api";
import { Unauthorized } from "../../components/Unauthorized";

/**
 * Quality & retraining queue (spec §4.4, P6-06).
 *
 * Flags are opened server-side by the review aggregation rules (rolling
 * average below the low threshold, or Elite qualification). Resolving
 * requires a note — the resolution is audited on the server.
 */

interface QualityFlag {
  id: string;
  guide_id: string;
  guide_name?: string | null;
  kind: string;
  status: string;
  rating_avg_at_flag: number;
  detail?: string | null;
  created_at: string;
  resolved_at?: string | null;
  resolution_note?: string | null;
}

type LoadState = "loading" | "unauthorized" | "forbidden" | "error" | "ready";

function asRecord(value: unknown): Record<string, unknown> | null {
  return value !== null && typeof value === "object"
    ? (value as Record<string, unknown>)
    : null;
}

function parseFlags(data: unknown): QualityFlag[] {
  const list = asRecord(data)?.flags;
  if (!Array.isArray(list)) return [];
  return list
    .map((entry) => asRecord(entry))
    .filter((r): r is Record<string, unknown> => r !== null && typeof r.id === "string")
    .map((r) => ({
      id: r.id as string,
      guide_id: String(r.guide_id ?? ""),
      guide_name: (r.guide_name as string | null) ?? null,
      kind: String(r.kind ?? ""),
      status: String(r.status ?? "open"),
      rating_avg_at_flag: Number(r.rating_avg_at_flag ?? 0),
      detail: (r.detail as string | null) ?? null,
      created_at: String(r.created_at ?? ""),
      resolved_at: (r.resolved_at as string | null) ?? null,
      resolution_note: (r.resolution_note as string | null) ?? null,
    }));
}

function fmt(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleDateString(undefined, { day: "numeric", month: "short", year: "numeric" });
}

export default function QualityQueuePage() {
  const [state, setState] = useState<LoadState>("loading");
  const [showResolved, setShowResolved] = useState(false);
  const [flags, setFlags] = useState<QualityFlag[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [busyId, setBusyId] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      const data = await api(`/admin/quality-flags${showResolved ? "" : "?status=open"}`);
      setFlags(parseFlags(data));
      setState("ready");
      setError(null);
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        setState("unauthorized");
      } else if (err instanceof ApiError && err.status === 403) {
        setState("forbidden");
      } else {
        setError(errorMessage(err, "Could not load the quality queue."));
        setState("error");
      }
    }
  }, [showResolved]);

  useEffect(() => {
    void load();
  }, [load]);

  async function resolve(flag: QualityFlag) {
    const note = window.prompt(
      `Resolution note for ${flag.guide_name ?? flag.guide_id} (required)`,
    );
    if (!note) return;
    setBusyId(flag.id);
    setError(null);
    try {
      await api(`/admin/quality-flags/${flag.id}/resolve`, {
        method: "POST",
        body: { note },
      });
      await load();
    } catch (err) {
      setError(errorMessage(err, "Could not resolve the flag."));
    } finally {
      setBusyId(null);
    }
  }

  if (state === "unauthorized" || state === "forbidden") {
    return <Unauthorized />;
  }

  return (
    <div className="stack">
      <section aria-labelledby="quality-heading">
        <h1 id="quality-heading">Quality &amp; retraining</h1>
        <p className="muted">
          Opened automatically by the review rules (§4.4): rolling average
          below the low threshold flags retraining; above the Elite bar flags
          an Elite qualification review.
        </p>
        <div className="nav-actions" role="group" aria-label="Filter flags">
          <Button
            variant={!showResolved ? "primary" : "secondary"}
            onClick={() => setShowResolved(false)}
          >
            Open
          </Button>
          <Button
            variant={showResolved ? "primary" : "secondary"}
            onClick={() => setShowResolved(true)}
          >
            All
          </Button>
        </div>
      </section>

      {state === "error" && error ? <Alert tone="error">{error}</Alert> : null}

      <Card title={`Flags (${flags.length})`}>
        {flags.length === 0 ? (
          <p className="muted">
            {showResolved ? "No flags yet." : "Queue clear — no open flags."}
          </p>
        ) : (
          <ul className="stack" aria-label="Quality flags">
            {flags.map((flag) => (
              <li key={flag.id} className="stack">
                <p>
                  <Badge tone={flag.kind === "low_rating" ? "danger" : "success"}>
                    {flag.kind === "low_rating" ? "Retraining" : "Elite review"}
                  </Badge>{" "}
                  <strong>{flag.guide_name ?? flag.guide_id.slice(0, 8)}</strong> ·
                  rating {flag.rating_avg_at_flag.toFixed(2)} · {fmt(flag.created_at)}{" "}
                  {flag.status === "resolved" ? (
                    <Badge tone="neutral">resolved</Badge>
                  ) : null}
                </p>
                {flag.detail ? <p className="muted">{flag.detail}</p> : null}
                {flag.status === "resolved" && flag.resolution_note ? (
                  <p className="muted">Resolution: {flag.resolution_note}</p>
                ) : null}
                {flag.status === "open" ? (
                  <p>
                    <Button
                      variant="secondary"
                      disabled={busyId === flag.id}
                      onClick={() => void resolve(flag)}
                    >
                      Resolve with note
                    </Button>
                  </p>
                ) : null}
              </li>
            ))}
          </ul>
        )}
      </Card>
    </div>
  );
}
