"use client";

import { useState } from "react";
import Link from "next/link";
import { Alert, Button, Input } from "@proguidegh/ui";
import { api, errorMessage } from "../lib/api";
import { AuthShell } from "../components/AuthShell";

export default function ForgotPasswordPage() {
  const [email, setEmail] = useState("");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [sent, setSent] = useState(false);

  async function onSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setPending(true);
    setError(null);
    try {
      await api("/auth/password/forgot", {
        method: "POST",
        body: { email },
        skipRefreshRetry: true,
      });
      setSent(true);
    } catch (err) {
      setError(
        errorMessage(
          err,
          "Could not reach the server. Check your connection and try again.",
        ),
      );
    } finally {
      setPending(false);
    }
  }

  return (
    <AuthShell eyebrow="Account recovery" title="Get securely back to your plans."><div className="stack auth-form" aria-busy={pending}>
      <section aria-labelledby="forgot-heading">
        <h1 id="forgot-heading">Reset your password</h1>
        <p className="muted">
          Enter your account email and we will send you a reset link.
        </p>
      </section>

      {error ? (
        <Alert tone="error" title="Request failed">
          <p>{error}</p>
        </Alert>
      ) : null}

      {sent ? (
        <Alert tone="success" title="Check your inbox">
          <p>
            If an account exists for {email}, a password reset link is on its
            way. The link expires shortly — request a new one if it lapses.
          </p>
        </Alert>
      ) : (
        <form className="stack" onSubmit={onSubmit}>
          <Input
            label="Email address"
            name="email"
            type="email"
            autoComplete="email"
            required
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            disabled={pending}
          />
          <div>
            <Button type="submit" disabled={pending}>
              {pending ? "Sending…" : "Send reset link"}
            </Button>
          </div>
        </form>
      )}

      <p className="muted">
        Remembered it? <Link href="/login">Back to sign in</Link>
      </p>
    </div></AuthShell>
  );
}
