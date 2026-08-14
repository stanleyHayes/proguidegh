"use client";

import { useState } from "react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { Alert, Button, Input } from "@proguidegh/ui";
import { AuthShell } from "../components/AuthShell";
import { api, errorMessage } from "../lib/api";

export function ResetPasswordForm() {
  const token = useSearchParams().get("token") ?? ""; const [password, setPassword] = useState(""); const [confirm, setConfirm] = useState(""); const [pending, setPending] = useState(false); const [error, setError] = useState<string | null>(null); const [fieldError, setFieldError] = useState<string>(); const [done, setDone] = useState(false);
  async function onSubmit(event: React.FormEvent<HTMLFormElement>) { event.preventDefault(); setError(null); if (password !== confirm) { setFieldError("Passwords do not match."); return; } setFieldError(undefined); setPending(true); try { await api("/auth/password/reset", { method: "POST", body: { token, new_password: password }, skipRefreshRetry: true }); setDone(true); } catch (err) { setError(errorMessage(err, "The link may have expired — request a new one.")); } finally { setPending(false); } }
  if (!token) return <AuthShell eyebrow="Invalid link" title="Let us get you a fresh reset link."><div className="stack auth-form"><section><h1>Reset your password</h1></section><Alert tone="error" title="Invalid reset link"><p>This link is missing its reset token.</p></Alert><Link className="gg-button gg-button--primary" href="/forgot-password">Request a new link</Link></div></AuthShell>;
  if (done) return <AuthShell eyebrow="Password updated" title="Your guide account is secure again."><div className="stack auth-form"><section><h1>Password updated</h1></section><Alert tone="success"><p>You can sign in with your new password.</p></Alert><Link className="gg-button gg-button--primary" href="/login">Sign in</Link></div></AuthShell>;
  return <AuthShell eyebrow="Secure your account" title="Choose a password only you know."><div className="stack auth-form" aria-busy={pending}><section><h1>Choose a new password</h1></section>{error && <Alert tone="error"><p>{error}</p></Alert>}<form className="stack" onSubmit={onSubmit}><Input label="New password" name="new_password" type="password" autoComplete="new-password" hint="At least 8 characters." required minLength={8} value={password} onChange={(event) => setPassword(event.target.value)} disabled={pending} /><Input label="Confirm new password" name="confirm" type="password" autoComplete="new-password" required minLength={8} value={confirm} onChange={(event) => setConfirm(event.target.value)} disabled={pending} error={fieldError} /><Button type="submit" disabled={pending}>{pending ? "Updating…" : "Update password"}</Button></form></div></AuthShell>;
}
