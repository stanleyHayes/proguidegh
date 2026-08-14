"use client";

import { useCallback, useEffect, useState } from "react";
import { Alert, Badge, Card } from "@proguidegh/ui";
import { api, ApiError, errorMessage } from "../../lib/api";
import { Unauthorized } from "../../components/Unauthorized";

/**
 * Reports console (P8-02): executive KPIs, the bookings report and the
 * permitted CSV export (reports.export — the link carries the session
 * cookie like every other request).
 */

interface KPIs {
  users_total: number;
  guides_certified: number;
  bookings_30d: number;
  bookings_active: number;
  gmv_30d_minor: number;
  platform_revenue_30d_minor: number;
  sos_30d: number;
  average_rating: number;
  reviews_total: number;
  payouts_paid_30d_minor: number;
}

interface BookingsReport {
  from: string;
  to: string;
  total: number;
  by_status: { status: string; count: number }[];
  gmv_minor: number;
  refunded_minor: number;
}

type LoadState = "loading" | "unauthorized" | "forbidden" | "error" | "ready";

function asRecord(value: unknown): Record<string, unknown> | null {
  return value !== null && typeof value === "object"
    ? (value as Record<string, unknown>)
    : null;
}

function money(minor: number): string {
  return new Intl.NumberFormat(undefined, {
    style: "currency",
    currency: "GHS",
  }).format(minor / 100);
}

export default function ReportsPage() {
  const [state, setState] = useState<LoadState>("loading");
  const [kpis, setKpis] = useState<KPIs | null>(null);
  const [report, setReport] = useState<BookingsReport | null>(null);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      const [kpiData, reportData] = await Promise.all([
        api("/admin/reports/kpis"),
        api("/admin/reports/bookings"),
      ]);
      setKpis(asRecord(asRecord(kpiData)?.kpis) as unknown as KPIs);
      setReport(asRecord(asRecord(reportData)?.report) as unknown as BookingsReport);
      setState("ready");
      setError(null);
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        setState("unauthorized");
      } else if (err instanceof ApiError && err.status === 403) {
        setState("forbidden");
      } else {
        setError(errorMessage(err, "Could not load reports."));
        setState("error");
      }
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  if (state === "unauthorized" || state === "forbidden") {
    return <Unauthorized />;
  }

  return (
    <div className="stack">
      <section aria-labelledby="reports-heading">
        <h1 id="reports-heading">Reports</h1>
        <p className="muted">
          Executive KPIs and the operational bookings report (last 30 days).
        </p>
      </section>

      {state === "error" && error ? <Alert tone="error">{error}</Alert> : null}

      <div className="grid grid--cols-4" aria-label="Key performance indicators">
        <Card title="Users">
          <p className="stat">{kpis?.users_total ?? "…"}</p>
          {kpis?.guides_certified ?? 0} certified guides.
        </Card>
        <Card title="Bookings (30d)">
          <p className="stat">{kpis?.bookings_30d ?? "…"}</p>
          {kpis?.bookings_active ?? 0} active now.
        </Card>
        <Card title="GMV (30d)">
          <p className="stat">{money(kpis?.gmv_30d_minor ?? 0)}</p>
          {money(kpis?.platform_revenue_30d_minor ?? 0)} platform revenue.
        </Card>
        <Card title="Rating">
          <p className="stat">{(kpis?.average_rating ?? 0).toFixed(2)}</p>
          {kpis?.reviews_total ?? 0} reviews · {kpis?.sos_30d ?? 0} SOS (30d).
        </Card>
      </div>

      <Card title={`Bookings ${report ? `${report.from} → ${report.to}` : ""}`}>
        {report ? (
          <>
            <p>
              Total <strong>{report.total}</strong> · GMV{" "}
              <strong>{money(report.gmv_minor)}</strong> · refunded{" "}
              {money(report.refunded_minor)} · payouts paid (30d){" "}
              {money(kpis?.payouts_paid_30d_minor ?? 0)}
            </p>
            {report.by_status.length === 0 ? (
              <p className="muted">No bookings in the window.</p>
            ) : (
              <ul aria-label="Bookings by status">
                {report.by_status.map((row) => (
                  <li key={row.status}>
                    <Badge tone="neutral">{row.status}</Badge> {row.count}
                  </li>
                ))}
              </ul>
            )}
          </>
        ) : (
          <p className="muted">Loading…</p>
        )}
      </Card>
    </div>
  );
}
