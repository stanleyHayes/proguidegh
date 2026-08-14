"use client";

import { useEffect, useState } from "react";
import { useSearchParams } from "next/navigation";
import { Alert, Button, Card, Input } from "@proguidegh/ui";
import { api, errorMessage } from "../lib/api";

const tabs = ["profile", "security", "preferences", "notifications"] as const;
type Tab = (typeof tabs)[number];
type Choices = { compact: boolean; reducedData: boolean; safety: boolean; finance: boolean; certification: boolean; digest: boolean };
type MFAEnrollment = { secret: string; otpauth_uri: string };
const defaults: Choices = { compact: false, reducedData: false, safety: true, finance: true, certification: true, digest: false };

export function AccountWorkspace() {
  const params = useSearchParams();
  const requested = params.get("tab");
  const [tab, setTab] = useState<Tab>(tabs.includes(requested as Tab) ? requested as Tab : "profile");
  const [choices, setChoices] = useState(defaults);
  const [email, setEmail] = useState("admin@proguidegh.com");
  const [notice, setNotice] = useState<string | null>(null);
  const [mfa, setMfa] = useState<MFAEnrollment | null>(null); const [mfaCode, setMfaCode] = useState(""); const [backupCodes, setBackupCodes] = useState<string[]>([]); const [mfaPending, setMfaPending] = useState(false);

  useEffect(() => {
    try { const stored = localStorage.getItem("proguidegh-admin-preferences"); if (stored) setChoices({ ...defaults, ...JSON.parse(stored) as Partial<Choices> }); } catch { /* keep defaults */ }
  }, []);

  function saveChoices(next = choices) { setChoices(next); localStorage.setItem("proguidegh-admin-preferences", JSON.stringify(next)); setNotice("Workspace preferences saved on this device."); }
  async function requestReset() {
    setNotice(null);
    try { await api("/auth/password/forgot", { method: "POST", body: { email }, skipRefreshRetry: true }); setNotice("If that account exists, password reset instructions have been sent."); }
    catch (err) { setNotice(errorMessage(err, "Password reset could not be requested.")); }
  }
  async function beginMFA() { setNotice(null); setMfaPending(true); try { setMfa(await api<MFAEnrollment>("/me/mfa/enroll", { method: "POST" })) } catch (err) { setNotice(errorMessage(err, "MFA enrollment could not be started.")) } finally { setMfaPending(false) } }
  async function verifyMFA(event: React.FormEvent<HTMLFormElement>) { event.preventDefault(); setNotice(null); setMfaPending(true); try { const result = await api<{ backup_codes: string[] }>("/me/mfa/verify", { method: "POST", body: { code: mfaCode } }); setBackupCodes(result.backup_codes); setMfa(null); setMfaCode(""); setNotice("Multi-factor authentication is now enabled.") } catch (err) { setNotice(errorMessage(err, "That authenticator code could not be verified.")) } finally { setMfaPending(false) } }

  return <div className="stack account-workspace">
    <section><p className="eyebrow">Personal workspace</p><h1>Account settings</h1><p className="muted">Profile, security, preferences and notification controls for this administrator.</p></section>
    <nav className="account-tabs" aria-label="Account settings">{tabs.map((item) => <button type="button" key={item} aria-current={tab === item ? "page" : undefined} onClick={() => setTab(item)}>{item}</button>)}</nav>
    {notice && <Alert tone="info">{notice}</Alert>}
    {tab === "profile" && <div className="grid grid--cols-2"><Card title="Administrator profile"><div className="account-avatar-large">SA</div><dl className="account-details"><div><dt>Name</dt><dd>System administrator</dd></div><div><dt>Email</dt><dd>admin@proguidegh.com</dd></div><div><dt>Role</dt><dd>Super administrator</dd></div><div><dt>Region</dt><dd>Africa/Accra</dd></div></dl></Card><Card title="Update profile"><Alert tone="info">Profile changes require the audited administrator-profile endpoint. Display data remains read-only until that security-sensitive API is available.</Alert><Input label="Display name" value="System administrator" disabled /><Input label="Work email" value="admin@proguidegh.com" disabled /></Card></div>}
    {tab === "security" && <div className="grid grid--cols-2"><Card title="Password"><p>Request a secure password-reset link. Completing the reset revokes active sessions.</p><div className="account-form"><Input label="Account email" type="email" value={email} onChange={(event) => setEmail(event.target.value)} /><Button onClick={() => void requestReset()}>Send reset instructions</Button></div></Card><Card title="Multi-factor authentication"><p>Privileged accounts require MFA. Connect an authenticator before handling sensitive operations.</p><p><span className="security-status"><i /> Required for this role</span></p>{backupCodes.length ? <div className="mfa-backup"><Alert tone="success" title="Save your recovery codes"><p>Each code works once. Store them somewhere private; they will not be shown again.</p></Alert><code>{backupCodes.join("\n")}</code></div> : mfa ? <form className="account-form" onSubmit={verifyMFA}><div className="mfa-secret"><small>Authenticator setup key</small><code>{mfa.secret}</code><button type="button" onClick={() => void navigator.clipboard.writeText(mfa.otpauth_uri)}>Copy setup URI</button></div><Input label="Six-digit code" name="mfa_code" inputMode="numeric" autoComplete="one-time-code" placeholder="000 000" required value={mfaCode} onChange={(event) => setMfaCode(event.target.value)} /><Button type="submit" disabled={mfaPending}>{mfaPending ? "Verifying…" : "Verify and enable MFA"}</Button></form> : <Button variant="secondary" disabled={mfaPending} onClick={() => void beginMFA()}>{mfaPending ? "Starting…" : "Set up authenticator"}</Button>}</Card></div>}
    {tab === "preferences" && <Card title="Workspace preferences"><div className="setting-list"><Toggle label="Compact data density" description="Fit more operational rows on larger screens." checked={choices.compact} onChange={(value) => saveChoices({ ...choices, compact: value })} /><Toggle label="Reduced data mode" description="Limit non-essential refreshes on constrained connections." checked={choices.reducedData} onChange={(value) => saveChoices({ ...choices, reducedData: value })} /></div></Card>}
    {tab === "notifications" && <Card title="Notification preferences"><div className="setting-list"><Toggle label="Safety and SOS alerts" description="Critical operational alerts; recommended and visually prioritized." checked={choices.safety} onChange={(value) => saveChoices({ ...choices, safety: value })} /><Toggle label="Finance and payout events" description="Settlement, reconciliation and payout exceptions." checked={choices.finance} onChange={(value) => saveChoices({ ...choices, finance: value })} /><Toggle label="Certification queue" description="New applications and expiring guide credentials." checked={choices.certification} onChange={(value) => saveChoices({ ...choices, certification: value })} /><Toggle label="Daily operations digest" description="One summarized update at the end of the operating day." checked={choices.digest} onChange={(value) => saveChoices({ ...choices, digest: value })} /></div></Card>}
  </div>;
}

function Toggle({ label, description, checked, onChange }: { label: string; description: string; checked: boolean; onChange: (value: boolean) => void }) {
  return <label className="setting-toggle"><span><strong>{label}</strong><small>{description}</small></span><input type="checkbox" checked={checked} onChange={(event) => onChange(event.target.checked)} /><i aria-hidden="true" /></label>;
}
