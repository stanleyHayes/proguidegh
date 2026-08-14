"use client";

import { useState } from "react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { Alert, Button, Input } from "@proguidegh/ui";
import { api, errorMessage } from "../lib/api";

export function ResetPasswordForm() {
  const searchParams = useSearchParams();
  const token = searchParams.get("token") ?? "";

  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [fieldError, setFieldError] = useState<string | undefined>();
  const [done, setDone] = useState(false);

  async function onSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError(null);
    if (password !== confirm) {
      setFieldError("Passwords do not match.");
      return;
    }
    setFieldError(undefined);
    setPending(true);
    try {
      await api("/auth/password/reset", {
        method: "POST",
        body: { token, new_password: password },
        skipRefreshRetry: true,
      });
      setDone(true);
    } catch (err) {
      setError(
        errorMessage(
          err,
          "Could not reach the server. The link may have expired — request a new one.",
        ),
      );
    } finally {
      setPending(false);
    }
  }

  if (!token) {
    return (
      <div className="stack">
        <h1>Reset your password</h1>
        <Alert tone="error" title="Invalid reset link">
          <p>
            This link is missing its reset token. Request a fresh password
            reset link and try again.
          </p>
        </Alert>
        <p>
          <Link className="gg-button gg-button--primary" href="/forgot-password">
            Request a new link
          </Link>
        </p>
      </div>
    );
  }

  if (done) {
    return (
      <div className="stack">
        <h1>Password updated</h1>
        <Alert tone="success" title="You can sign in again">
          <p>Your password has been changed successfully.</p>
        </Alert>
        <p>
          <Link className="gg-button gg-button--primary" href="/login">
            Sign in
          </Link>
        </p>
      </div>
    );
  }

  return (
    <div className="stack" aria-busy={pending}>
      <section aria-labelledby="reset-heading">
        <h1 id="reset-heading">Choose a new password</h1>
      </section>

      {error ? (
        <Alert tone="error" title="Reset failed">
          <p>{error}</p>
        </Alert>
      ) : null}

      <form className="stack" onSubmit={onSubmit}>
        <Input
          label="New password"
          name="new_password"
          type="password"
          autoComplete="new-password"
          hint="At least 8 characters."
          required
          minLength={8}
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          disabled={pending}
        />
        <Input
          label="Confirm new password"
          name="confirm"
          type="password"
          autoComplete="new-password"
          required
          minLength={8}
          value={confirm}
          onChange={(e) => setConfirm(e.target.value)}
          disabled={pending}
          error={fieldError}
        />
        <div>
          <Button type="submit" disabled={pending}>
            {pending ? "Updating…" : "Update password"}
          </Button>
        </div>
      </form>
    </div>
  );
}
