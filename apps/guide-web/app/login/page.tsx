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
  const [mfaCode, setMfaCode] = useState("");

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

  async function submitMFA(event: React.FormEvent<HTMLFormElement>) { event.preventDefault(); setPending(true); setError(null); try { await api("/auth/login/mfa", { method: "POST", body: { challenge: mfaChallenge, code: mfaCode }, skipRefreshRetry: true }); router.push("/guide") } catch (err) { setError(errorMessage(err, "That verification code was not accepted.")) } finally { setPending(false) } }

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
        <form className="stack" onSubmit={submitMFA}><Alert tone="info" title="Verification required"><p>Enter the six-digit code from your authenticator app or one unused recovery code.</p></Alert><Input label="Verification code" name="code" inputMode="numeric" autoComplete="one-time-code" required value={mfaCode} onChange={(event) => setMfaCode(event.target.value)} disabled={pending} placeholder="000 000" /><Button type="submit" disabled={pending}>{pending ? "Verifying…" : "Verify and continue"}</Button><button className="auth-text-action" type="button" onClick={() => { setMfaChallenge(null); setMfaCode("") }}>Use another account</button></form>
      ) : (
        <form className="stack" onSubmit={onSubmit}>
          <Input
            label="Email address"
            name="identifier"
            type="email"
            autoComplete="email"
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
