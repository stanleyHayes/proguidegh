"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { Alert, Button, Input } from "@proguidegh/ui";
import { api, errorMessage } from "../lib/api";
import { AdminAuthShell } from "../components/AdminAuthShell";

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
      router.push("/admin/users");
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
    <AdminAuthShell><div className="admin-auth__form" aria-busy={pending}>
      <header aria-labelledby="login-heading">
        <span className="admin-auth__lock" aria-hidden="true"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round"><rect x="5" y="10" width="14" height="11" rx="2"/><path d="M8 10V7a4 4 0 0 1 8 0v3M12 14v3"/></svg></span>
        <p>Secure administrator access</p>
        <h1 id="login-heading">Enter the command center.</h1>
        <span>Use your approved operations, finance or administrator account.</span>
      </header>

      {error ? (
        <Alert tone="error" title="Sign-in failed">
          <p>{error}</p>
        </Alert>
      ) : null}

      {mfaChallenge !== null ? (
        <Alert tone="info" title="Multi-factor authentication required">
          <p>
            Admin accounts require a verification code to finish signing in.
            Code entry for MFA challenges is enabled in a later phase — contact
            a Super Admin if you cannot sign in.
          </p>
          {mfaChallenge ? (
            <p className="muted">Challenge reference: {mfaChallenge}</p>
          ) : null}
        </Alert>
      ) : (
        <form onSubmit={onSubmit}>
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
          <div className="admin-auth__submit">
            <Button type="submit" disabled={pending}>
              {pending ? "Signing in…" : "Sign in"}
            </Button>
            <small><i aria-hidden="true" /> Encrypted session</small>
          </div>
        </form>
      )}
    </div></AdminAuthShell>
  );
}
