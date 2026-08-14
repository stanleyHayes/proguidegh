import type { ReactNode } from "react";

export type AlertTone = "error" | "success" | "info";

export interface AlertProps {
  tone?: AlertTone;
  title?: string;
  children: ReactNode;
}

/**
 * Inline banner for form/page feedback. Errors use role="alert" (assertive),
 * success/info use role="status" (polite).
 */
export function Alert({ tone = "info", title, children }: AlertProps) {
  return (
    <div
      className={`gg-alert gg-alert--${tone}`}
      role={tone === "error" ? "alert" : "status"}
    >
      {title ? <p className="gg-alert__title">{title}</p> : null}
      <div className="gg-alert__body">{children}</div>
    </div>
  );
}
