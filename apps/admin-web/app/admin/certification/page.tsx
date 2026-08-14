"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { Alert, Badge, Button } from "@proguidegh/ui";
import { api, ApiError, errorMessage } from "../../lib/api";
import { Unauthorized } from "../../components/Unauthorized";
import {
  ALL_STATUSES,
  formatDate,
  stageLabel,
  statusTone,
} from "../../lib/certification";

/** Assumed shape of GET /admin/certification/queue entries (spec §5, §18.3). */
interface QueueEntry {
  case_id?: string;
  id?: string;
  guide?: { id?: string; public_name?: string };
  status?: string;
  opened_at?: string;
}

type LoadState = "loading" | "unauthorized" | "forbidden" | "error" | "ready";

function parseQueue(data: unknown): QueueEntry[] {
  if (Array.isArray(data)) return data as QueueEntry[];
  for (const key of ["cases", "queue", "items"]) {
    if (data !== null && typeof data === "object" && key in data) {
      const list = (data as Record<string, unknown>)[key];
      if (Array.isArray(list)) return list as QueueEntry[];
    }
  }
  return [];
}

function entryCaseId(entry: QueueEntry): string {
  return entry.case_id ?? entry.id ?? "";
}

export default function AdminCertificationQueuePage() {
  const [state, setState] = useState<LoadState>("loading");
  const [loadError, setLoadError] = useState<string | null>(null);
  const [entries, setEntries] = useState<QueueEntry[]>([]);
  const [statusFilter, setStatusFilter] = useState<string>("ALL");

  const load = useCallback(async (status: string) => {
    setState("loading");
    setLoadError(null);
    try {
      const query = status === "ALL" ? "" : `?status=${encodeURIComponent(status)}`;
      const data = await api<unknown>(`/admin/certification/queue${query}`);
      setEntries(parseQueue(data));
      setState("ready");
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        setState("unauthorized");
      } else if (err instanceof ApiError && err.status === 403) {
        setState("forbidden");
      } else {
        setLoadError(
          errorMessage(err, "Could not load the certification queue. Please retry."),
        );
        setState("error");
      }
    }
  }, []);

  useEffect(() => {
    void load(statusFilter);
  }, [load, statusFilter]);

  if (state === "unauthorized" || state === "forbidden") {
    return (
      <div className="stack">
        <h1>Certification queue</h1>
        <Unauthorized forbidden={state === "forbidden"} />
      </div>
    );
  }

  return (
    <div className="stack">
      <section aria-labelledby="queue-heading">
        <h1 id="queue-heading">Certification queue</h1>
        <p className="muted">
          Review certification cases by pipeline stage. Transitions happen on
          the case detail screen and are audited by the backend.
        </p>
      </section>

      <div className="filter-tabs" role="group" aria-label="Filter by stage">
        {["ALL", ...ALL_STATUSES].map((status) => (
          <Button
            key={status}
            type="button"
            variant={statusFilter === status ? "primary" : "secondary"}
            aria-pressed={statusFilter === status}
            onClick={() => setStatusFilter(status)}
          >
            {status === "ALL" ? "All" : stageLabel(status)}
          </Button>
        ))}
      </div>

      {state === "error" ? (
        <>
          <Alert tone="error" title="Something went wrong">
            <p>{loadError}</p>
          </Alert>
          <div>
            <Button type="button" onClick={() => void load(statusFilter)}>
              Retry
            </Button>
          </div>
        </>
      ) : null}

      {state === "loading" ? (
        <div className="stack" aria-busy="true" aria-label="Loading certification queue">
          {Array.from({ length: 5 }, (_, i) => (
            <div key={i} className="gg-skeleton" style={{ height: "3rem" }} />
          ))}
        </div>
      ) : null}

      {state === "ready" && entries.length === 0 ? (
        <Alert tone="info" title="Queue empty">
          <p>
            No certification cases
            {statusFilter === "ALL"
              ? " found."
              : ` in stage “${stageLabel(statusFilter)}”.`}
          </p>
        </Alert>
      ) : null}

      {state === "ready" && entries.length > 0 ? (
        <div className="gg-table-scroll">
          <table className="gg-table">
            <thead>
              <tr>
                <th scope="col">Guide</th>
                <th scope="col">Status</th>
                <th scope="col">Opened</th>
                <th scope="col">Case</th>
              </tr>
            </thead>
            <tbody>
              {entries.map((entry) => {
                const caseId = entryCaseId(entry);
                return (
                  <tr key={caseId || entry.guide?.id}>
                    <td>{entry.guide?.public_name ?? "—"}</td>
                    <td>
                      <Badge tone={statusTone(entry.status)}>
                        {stageLabel(entry.status)}
                      </Badge>
                    </td>
                    <td>{formatDate(entry.opened_at)}</td>
                    <td>
                      {caseId ? (
                        <Link href={`/admin/certification/${caseId}`}>
                          Review case
                        </Link>
                      ) : (
                        "—"
                      )}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      ) : null}

      <p className="muted">
        Current filter:{" "}
        {statusFilter === "ALL" ? "all stages" : stageLabel(statusFilter)} ·{" "}
        {entries.length} case{entries.length === 1 ? "" : "s"}
      </p>
    </div>
  );
}
