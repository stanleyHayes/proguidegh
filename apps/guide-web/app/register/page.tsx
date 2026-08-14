"use client";

import { useState } from "react";
import Link from "next/link";
import { Alert, Button, Input } from "@proguidegh/ui";
import { api, errorMessage } from "../lib/api";
import { AuthShell } from "../components/AuthShell";

export default function RegisterPage() {
  const [identifier, setIdentifier] = useState("");
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
      const body = { email: identifier, password, intent: "guide" };
      await api("/auth/register", {
        method: "POST",
        body,
        skipRefreshRetry: true,
      });
      setDone(true);
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

  if (done) {
    return (
      <AuthShell eyebrow="Welcome aboard" title="Your professional guide journey starts here."><div className="stack auth-form">
        <section aria-labelledby="registered-heading">
          <h1 id="registered-heading">Account created</h1>
        </section>
        <Alert tone="success" title="Welcome aboard">
          <p>
            Your guide account was created. Sign in, then submit your
            application to start the certification pipeline.
          </p>
        </Alert>
        <p>
          <Link className="gg-button gg-button--primary" href="/login">
            Continue to sign in
          </Link>
        </p>
      </div></AuthShell>
    );
  }

  return (
    <AuthShell eyebrow="Become a partner" title="Turn local knowledge into trusted work."><div className="stack auth-form" aria-busy={pending}>
      <section aria-labelledby="register-heading">
        <h1 id="register-heading">Register as a guide</h1>
        <p className="muted">
          Create your account with an email address. You will complete your
          professional application after signing in.
        </p>
      </section>

      {error ? (
        <Alert tone="error" title="Registration failed">
          <p>{error}</p>
        </Alert>
      ) : null}

      <form className="stack" onSubmit={onSubmit}>
        <Input
          label="Email address"
          name="identifier"
          type="email"
          autoComplete="email"
          hint="We use this for certification updates and account recovery."
          required
          value={identifier}
          onChange={(e) => setIdentifier(e.target.value)}
          disabled={pending}
        />
        <Input
          label="Password"
          name="password"
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
          label="Confirm password"
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
            {pending ? "Creating account…" : "Create account"}
          </Button>
        </div>
      </form>

      <p className="muted">
        Already registered? <Link href="/login">Sign in</Link>
      </p>
    </div></AuthShell>
  );
}
