"use client";

/**
 * Web account deletion (Phase M, M-20).
 *
 * Google Play's data-deletion policy requires a deletion route reachable
 * WITHOUT installing the app — the URL submitted on the Data Safety form
 * points here. Apple 5.1.1(v) is satisfied by the in-app screen; this page is
 * the web half, and it must work for someone who never had the app.
 *
 * Same contract as the native screen: what is removed and what is retained
 * comes from the server, so this page and the privacy policy cannot drift.
 * A refusal names its specific, temporary reason.
 */

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { Alert, Button } from "@proguidegh/ui";
import { api, errorMessage } from "../../lib/api";

interface Blocker {
  reason: string;
  message: string;
}

interface Preview {
  can_delete?: boolean;
  blockers?: Blocker[];
  retained?: string[];
  removed?: string[];
}

export default function DeleteAccountPage() {
  const [preview, setPreview] = useState<Preview | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [signedOut, setSignedOut] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const [busy, setBusy] = useState(false);
  const [deleteError, setDeleteError] = useState<string | null>(null);
  const [done, setDone] = useState(false);

  const load = useCallback(async () => {
    try {
      setPreview(await api<Preview>("/me/deletion"));
      setLoadError(null);
      setSignedOut(false);
    } catch (err) {
      // 401 is the ordinary case here: someone followed the Play listing link
      // without a session. Tell them how to proceed rather than erroring.
      const message = errorMessage(err, "Could not load your account.");
      if (/unauthenticated|401|sign in/i.test(message)) setSignedOut(true);
      else setLoadError(message);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  async function confirmDelete() {
    setBusy(true);
    setDeleteError(null);
    try {
      await api("/me", { method: "DELETE" });
      setDone(true);
    } catch (err) {
      setDeleteError(errorMessage(err, "Could not delete your account."));
      setConfirming(false);
      void load();
    } finally {
      setBusy(false);
    }
  }

  if (done) {
    return (
      <section className="stack">
        <h1>Your account has been deleted</h1>
        <Alert tone="success" title="Deletion complete">
          Your personal data has been removed and you have been signed out. This
          cannot be undone.
        </Alert>
        <p className="muted">
          Payment records and receipts are kept for tax and tourism-levy
          reconciliation as required by Ghanaian law. They no longer identify you.
        </p>
        <Link href="/">Return to ProGuideGH</Link>
      </section>
    );
  }

  return (
    <section className="stack">
      <h1>Delete your ProGuideGH account</h1>

      {signedOut ? (
        <>
          <Alert tone="info" title="Sign in to continue">
            For your safety we only delete an account after you prove it is yours.
            Sign in and return to this page.
          </Alert>
          <Link href="/login">Sign in</Link>
          <p className="muted">
            If you can no longer sign in — for example you have lost access to your
            email — contact <a href="mailto:privacy@proguidegh.com">privacy@proguidegh.com</a>{" "}
            from the address on the account and we will verify you another way.
          </p>
        </>
      ) : null}

      {loadError ? <Alert tone="error" title="Something went wrong">{loadError}</Alert> : null}

      {preview ? (
        <>
          {preview.removed?.length ? (
            <div>
              <h2>This will permanently remove</h2>
              <ul>
                {preview.removed.map((item) => (
                  <li key={item}>{item}</li>
                ))}
              </ul>
            </div>
          ) : null}

          {preview.retained?.length ? (
            <div>
              <h2>This will be kept</h2>
              <ul>
                {preview.retained.map((item) => (
                  <li key={item}>{item}</li>
                ))}
              </ul>
            </div>
          ) : null}

          {preview.can_delete === false ? (
            <Alert tone="info" title="Not yet">
              {preview.blockers?.map((b) => <p key={b.reason}>{b.message}</p>)}
            </Alert>
          ) : null}

          {deleteError ? (
            <Alert tone="error" title="Could not delete">{deleteError}</Alert>
          ) : null}

          {confirming ? (
            <>
              <Alert tone="error" title="This cannot be undone">
                You will be signed out immediately and will not be able to sign in
                again with this account.
              </Alert>
              <Button disabled={busy} onClick={() => void confirmDelete()} variant="primary">
                {busy ? "Deleting…" : "Yes, delete my account permanently"}
              </Button>
              <Button onClick={() => setConfirming(false)} variant="secondary">
                Cancel
              </Button>
            </>
          ) : (
            <Button
              disabled={preview.can_delete !== true}
              onClick={() => setConfirming(true)}
              variant="primary"
            >
              Delete my account
            </Button>
          )}
        </>
      ) : null}
    </section>
  );
}
