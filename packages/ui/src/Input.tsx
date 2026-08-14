"use client";
import { useId, useState } from "react";
import type { InputHTMLAttributes, ReactNode } from "react";

export interface InputProps extends InputHTMLAttributes<HTMLInputElement> { label: string; error?: string; hint?: string; startIcon?: ReactNode; endIcon?: ReactNode; showPasswordToggle?: boolean; }

function iconFor(type: string, name: string, label: string) {
  const key = `${name} ${label}`.toLowerCase();
  if (type === "email" || key.includes("email")) return <><rect x="3" y="5" width="18" height="14" rx="2"/><path d="m4 7 8 6 8-6"/></>;
  if (type === "tel" || key.includes("phone")) return <path d="M8 3H5a2 2 0 0 0-2 2c0 8.8 7.2 16 16 16a2 2 0 0 0 2-2v-3l-4-1-1.2 2.4a14 14 0 0 1-9.2-9.2L9 7 8 3Z"/>;
  if (type === "password") return <><rect x="5" y="10" width="14" height="11" rx="2"/><path d="M8 10V7a4 4 0 0 1 8 0v3"/></>;
  if (type === "search" || key.includes("search")) return <><circle cx="10.5" cy="10.5" r="5.5"/><path d="m15 15 4 4"/></>;
  if (key.includes("location") || key.includes("meeting") || key.includes("address")) return <><path d="M12 21s6-5.2 6-11a6 6 0 1 0-12 0c0 5.8 6 11 6 11Z"/><circle cx="12" cy="10" r="2"/></>;
  if (key.includes("name") || key.includes("identifier")) return <><circle cx="12" cy="8" r="4"/><path d="M4.5 21a7.5 7.5 0 0 1 15 0"/></>;
  if (type === "number") return <><circle cx="12" cy="12" r="9"/><path d="M9 8h4.5a2.5 2.5 0 0 1 0 5H11a2.5 2.5 0 0 0 0 5h4M12 5v3M12 18v2"/></>;
  return <><path d="M5 19h14M7 16l9-9 2 2-9 9H7v-2Z"/></>;
}

function defaultPlaceholder(type: string, label: string) {
  if (type === "email") return "name@example.com";
  if (type === "tel") return "+233 24 000 0000";
  if (type === "password") return "Enter your password";
  if (type === "search") return "Search…";
  if (type === "number") return "Enter a number";
  const cleaned = label.replace(/\s*\(optional\)\s*/i, "").toLowerCase();
  return `Enter ${cleaned}`;
}

export function Input({ label, error, hint, id, startIcon, endIcon, showPasswordToggle = true, type = "text", name = "", placeholder, ...rest }: InputProps) {
  const autoId = useId(); const fieldId = id ?? autoId; const hintId = hint ? `${fieldId}-hint` : undefined; const errorId = error ? `${fieldId}-error` : undefined; const describedBy = [hintId, errorId].filter(Boolean).join(" ") || undefined; const [revealed, setRevealed] = useState(false); const password = type === "password";
  const leading = startIcon ?? <svg viewBox="0 0 24 24" fill="none">{iconFor(type, name, label)}</svg>;
  return <div className={`gg-field${error ? " gg-field--invalid" : ""}`}><label className="gg-field__label" htmlFor={fieldId}>{label}</label>{hint ? <p className="gg-field__hint" id={hintId}>{hint}</p> : null}<div className="gg-input"><span className="gg-input__icon" aria-hidden="true">{leading}</span><input className="gg-field__control gg-input__control" id={fieldId} name={name} type={password && revealed ? "text" : type} placeholder={placeholder ?? defaultPlaceholder(type, label)} aria-invalid={error ? true : undefined} aria-describedby={describedBy} {...rest}/>{password && showPasswordToggle ? <button className="gg-input__action" type="button" aria-label={revealed ? "Hide password" : "Show password"} aria-pressed={revealed} onClick={() => setRevealed((current) => !current)}><svg viewBox="0 0 24 24" fill="none">{revealed ? <><path d="M3 3l18 18M10.6 10.7a2 2 0 0 0 2.7 2.7M9.9 4.3A10.8 10.8 0 0 1 12 4c6.5 0 10 8 10 8a18 18 0 0 1-2.1 3.2M6.6 6.6C3.5 8.4 2 12 2 12s3.5 8 10 8a10.8 10.8 0 0 0 4.3-.9"/></> : <><path d="M2 12s3.5-8 10-8 10 8 10 8-3.5 8-10 8S2 12 2 12Z"/><circle cx="12" cy="12" r="3"/></>}</svg></button> : endIcon ? <span className="gg-input__end" aria-hidden="true">{endIcon}</span> : null}</div>{error ? <p className="gg-field__error" id={errorId} role="alert">{error}</p> : null}</div>;
}
