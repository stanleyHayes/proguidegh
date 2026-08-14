"use client";

import { useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { Alert, Button, Input } from "@proguidegh/ui";
import { api, errorMessage } from "../lib/api";
import { AuthShell } from "../components/AuthShell";

interface LoginResponse {
  mfa_required?: boolean;
  challenge?: string;
}

export default function LoginPage() {
  const router = useRouter();
  const [identifier, setIdentifier] = useState("");
  const [password, setPassword] = useState("");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [mfaChallenge, setMfaChallenge] = useState<string | null>(null);

  async function onSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setPending(true);
    setError(null);
    try {
      const credentials = identifier.includes("@")
        ? { email: identifier, password }
        : { phone: identifier, password };
      const res = await api<LoginResponse | undefined>("/auth/login", {
        method: "POST",
        body: credentials,
        skipRefreshRetry: true,
      });
      if (res?.mfa_required) {
        setMfaChallenge(res.challenge ?? "");
        return;
      }
      router.push("/guide");
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
    <AuthShell eyebrow="Partner access" title="Return to your guide workspace."><div className="stack auth-form" aria-busy={pending}>
      <section aria-labelledby="login-heading">
        <h1 id="login-heading">Guide sign in</h1>
        <p className="muted">
          Access your dashboard, verification pipeline and job offers.
        </p>
      </section>

      {error ? (
        <Alert tone="error" title="Sign-in failed">
          <p>{error}</p>
        </Alert>
      ) : null}

      {mfaChallenge !== null ? (
        <Alert tone="info" title="Multi-factor authentication required">
          <p>
            This account requires a verification code to finish signing in.
            Code entry for MFA challenges is enabled in a later phase — please
            contact support if you cannot sign in.
          </p>
          {mfaChallenge ? (
            <p className="muted">Challenge reference: {mfaChallenge}</p>
          ) : null}
        </Alert>
      ) : (
        <form className="stack" onSubmit={onSubmit}>
          <Input
            label="Email or phone number"
            name="identifier"
            type="text"
            autoComplete="username"
            required
            value={identifier}
            onChange={(e) => setIdentifier(e.target.value)}
            disabled={pending}
          />
          <Input
            label="Password"
            name="password"
            type="password"
            autoComplete="current-password"
            required
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            disabled={pending}
          />
          <div>
            <Button type="submit" disabled={pending}>
              {pending ? "Signing in…" : "Sign in"}
            </Button>
          </div>
        </form>
      )}

      <p className="muted">
        <Link href="/forgot-password">Forgot your password?</Link>{" · "}New to guiding? <Link href="/register">Register as a guide</Link>
      </p>
    </div></AuthShell>
  );
}
