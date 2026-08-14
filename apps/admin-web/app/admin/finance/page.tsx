"use client";

import { useCallback, useEffect, useState } from "react";
import { Alert, Badge, Button, Card } from "@proguidegh/ui";
import { api, ApiError, API_BASE_URL, errorMessage } from "../../lib/api";
import { Unauthorized } from "../../components/Unauthorized";

/**
 * Finance console (spec §8.4, Phase 7): the payout batch, the payout state
 * machine, the transfer CSV export and the tourism-levy report.
 *
 * The batch queues one payout per guide whose eligible balance has cleared
 * the payout-delay hold; re-running it for the same date is a server-side
 * no-op (P7-07). PAID postings are ledger-backed and atomic on the server.
 */

interface Payout {
  id: string;
  guide_id: string;
  guide_name?: string | null;
  amount_minor: number;
  currency: string;
  status: string;
  provider_reference?: string | null;
  scheduled_for?: string | null;
  failure_reason?: string | null;
  ledger_transaction_id?: string | null;
  created_at: string;
}

interface LevyReport {
  balance_minor: number;
  period_credits_minor: number;
  period_debits_minor: number;
  currency: string;
}

type LoadState = "loading" | "unauthorized" | "forbidden" | "error" | "ready";

const STATUS_FILTERS = [
  "QUEUED",
  "PROCESSING",
  "RETRY_QUEUED",
  "MANUAL_REVIEW",
  "FAILED",
  "PAID",
] as const;

const NEXT_ACTIONS: Record<string, string[]> = {
  PENDING_ELIGIBILITY: ["QUEUED", "MANUAL_REVIEW"],
  ELIGIBLE: ["QUEUED", "MANUAL_REVIEW"],
  QUEUED: ["PROCESSING", "MANUAL_REVIEW"],
  PROCESSING: ["PAID", "FAILED", "MANUAL_REVIEW"],
  FAILED: ["RETRY_QUEUED", "MANUAL_REVIEW"],
  RETRY_QUEUED: ["PROCESSING", "MANUAL_REVIEW"],
  MANUAL_REVIEW: ["RETRY_QUEUED"],
  PAID: [],
};

function asRecord(value: unknown): Record<string, unknown> | null {
  return value !== null && typeof value === "object"
    ? (value as Record<string, unknown>)
    : null;
}

function parsePayouts(data: unknown): Payout[] {
  const list = asRecord(data)?.payouts;
  if (!Array.isArray(list)) return [];
  return list
    .map((entry) => asRecord(entry))
    .filter((r): r is Record<string, unknown> => r !== null && typeof r.id === "string")
    .map((r) => ({
      id: r.id as string,
      guide_id: String(r.guide_id ?? ""),
      guide_name: (r.guide_name as string | null) ?? null,
      amount_minor: Number(r.amount_minor ?? 0),
      currency: String(r.currency ?? "GHS"),
      status: String(r.status ?? ""),
      provider_reference: (r.provider_reference as string | null) ?? null,
      scheduled_for: (r.scheduled_for as string | null) ?? null,
      failure_reason: (r.failure_reason as string | null) ?? null,
      ledger_transaction_id: (r.ledger_transaction_id as string | null) ?? null,
      created_at: String(r.created_at ?? ""),
    }));
}

function parseLevy(data: unknown): LevyReport | null {
  const r = asRecord(asRecord(data)?.report);
  if (r === null) return null;
  return {
    balance_minor: Number(r.balance_minor ?? 0),
    period_credits_minor: Number(r.period_credits_minor ?? 0),
    period_debits_minor: Number(r.period_debits_minor ?? 0),
    currency: String(r.currency ?? "GHS"),
  };
}

function money(minor: number, currency = "GHS"): string {
  return new Intl.NumberFormat(undefined, {
    style: "currency",
    currency,
  }).format(minor / 100);
}

function toneFor(status: string): "neutral" | "success" | "warning" | "danger" {
  if (status === "PAID") return "success";
  if (status === "FAILED") return "danger";
  if (status === "MANUAL_REVIEW" || status === "PROCESSING") return "warning";
  return "neutral";
}

export default function FinancePage() {
  const today = new Date().toISOString().slice(0, 10);
  const [state, setState] = useState<LoadState>("loading");
  const [statusFilter, setStatusFilter] = useState<string>("");
  const [payouts, setPayouts] = useState<Payout[]>([]);
  const [levy, setLevy] = useState<LevyReport | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [busyId, setBusyId] = useState<string | null>(null);
  const [batchBusy, setBatchBusy] = useState(false);

  const load = useCallback(async () => {
    try {
      const query = statusFilter ? `?status=${statusFilter}` : "";
      const [payoutData, levyData] = await Promise.all([
        api(`/admin/payouts${query}`),
        api("/admin/reports/tourism-levy"),
      ]);
      setPayouts(parsePayouts(payoutData));
      setLevy(parseLevy(levyData));
      setState("ready");
      setError(null);
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        setState("unauthorized");
      } else if (err instanceof ApiError && err.status === 403) {
        setState("forbidden");
      } else {
        setError(errorMessage(err, "Could not load finance data."));
        setState("error");
      }
    }
  }, [statusFilter]);

  useEffect(() => {
    void load();
  }, [load]);

  async function runBatch() {
    setBatchBusy(true);
    setError(null);
    setNotice(null);
    try {
      const data = asRecord(
        await api("/admin/payouts/batch", { method: "POST", body: {} }),
      );
      setNotice(
        `Batch ${String(data?.scheduled_for ?? "")}: ${String(data?.created ?? 0)} payout(s) queued.`,
      );
      await load();
    } catch (err) {
      setError(errorMessage(err, "Could not run the payout batch."));
    } finally {
      setBatchBusy(false);
    }
  }

  async function transition(payout: Payout, to: string) {
    const body: Record<string, string> = { to };
    if (to === "FAILED") {
      const reason = window.prompt("Failure reason (required)");
      if (!reason) return;
      body.failure_reason = reason;
    }
    if (to === "PAID") {
      const ref = window.prompt("Provider reference (optional)") ?? "";
      if (ref) body.provider_reference = ref;
    }
    setBusyId(payout.id);
    setError(null);
    try {
      await api(`/admin/payouts/${payout.id}/transition`, {
        method: "POST",
        body,
      });
      await load();
    } catch (err) {
      setError(errorMessage(err, `Could not move the payout to ${to}.`));
    } finally {
      setBusyId(null);
    }
  }

  if (state === "unauthorized" || state === "forbidden") {
    return <Unauthorized />;
  }

  return (
    <div className="stack">
      <section aria-labelledby="finance-heading">
        <h1 id="finance-heading">Finance</h1>
        <p className="muted">
          Weekly payout batch, payout transitions and the tourism-levy report
          (§8.4). Batches are idempotent per guide and date.
        </p>
        <div className="nav-actions" role="group" aria-label="Finance actions">
          <Button variant="primary" disabled={batchBusy} onClick={() => void runBatch()}>
            {batchBusy ? "Running…" : "Run payout batch"}
          </Button>
          <a
            className="gg-button gg-button--secondary"
            href={`${API_BASE_URL}/api/v1/admin/payouts/export?scheduled_for=${today}`}
          >
            Export today&rsquo;s CSV
          </a>
        </div>
      </section>

      {notice ? <Alert tone="success">{notice}</Alert> : null}
      {state === "error" && error ? <Alert tone="error">{error}</Alert> : null}
      {state === "ready" && error ? <Alert tone="error">{error}</Alert> : null}

      <Card title="Tourism levy">
        {levy ? (
          <p>
            Payable balance <strong>{money(levy.balance_minor, levy.currency)}</strong>
          </p>
        ) : (
          <p className="muted">Loading…</p>
        )}
      </Card>

      <Card title={`Payouts (${payouts.length})`}>
        <div className="nav-actions" role="group" aria-label="Filter by status">
          <Button
            variant={statusFilter === "" ? "primary" : "secondary"}
            onClick={() => setStatusFilter("")}
          >
            All
          </Button>
          {STATUS_FILTERS.map((s) => (
            <Button
              key={s}
              variant={statusFilter === s ? "primary" : "secondary"}
              onClick={() => setStatusFilter(s)}
            >
              {s.replace(/_/g, " ")}
            </Button>
          ))}
        </div>
        {payouts.length === 0 ? (
          <p className="muted">No payouts match this filter.</p>
        ) : (
          <ul className="stack" aria-label="Payouts">
            {payouts.map((payout) => (
              <li key={payout.id} className="stack">
                <p>
                  <Badge tone={toneFor(payout.status)}>{payout.status}</Badge>{" "}
                  <strong>{payout.guide_name ?? payout.guide_id.slice(0, 8)}</strong> ·{" "}
                  {money(payout.amount_minor, payout.currency)}
                  {payout.scheduled_for ? ` · scheduled ${payout.scheduled_for}` : ""}
                </p>
                {payout.failure_reason ? (
                  <p className="muted">Failure: {payout.failure_reason}</p>
                ) : null}
                {payout.provider_reference ? (
                  <p className="muted">Provider ref: {payout.provider_reference}</p>
                ) : null}
                {(NEXT_ACTIONS[payout.status] ?? []).length > 0 ? (
                  <p className="nav-actions">
                    {(NEXT_ACTIONS[payout.status] ?? []).map((to) => (
                      <Button
                        key={to}
                        variant={to === "FAILED" ? "secondary" : "primary"}
                        disabled={busyId === payout.id}
                        onClick={() => void transition(payout, to)}
                      >
                        {to.replace(/_/g, " ")}
                      </Button>
                    ))}
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
