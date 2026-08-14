"use client";

import { useId } from "react";
import type { TextareaHTMLAttributes } from "react";

export interface TextareaProps extends TextareaHTMLAttributes<HTMLTextAreaElement> {
  /** Visible label, always rendered and associated via htmlFor. */
  label: string;
  /** Error message; marks the control aria-invalid and links it via aria-describedby. */
  error?: string;
  /** Optional helper text shown above the control. */
  hint?: string;
}

export function Textarea({ label, error, hint, id, ...rest }: TextareaProps) {
  const autoId = useId();
  const fieldId = id ?? autoId;
  const hintId = hint ? `${fieldId}-hint` : undefined;
  const errorId = error ? `${fieldId}-error` : undefined;
  const describedBy = [hintId, errorId].filter(Boolean).join(" ") || undefined;

  return (
    <div className={`gg-field${error ? " gg-field--invalid" : ""}`}>
      <label className="gg-field__label" htmlFor={fieldId}>
        {label}
      </label>
      {hint ? (
        <p className="gg-field__hint" id={hintId}>
          {hint}
        </p>
      ) : null}
      <textarea
        className="gg-field__control"
        id={fieldId}
        aria-invalid={error ? true : undefined}
        aria-describedby={describedBy}
        {...rest}
      />
      {error ? (
        <p className="gg-field__error" id={errorId} role="alert">
          {error}
        </p>
      ) : null}
    </div>
  );
}
