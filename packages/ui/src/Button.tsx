import type { ButtonHTMLAttributes, ReactNode } from "react";

export type ButtonVariant = "primary" | "secondary";

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant;
  children: ReactNode;
}

export function Button({ variant = "primary", children, ...rest }: ButtonProps) {
  return (
    <button className={`gg-button gg-button--${variant}`} {...rest}>
      {children}
    </button>
  );
}
