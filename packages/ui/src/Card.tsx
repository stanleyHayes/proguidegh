import type { ReactNode } from "react";

export interface CardProps {
  title?: string;
  children: ReactNode;
}

export function Card({ title, children }: CardProps) {
  return (
    <section className="gg-card">
      {title ? <h2 className="gg-card__title">{title}</h2> : null}
      <div className="gg-card__body">{children}</div>
    </section>
  );
}
