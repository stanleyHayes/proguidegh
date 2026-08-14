"use client";

import { useId } from "react";
import type { FieldsetHTMLAttributes, ReactNode } from "react";

export interface FieldsetProps extends FieldsetHTMLAttributes<HTMLFieldSetElement> {
  /** Legend text for the group. */
  legend: string;
  /** Error message for the group, linked via aria-describedby. */
  error?: string;
  /** Optional helper text shown under the legend. */
  hint?: string;
  children: ReactNode;
}

export function Fieldset({ legend, error, hint, children, ...rest }: FieldsetProps) {
  const autoId = useId();
  const hintId = hint ? `${autoId}-hint` : undefined;
  const errorId = error ? `${autoId}-error` : undefined;
  const describedBy = [hintId, errorId].filter(Boolean).join(" ") || undefined;

  return (
    <fieldset
      className={`gg-fieldset${error ? " gg-fieldset--invalid" : ""}`}
      aria-describedby={describedBy}
      {...rest}
    >
      <legend className="gg-fieldset__legend">{legend}</legend>
      {hint ? (
        <p className="gg-field__hint" id={hintId}>
          {hint}
        </p>
      ) : null}
      {children}
      {error ? (
        <p className="gg-field__error" id={errorId} role="alert">
          {error}
        </p>
      ) : null}
    </fieldset>
  );
}
