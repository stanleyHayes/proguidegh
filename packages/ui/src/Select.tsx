"use client";
import { Children, isValidElement, useEffect, useId, useRef, useState } from "react";
import type { ChangeEvent, ReactElement, ReactNode, SelectHTMLAttributes } from "react";
type NativeSelectProps = Omit<SelectHTMLAttributes<HTMLSelectElement>, "children" | "multiple" | "size">;
export interface SelectProps extends NativeSelectProps { label: string; error?: string; hint?: string; children: ReactNode; }
type OptionProps = { value?: string | number; disabled?: boolean; children?: ReactNode };
export function Select({ label, error, hint, id, children, value, defaultValue, onChange, name, disabled, required }: SelectProps) {
  const autoId = useId(); const fieldId = id ?? autoId; const listId = `${fieldId}-listbox`;
  const hintId = hint ? `${fieldId}-hint` : undefined; const errorId = error ? `${fieldId}-error` : undefined; const describedBy = [hintId, errorId].filter(Boolean).join(" ") || undefined;
  const [open, setOpen] = useState(false); const [internalValue, setInternalValue] = useState(String(defaultValue ?? "")); const rootRef = useRef<HTMLDivElement>(null); const selectedValue = String(value ?? internalValue);
  const options = Children.toArray(children).filter(isValidElement).map((child) => { const option = child as ReactElement<OptionProps>; return { value: String(option.props.value ?? ""), label: option.props.children, disabled: option.props.disabled }; });
  const selected = options.find((option) => option.value === selectedValue) ?? options[0];
  useEffect(() => { const close = (event: PointerEvent) => { if (!rootRef.current?.contains(event.target as Node)) setOpen(false); }; document.addEventListener("pointerdown", close); return () => document.removeEventListener("pointerdown", close); }, []);
  function choose(nextValue: string) { if (value === undefined) setInternalValue(nextValue); onChange?.({ target: { value: nextValue, name: name ?? "" }, currentTarget: { value: nextValue, name: name ?? "" } } as ChangeEvent<HTMLSelectElement>); setOpen(false); }
  return <div className={`gg-field${error ? " gg-field--invalid" : ""}`} ref={rootRef}>
    <label className="gg-field__label" id={`${fieldId}-label`}>{label}</label>{hint ? <p className="gg-field__hint" id={hintId}>{hint}</p> : null}
    <div className={`gg-select${open ? " is-open" : ""}`}><button type="button" className="gg-field__control gg-select__trigger" id={fieldId} role="combobox" aria-haspopup="listbox" aria-expanded={open} aria-controls={listId} aria-labelledby={`${fieldId}-label ${fieldId}`} aria-describedby={describedBy} aria-invalid={error ? true : undefined} disabled={disabled} onClick={() => setOpen((current) => !current)} onKeyDown={(event) => { if (event.key === "Escape") setOpen(false); if (event.key === "ArrowDown") { event.preventDefault(); setOpen(true); } }}><span>{selected?.label ?? "Select an option"}</span><svg aria-hidden="true" viewBox="0 0 20 20" fill="none"><path d="m5.5 7.5 4.5 4.5 4.5-4.5" /></svg></button>
      {open ? <div className="gg-select__menu" id={listId} role="listbox" aria-labelledby={`${fieldId}-label`}>{options.map((option) => <button key={option.value} type="button" role="option" aria-selected={option.value === selectedValue} disabled={option.disabled} onClick={() => choose(option.value)}><span>{option.label}</span>{option.value === selectedValue ? <svg aria-hidden="true" viewBox="0 0 20 20" fill="none"><path d="m4.5 10 3.5 3.5 7.5-8" /></svg> : null}</button>)}</div> : null}</div>
    <input type="hidden" name={name} value={selectedValue} required={required} />{error ? <p className="gg-field__error" id={errorId} role="alert">{error}</p> : null}
  </div>;
}
