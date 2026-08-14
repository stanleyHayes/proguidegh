"use client";

import { useState } from "react";
import Link from "next/link";
import { Alert, Button, Input } from "@proguidegh/ui";
import { AuthShell } from "../components/AuthShell";
import { api, errorMessage } from "../lib/api";

export default function ForgotPasswordPage() {
  const [email, setEmail] = useState(""); const [pending, setPending] = useState(false); const [error, setError] = useState<string | null>(null); const [sent, setSent] = useState(false);
  async function onSubmit(event: React.FormEvent<HTMLFormElement>) { event.preventDefault(); setPending(true); setError(null); try { await api("/auth/password/forgot", { method: "POST", body: { email }, skipRefreshRetry: true }); setSent(true); } catch (err) { setError(errorMessage(err, "Could not reach the server. Check your connection and try again.")); } finally { setPending(false); } }
  return <AuthShell eyebrow="Account recovery" title="Get securely back to your guide workspace."><div className="stack auth-form" aria-busy={pending}><section><h1>Reset your password</h1><p className="muted">Enter your account email and we will send you a secure reset link.</p></section>{error && <Alert tone="error" title="Request failed"><p>{error}</p></Alert>}{sent ? <Alert tone="success" title="Check your inbox"><p>If an account exists for {email}, a reset link is on its way.</p></Alert> : <form className="stack" onSubmit={onSubmit}><Input label="Email address" name="email" type="email" autoComplete="email" required value={email} onChange={(event) => setEmail(event.target.value)} disabled={pending} /><Button type="submit" disabled={pending}>{pending ? "Sending…" : "Send reset link"}</Button></form>}<p className="muted">Remembered it? <Link href="/login">Back to sign in</Link></p></div></AuthShell>;
}
