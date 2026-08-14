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
      const body = identifier.includes("@")
        ? { email: identifier, password, intent: "tourist" }
        : { phone: identifier, password, intent: "tourist" };
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
      <AuthShell eyebrow="You are ready" title="A better way to discover Ghana."><div className="stack auth-form">
        <section aria-labelledby="registered-heading">
          <h1 id="registered-heading">Account created</h1>
        </section>
        <Alert tone="success" title="Welcome to ProGuideGH">
          <p>
            Your account was created. If verification is required, we sent
            instructions to your email or phone.
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
    <AuthShell eyebrow="Start exploring" title="Book the person who knows the place."><div className="stack auth-form" aria-busy={pending}>
      <section aria-labelledby="register-heading">
        <h1 id="register-heading">Create your account</h1>
        <p className="muted">
          Sign up with your email address or phone number.
        </p>
      </section>

      {error ? (
        <Alert tone="error" title="Registration failed">
          <p>{error}</p>
        </Alert>
      ) : null}

      <form className="stack" onSubmit={onSubmit}>
        <Input
          label="Email or phone number"
          name="identifier"
          type="text"
          autoComplete="username"
          hint="Use an email address or a phone number with country code, e.g. +233…"
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
        Already have an account? <Link href="/login">Sign in</Link>
      </p>
    </div></AuthShell>
  );
}
