/**
 * Shared catalog types and helpers for search & booking.
 *
 * API shapes are assumed per spec §13.2 while the backend is built
 * concurrently — parsers tolerate snake_case, wrapped or bare lists, and
 * string-or-object entries.
 */

/** Assumed shape of GET /tour-packages entries. */
export interface TourPackage {
  id: string;
  code?: string;
  name?: string;
  duration_minutes?: number;
  base_price?: number;
  currency?: string;
}

/** A named reference option (region, specialty). */
export interface NamedOption {
  id: string;
  name: string;
}

/** Guide languages offered at signup; mirrors the profile page options. */
export const LANGUAGE_OPTIONS = [
  { value: "en", label: "English" },
  { value: "tw", label: "Twi" },
  { value: "ee", label: "Ewe" },
  { value: "gaa", label: "Ga" },
  { value: "dag", label: "Dagbani" },
  { value: "ha", label: "Hausa" },
  { value: "fr", label: "French" },
];

export function packageName(pkg?: TourPackage): string {
  return pkg?.name ?? pkg?.code ?? "Tour package";
}

/** Parse a list that may be bare or wrapped in a known key. */
export function parseList(data: unknown, keys: string[]): unknown[] {
  if (Array.isArray(data)) return data;
  if (data !== null && typeof data === "object") {
    for (const key of keys) {
      const list = (data as Record<string, unknown>)[key];
      if (Array.isArray(list)) return list;
    }
  }
  return [];
}

export function parsePackages(data: unknown): TourPackage[] {
  return parseList(data, ["tour_packages", "packages", "items"]).filter(
    (entry): entry is TourPackage =>
      entry !== null && typeof entry === "object" && "id" in entry,
  );
}

/** Parse regions/specialties; entries may be {id, name} objects or plain strings. */
export function parseNamedOptions(data: unknown, keys: string[]): NamedOption[] {
  const options: NamedOption[] = [];
  for (const entry of parseList(data, keys)) {
    if (typeof entry === "string" && entry) {
      options.push({ id: entry, name: entry });
    } else if (entry !== null && typeof entry === "object") {
      const record = entry as Record<string, unknown>;
      const id = record.id ?? record.code ?? record.name;
      const name = record.name ?? record.label ?? record.id;
      if (typeof id === "string" && typeof name === "string") {
        options.push({ id, name });
      }
    }
  }
  return options;
}

export function formatDuration(minutes?: number): string {
  if (!minutes || minutes <= 0) return "Duration on request";
  const hours = Math.floor(minutes / 60);
  const rest = minutes % 60;
  if (hours === 0) return `${rest} min`;
  return rest === 0 ? `${hours} h` : `${hours} h ${rest} min`;
}

/** Prices are assumed to be major units (e.g. 250.00 GHS). */
export function formatPrice(price?: number, currency?: string): string {
  if (price === undefined || price === null) return "Price on request";
  const code = (currency ?? "GHS").toUpperCase();
  const symbol = code === "GHS" ? "GH₵" : `${code} `;
  return `${symbol}${price.toLocaleString(undefined, {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  })}`;
}
