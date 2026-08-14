"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { Alert, Button, Card } from "@proguidegh/ui";
import { api, ApiError, errorMessage } from "../../lib/api";
import { formatPrice } from "../../lib/catalog";
import { formatDateTime, parseReceipt, type Receipt } from "../../lib/bookings";

type LoadState =
  | "loading"
  | "unauthenticated"
  | "not-ready"
  | "error"
  | "ready";

/**
 * Receipt view for a paid booking (spec §17). The receipt is generated
 * server-side only after the payment webhook confirms the charge, so a 404
 * means "not issued yet" rather than an error.
 */
export default function ReceiptClient({ bookingId }: { bookingId: string }) {
  const [state, setState] = useState<LoadState>("loading");
  const [loadError, setLoadError] = useState<string | null>(null);
  const [receipt, setReceipt] = useState<Receipt | null>(null);

  const load = useCallback(async () => {
    setState("loading");
    setLoadError(null);
    try {
      const data = await api<unknown>(`/bookings/${bookingId}/receipt`);
      setReceipt(parseReceipt(data));
      setState("ready");
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        setState("unauthenticated");
      } else if (err instanceof ApiError && err.status === 404) {
        setState("not-ready");
      } else {
        setLoadError(
          errorMessage(err, "Could not load this receipt. Please retry."),
        );
        setState("error");
      }
    }
  }, [bookingId]);

  useEffect(() => {
    void load();
  }, [load]);

  if (state === "loading") {
    return (
      <div className="stack" aria-busy="true" aria-label="Loading your receipt">
        <div className="gg-skeleton" style={{ height: "2rem", width: "40%" }} />
        <div className="gg-skeleton" style={{ height: "12rem" }} />
      </div>
    );
  }

  if (state === "unauthenticated") {
    return (
      <div className="stack">
        <h1>Receipt</h1>
        <Alert tone="info" title="Sign in required">
          <p>You need to sign in to view this receipt.</p>
        </Alert>
        <p>
          <Link className="gg-button gg-button--primary" href="/login">
            Sign in
          </Link>
        </p>
      </div>
    );
  }

  if (state === "not-ready") {
    return (
      <div className="stack">
        <h1>Receipt</h1>
        <Alert tone="info" title="Receipt not available yet">
          <p>
            The receipt is issued once the payment is confirmed. If you just
            paid, this can take a moment — please check again shortly.
          </p>
        </Alert>
        <div className="nav-actions">
          <Button type="button" onClick={() => void load()}>
            Check again
          </Button>
          <Link
            className="gg-button gg-button--secondary"
            href={`/bookings/${bookingId}`}
          >
            Booking details
          </Link>
        </div>
      </div>
    );
  }

  if (state === "error" || !receipt) {
    return (
      <div className="stack">
        <h1>Receipt</h1>
        <Alert tone="error" title="Something went wrong">
          <p>{loadError}</p>
        </Alert>
        <div>
          <Button type="button" onClick={() => void load()}>
            Retry
          </Button>
        </div>
      </div>
    );
  }

  return (
    <div className="stack">
      <section aria-labelledby="receipt-heading">
        <h1 id="receipt-heading">
          Receipt {receipt.receipt_number ?? ""}
        </h1>
        <p className="muted">
          Issued by ProGuideGH after your confirmed payment.
        </p>
      </section>

      <Card title="Payment receipt">
        <dl className="quote-rows">
          <div className="quote-row">
            <dt>Receipt number</dt>
            <dd>{receipt.receipt_number ?? "—"}</dd>
          </div>
          <div className="quote-row">
            <dt>Issued</dt>
            <dd>{formatDateTime(receipt.issued_at)}</dd>
          </div>
          {receipt.amount !== undefined ? (
            <div className="quote-row">
              <dt>Amount paid</dt>
              <dd>{formatPrice(receipt.amount, receipt.currency)}</dd>
            </div>
          ) : null}
        </dl>
      </Card>

      <nav aria-label="Receipt actions" className="nav-actions">
        {receipt.download_url ? (
          <a
            className="gg-button gg-button--primary"
            href={receipt.download_url}
            target="_blank"
            rel="noopener noreferrer"
          >
            Download PDF
          </a>
        ) : null}
        <Link
          className="gg-button gg-button--secondary"
          href={`/bookings/${bookingId}`}
        >
          Booking details
        </Link>
        <Link className="gg-button gg-button--secondary" href="/bookings">
          My bookings
        </Link>
      </nav>
    </div>
  );
}
