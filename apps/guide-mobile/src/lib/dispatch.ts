/**
 * Dispatch, presence and tour types for the guide app (Phase M, M-13–M-16).
 *
 * Mirrors the API contracts in docs/api/openapi.yaml. Offer expiry is
 * server-authoritative (§10.3 step 4: "DB expires_at is authoritative") — the
 * countdown here is presentation only and never gates the accept call. Let the
 * server reject an expired offer with 410; do not pre-empt it locally, because
 * a device clock that is slow would otherwise silently drop valid offers.
 */

export interface Offer {
  id: string;
  bookingId: string;
  reference: string;
  packageName: string | null;
  startsAt: string | null;
  endsAt: string | null;
  meetingPoint: string | null;
  numGuests: number | null;
  amount: number | null;
  currency: string;
  /** Server-side expiry instant; display only. */
  expiresAt: string | null;
}

export type TourStatus =
  | "CONFIRMED"
  | "GUIDE_ASSIGNED"
  | "GUIDE_EN_ROUTE"
  | "GUIDE_ARRIVED"
  | "IN_PROGRESS"
  | "COMPLETED"
  | "CANCELLED";

export interface GuideBooking {
  id: string;
  reference: string;
  status: TourStatus;
  packageName: string | null;
  startsAt: string | null;
  endsAt: string | null;
  meetingPoint: string | null;
  numGuests: number | null;
  amount: number | null;
  currency: string;
  touristName: string | null;
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

const TOUR_STATUSES: ReadonlySet<string> = new Set<TourStatus>([
  "CONFIRMED",
  "GUIDE_ASSIGNED",
  "GUIDE_EN_ROUTE",
  "GUIDE_ARRIVED",
  "IN_PROGRESS",
  "COMPLETED",
  "CANCELLED",
]);

function tourStatus(value: unknown): TourStatus {
  const raw = str(value)?.toUpperCase() ?? "";
  return TOUR_STATUSES.has(raw) ? (raw as TourStatus) : "GUIDE_ASSIGNED";
}

export function parseOffers(data: unknown): Offer[] {
  return asList(data, "offers", "items", "results")
    .map((entry) => {
      const rec = asRecord(entry);
      if (!rec) return null;
      const id = str(rec.id) ?? str(rec.offer_id);
      if (!id) return null;
      const booking = asRecord(rec.booking);
      const bookingId = str(rec.booking_id) ?? str(booking?.id);
      if (!bookingId) return null;
      return {
        id,
        bookingId,
        reference: str(booking?.reference) ?? str(rec.reference) ?? bookingId.slice(0, 8),
        packageName: str(booking?.package_name) ?? str(rec.package_name),
        startsAt: str(booking?.starts_at) ?? str(rec.starts_at),
        endsAt: str(booking?.ends_at) ?? str(rec.ends_at),
        meetingPoint: str(booking?.meeting_point) ?? str(rec.meeting_point),
        numGuests: num(booking?.num_guests) ?? num(rec.num_guests),
        amount: num(booking?.amount) ?? num(rec.amount),
        currency: str(booking?.currency) ?? str(rec.currency) ?? "GHS",
        expiresAt: str(rec.expires_at),
      } satisfies Offer;
    })
    .filter((v): v is Offer => v !== null);
}

export function parseGuideBookings(data: unknown): GuideBooking[] {
  return asList(data, "bookings", "items", "results")
    .map((entry) => {
      const rec = asRecord(entry);
      if (!rec) return null;
      const id = str(rec.id);
      if (!id) return null;
      return {
        id,
        reference: str(rec.reference) ?? id.slice(0, 8),
        status: tourStatus(rec.status),
        packageName: str(rec.package_name),
        startsAt: str(rec.starts_at),
        endsAt: str(rec.ends_at),
        meetingPoint: str(rec.meeting_point),
        numGuests: num(rec.num_guests),
        amount: num(rec.amount),
        currency: str(rec.currency) ?? "GHS",
        touristName: str(rec.tourist_name),
      } satisfies GuideBooking;
    })
    .filter((v): v is GuideBooking => v !== null);
}

/** The §8.2 transition a guide may trigger next, if any. */
export function nextTransition(
  status: TourStatus,
): { path: string; label: string } | null {
  switch (status) {
    case "CONFIRMED":
    case "GUIDE_ASSIGNED":
      return { path: "en-route", label: "Start travelling to meeting point" };
    case "GUIDE_EN_ROUTE":
      return { path: "arrived", label: "I have arrived" };
    case "GUIDE_ARRIVED":
      return { path: "start", label: "Start tour" };
    case "IN_PROGRESS":
      return { path: "complete", label: "Complete tour" };
    default:
      return null;
  }
}

/** Location streaming is only permitted inside this window (§11.1). */
export function isLocationWindow(status: TourStatus): boolean {
  return (
    status === "GUIDE_EN_ROUTE" ||
    status === "GUIDE_ARRIVED" ||
    status === "IN_PROGRESS"
  );
}

export function tourStatusLabel(status: TourStatus): string {
  switch (status) {
    case "CONFIRMED":
      return "Confirmed";
    case "GUIDE_ASSIGNED":
      return "Assigned to you";
    case "GUIDE_EN_ROUTE":
      return "On the way";
    case "GUIDE_ARRIVED":
      return "Arrived";
    case "IN_PROGRESS":
      return "In progress";
    case "COMPLETED":
      return "Completed";
    case "CANCELLED":
      return "Cancelled";
  }
}

export function formatDateTime(iso: string | null): string {
  if (!iso) return "—";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString(undefined, {
    day: "numeric",
    month: "short",
    hour: "2-digit",
    minute: "2-digit",
  });
}

export function formatPrice(amount: number, currency = "GHS"): string {
  const symbol = currency === "GHS" ? "GH₵" : `${currency} `;
  return `${symbol}${amount.toFixed(2)}`;
}

/** Seconds until `iso`, floored at 0. Display only — see file header. */
export function secondsUntil(iso: string | null, now = Date.now()): number {
  if (!iso) return 0;
  const t = new Date(iso).getTime();
  if (Number.isNaN(t)) return 0;
  return Math.max(0, Math.floor((t - now) / 1000));
}
