/**
 * Catalog/search types and tolerant parsers for the tourist app (Phase M,
 * M-09). Mirrors the web app's tolerance: lists may be bare or wrapped,
 * keys snake_case, amounts in major units GHS.
 */

export interface NamedOption {
  id: string;
  code?: string;
  name: string;
}

export interface GuideSummary {
  userId: string;
  publicName: string;
  ratingAvg: number | null;
  ratingCount: number;
  eliteStatus: boolean;
  regionName: string | null;
  languages: string[];
  specialties: string[];
  online: boolean;
}

export interface GuideDetail extends GuideSummary {
  bio: string | null;
}

export interface TourPackage {
  id: string;
  code: string;
  name: string;
  durationMinutes: number;
  basePrice: number;
  currency: string;
}

function asRecord(value: unknown): Record<string, unknown> | null {
  return value !== null && typeof value === "object"
    ? (value as Record<string, unknown>)
    : null;
}

function asList(data: unknown, ...keys: string[]): unknown[] {
  if (Array.isArray(data)) return data;
  const rec = asRecord(data);
  if (rec) {
    for (const key of keys) {
      if (Array.isArray(rec[key])) return rec[key] as unknown[];
    }
  }
  return [];
}

function str(value: unknown): string | null {
  return typeof value === "string" && value !== "" ? value : null;
}

function num(value: unknown): number | null {
  if (typeof value === "number") return value;
  if (typeof value === "string") {
    const parsed = Number(value);
    return Number.isFinite(parsed) ? parsed : null;
  }
  return null;
}

export function parseNamedOptions(data: unknown, ...keys: string[]): NamedOption[] {
  return asList(data, ...keys, "items", "results")
    .map((entry) => {
      const rec = asRecord(entry);
      if (!rec) return null;
      const id = str(rec.id);
      const name = str(rec.name) ?? str(rec.code);
      const code = str(rec.code);
      if (!id || !name) return null;
      const option: NamedOption = { id, name };
      if (code) option.code = code;
      return option;
    })
    .filter((v): v is NamedOption => v !== null);
}

export function parseGuides(data: unknown): GuideSummary[] {
  return asList(data, "guides", "items", "results")
    .map((entry) => {
      const rec = asRecord(entry);
      if (!rec) return null;
      const userId = str(rec.user_id) ?? str(rec.id);
      const publicName = str(rec.public_name) ?? "Guide";
      if (!userId) return null;
      return {
        userId,
        publicName,
        ratingAvg: num(rec.rating_avg),
        ratingCount: num(rec.rating_count) ?? 0,
        eliteStatus: rec.elite_status === true,
        regionName: str(rec.region_name) ?? str(rec.region),
        languages: Array.isArray(rec.languages)
          ? (rec.languages as unknown[]).map(String)
          : [],
        specialties: Array.isArray(rec.specialties)
          ? (rec.specialties as unknown[]).map(String)
          : [],
        online: rec.online === true,
      } satisfies GuideSummary;
    })
    .filter((v): v is GuideSummary => v !== null);
}

export function parseGuideDetail(data: unknown): GuideDetail | null {
  const rec = asRecord(asRecord(data)?.guide ?? data);
  if (!rec) return null;
  const guides = parseGuides([rec]);
  const base = guides[0];
  if (!base) return null;
  return { ...base, bio: str(rec.bio) };
}

export function parsePackages(data: unknown): TourPackage[] {
  return asList(data, "packages", "tour_packages", "items")
    .map((entry) => {
      const rec = asRecord(entry);
      if (!rec) return null;
      const id = str(rec.id);
      const name = str(rec.name);
      const price = num(rec.base_price);
      if (!id || !name || price === null) return null;
      return {
        id,
        code: str(rec.code) ?? "",
        name,
        durationMinutes: num(rec.duration_minutes) ?? 0,
        basePrice: price,
        currency: str(rec.currency) ?? "GHS",
      } satisfies TourPackage;
    })
    .filter((v): v is TourPackage => v !== null);
}

export function formatPrice(amount: number, currency = "GHS"): string {
  const symbol = currency === "GHS" ? "GH₵" : `${currency} `;
  return `${symbol}${amount.toFixed(2)}`;
}

export function formatDuration(minutes: number): string {
  const h = Math.floor(minutes / 60);
  const m = minutes % 60;
  if (h === 0) return `${m} min`;
  return m === 0 ? `${h} h` : `${h} h ${m} min`;
}

export function formatRating(avg: number | null, count: number): string {
  if (avg === null || count === 0) return "New guide";
  return `★ ${avg.toFixed(1)} (${count} tour${count === 1 ? "" : "s"})`;
}
