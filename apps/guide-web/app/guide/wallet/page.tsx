"use client";

import { useCallback, useEffect, useState } from "react";
import { Alert, Badge, Button, Card } from "@proguidegh/ui";
import { api, ApiError, errorMessage, unwrap } from "../../lib/api";

/**
 * Guide wallet (spec §8.1, Phase 7): balance summary, the earnings/payout
 * statement and the payout-account registration form.
 *
 * Balances are server-computed from the immutable ledger — eligible is net
 * of payout drawdowns, and payout-eligible only counts earnings whose
 * tours completed beyond the payout-delay hold (§8.4). Account references
 * are encrypted at rest; only the masked form ever comes back.
 */

interface Wallet {
  pending_minor: number;
  eligible_minor: number;
  payout_eligible_minor: number;
  in_flight_minor: number;
  paid_total_minor: number;
  payout_delay_days: number;
}

interface StatementEntry {
  at: string;
  id: string;
  kind: string; // ledger | payout
  reference: string;
  detail: string;
  amount_minor: number;
}

interface PayoutAccount {
  id: string;
  provider: string;
  network?: string | null;
  masked_ref: string;
  verified_at?: string | null;
}

type LoadState = "loading" | "unauthenticated" | "error" | "ready";

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

function fmt(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleDateString(undefined, {
    day: "numeric",
    month: "short",
    year: "numeric",
  });
}

export default function WalletPage() {
  const [state, setState] = useState<LoadState>("loading");
  const [wallet, setWallet] = useState<Wallet | null>(null);
  const [account, setAccount] = useState<PayoutAccount | null>(null);
  const [entries, setEntries] = useState<StatementEntry[]>([]);
  const [cursor, setCursor] = useState<string | null>(null);
  const [moreBusy, setMoreBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  const [provider, setProvider] = useState("mtn_momo");
  const [network, setNetwork] = useState("MTN");
  const [accountRef, setAccountRef] = useState("");
  const [saveBusy, setSaveBusy] = useState(false);

  const parseStatement = useCallback((data: unknown): StatementEntry[] => {
    const list = asRecord(data)?.entries;
    if (!Array.isArray(list)) return [];
    return list
      .map((entry) => asRecord(entry))
      .filter((r): r is Record<string, unknown> => r !== null && typeof r.id === "string")
      .map((r) => ({
        at: String(r.at ?? ""),
        id: r.id as string,
        kind: String(r.kind ?? ""),
        reference: String(r.reference ?? ""),
        detail: String(r.detail ?? ""),
        amount_minor: Number(r.amount_minor ?? 0),
      }));
  }, []);

  const load = useCallback(async () => {
    try {
      const [walletData, accountData, statementData] = await Promise.all([
        api("/me/guide/wallet"),
        api("/me/guide/payout-account"),
        api("/me/guide/statement?limit=20"),
      ]);
      setWallet(unwrap<Wallet>(walletData, "wallet"));
      setAccount(unwrap<PayoutAccount | null>(accountData, "account"));
      setEntries(parseStatement(statementData));
      setCursor((asRecord(statementData)?.next_cursor as string | null) ?? null);
      setState("ready");
      setError(null);
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        setState("unauthenticated");
      } else {
        setError(errorMessage(err, "Could not load your wallet."));
        setState("error");
      }
    }
  }, [parseStatement]);

  useEffect(() => {
    void load();
  }, [load]);

  async function loadMore() {
    if (!cursor) return;
    setMoreBusy(true);
    try {
      const data = await api(
        `/me/guide/statement?limit=20&cursor=${encodeURIComponent(cursor)}`,
      );
      setEntries((prev) => [...prev, ...parseStatement(data)]);
      setCursor((asRecord(data)?.next_cursor as string | null) ?? null);
    } catch (err) {
      setError(errorMessage(err, "Could not load more statement entries."));
    } finally {
      setMoreBusy(false);
    }
  }

  async function saveAccount(event: React.FormEvent) {
    event.preventDefault();
    setSaveBusy(true);
    setError(null);
    setNotice(null);
    try {
      const data = await api("/me/guide/payout-account", {
        method: "PUT",
        body: { provider, network, account_ref: accountRef },
      });
      setAccount(unwrap<PayoutAccount>(data, "account"));
      setAccountRef("");
      setNotice("Payout account saved. Finance will verify it before the next batch.");
    } catch (err) {
      setError(errorMessage(err, "Could not save the payout account."));
    } finally {
      setSaveBusy(false);
    }
  }

  if (state === "unauthenticated") {
    return (
      <div className="stack">
        <h1>Wallet</h1>
        <Alert tone="info">Sign in to view your wallet.</Alert>
      </div>
    );
  }

  return (
    <div className="stack">
      <section aria-labelledby="wallet-heading">
        <h1 id="wallet-heading">Wallet</h1>
        <p className="muted">
          Earnings become payout-eligible {wallet?.payout_delay_days ?? 7} days
          after a tour completes; payouts run weekly.
        </p>
      </section>

      {notice ? <Alert tone="success">{notice}</Alert> : null}
      {error ? <Alert tone="error">{error}</Alert> : null}

      <div className="grid grid--cols-3" aria-label="Balances">
        <Card title="Pending">
          <p className="stat">{money(wallet?.pending_minor ?? 0)}</p>
          Awaiting tour completion.
        </Card>
        <Card title="Eligible">
          <p className="stat">{money(wallet?.eligible_minor ?? 0)}</p>
          {money(wallet?.payout_eligible_minor ?? 0)} payout-eligible now.
        </Card>
        <Card title="Paid out">
          <p className="stat">{money(wallet?.paid_total_minor ?? 0)}</p>
          {money(wallet?.in_flight_minor ?? 0)} in flight.
        </Card>
      </div>

      <Card title="Payout account">
        {account ? (
          <p>
            <Badge tone={account.verified_at ? "success" : "warning"}>
              {account.verified_at ? "Verified" : "Awaiting verification"}
            </Badge>{" "}
            {account.provider}
            {account.network ? ` · ${account.network}` : ""} · {account.masked_ref}
          </p>
        ) : (
          <p className="muted">No payout account registered yet.</p>
        )}
        <form className="stack" onSubmit={(e) => void saveAccount(e)}>
          <label>
            Provider
            <select value={provider} onChange={(e) => setProvider(e.target.value)}>
              <option value="mtn_momo">MTN Mobile Money</option>
              <option value="vodafone_cash">Telecel Cash</option>
              <option value="airteltigo">AT Money</option>
              <option value="bank">Bank transfer</option>
            </select>
          </label>
          <label>
            Network
            <input
              value={network}
              onChange={(e) => setNetwork(e.target.value)}
              placeholder="MTN"
            />
          </label>
          <label>
            Account number
            <input
              value={accountRef}
              onChange={(e) => setAccountRef(e.target.value)}
              placeholder="024 400 0111"
              required
            />
          </label>
          <p>
            <Button variant="primary" disabled={saveBusy || accountRef.trim() === ""}>
              {saveBusy ? "Saving…" : account ? "Replace account" : "Save account"}
            </Button>
          </p>
        </form>
      </Card>

      <Card title={`Statement (${entries.length})`}>
        {state === "loading" ? (
          <p className="muted">Loading…</p>
        ) : entries.length === 0 ? (
          <p className="muted">No earnings yet — completed tours will appear here.</p>
        ) : (
          <>
            <ul className="stack" aria-label="Statement entries">
              {entries.map((entry) => (
                <li key={`${entry.kind}-${entry.id}`}>
                  <p>
                    <Badge tone={entry.amount_minor >= 0 ? "success" : "neutral"}>
                      {entry.kind === "payout" ? "Payout" : "Earnings"}
                    </Badge>{" "}
                    {money(entry.amount_minor)} · {entry.detail} · {fmt(entry.at)}
                  </p>
                </li>
              ))}
            </ul>
            {cursor ? (
              <p>
                <Button variant="secondary" disabled={moreBusy} onClick={() => void loadMore()}>
                  {moreBusy ? "Loading…" : "Load more"}
                </Button>
              </p>
            ) : null}
          </>
        )}
      </Card>
    </div>
  );
}
