/**
 * Dispatch / tour-operations types and helpers (spec §8.2, §10, §11).
 *
 * API shapes are assumed while the backend is built concurrently — parsers
 * tolerate wrapped-or-bare lists, snake_case-or-camelCase keys, and nested
 * package names ({package:{name}} vs package_name).
 */

/** Parse a list that may be bare or wrapped in a known key. */
function parseList(data: unknown, keys: string[]): unknown[] {
  if (Array.isArray(data)) return data;
  if (data !== null && typeof data === "object") {
    for (const key of keys) {
      const list = (data as Record<string, unknown>)[key];
      if (Array.isArray(list)) return list;
    }
  }
  return [];
}

function asRecord(value: unknown): Record<string, unknown> | null {
  return value !== null && typeof value === "object"
    ? (value as Record<string, unknown>)
    : null;
}

function asString(value: unknown): string | undefined {
  return typeof value === "string" && value ? value : undefined;
}

function asNumber(value: unknown): number | undefined {
  return typeof value === "number" && Number.isFinite(value)
    ? value
    : undefined;
}

/* ---------------------------------------------------------------- offers */

/** Assumed shape of GET /me/guide/offers → booking summary (spec §10.3). */
export interface OfferBooking {
  id: string;
  reference?: string;
  package_name?: string;
  starts_at?: string;
  meeting_point?: string;
  guests?: number;
  amount?: number;
  currency?: string;
}

/** Assumed shape of GET /me/guide/offers entries and /ws/guide pushes. */
export interface Offer {
  id: string;
  booking: OfferBooking;
  score?: number;
  expires_at?: string;
}

function parseOffer(entry: unknown): Offer | null {
  const record = asRecord(entry);
  if (!record || typeof record.id !== "string") return null;
  const booking = asRecord(record.booking);
  if (!booking || typeof booking.id !== "string") return null;
  const pkg = asRecord(booking.package);
  return {
    id: record.id,
    booking: {
      id: booking.id,
      reference: asString(booking.reference),
      package_name:
        asString(booking.package_name) ??
        asString(booking.packageName) ??
        asString(pkg?.name) ??
        asString(pkg?.code),
      starts_at: asString(booking.starts_at) ?? asString(booking.startsAt),
      meeting_point:
        asString(booking.meeting_point) ?? asString(booking.meetingPoint),
      guests: asNumber(booking.guests),
      amount: asNumber(booking.amount),
      currency: asString(booking.currency),
    },
    score: asNumber(record.score),
    expires_at: asString(record.expires_at) ?? asString(record.expiresAt),
  };
}

export function parseOffers(data: unknown): Offer[] {
  return parseList(data, ["offers", "items", "results"])
    .map(parseOffer)
    .filter((offer): offer is Offer => offer !== null);
}

/** True once the offer countdown has run out (spec §10.3: 20–45s TTLs). */
export function isOfferExpired(offer: Offer, now = Date.now()): boolean {
  if (!offer.expires_at) return false;
  const expiry = new Date(offer.expires_at).getTime();
  return !Number.isNaN(expiry) && expiry <= now;
}

export function secondsRemaining(offer: Offer, now = Date.now()): number {
  if (!offer.expires_at) return 0;
  const expiry = new Date(offer.expires_at).getTime();
  if (Number.isNaN(expiry)) return 0;
  return Math.max(0, Math.ceil((expiry - now) / 1000));
}

/* ----------------------------------------------------------------- tours */

/** Assumed shape of an assigned booking (list + detail, spec §8.2). */
export interface AssignedBooking {
  id: string;
  reference?: string;
  status?: string;
  package_name?: string;
  tourist_name?: string;
  starts_at?: string;
  ends_at?: string;
  meeting_point?: string;
  guests?: number;
  amount?: number;
  currency?: string;
  notes?: string;
}

function parseAssignedBooking(entry: unknown): AssignedBooking | null {
  const record = asRecord(entry);
  if (!record || typeof record.id !== "string") return null;
  const pkg = asRecord(record.package);
  const tourist =
    asRecord(record.tourist) ?? asRecord(record.customer) ?? asRecord(record.user);
  return {
    id: record.id,
    reference: asString(record.reference),
    status: asString(record.status),
    package_name:
      asString(record.package_name) ??
      asString(record.packageName) ??
      asString(pkg?.name) ??
      asString(pkg?.code),
    tourist_name:
      asString(record.tourist_name) ??
      asString(tourist?.name) ??
      asString(tourist?.full_name) ??
      asString(tourist?.public_name),
    starts_at: asString(record.starts_at) ?? asString(record.startsAt),
    ends_at: asString(record.ends_at) ?? asString(record.endsAt),
    meeting_point:
      asString(record.meeting_point) ?? asString(record.meetingPoint),
    guests: asNumber(record.guests),
    amount: asNumber(record.amount),
    currency: asString(record.currency),
    notes: asString(record.notes),
  };
}

export function parseAssignedBookings(data: unknown): AssignedBooking[] {
  return parseList(data, ["bookings", "items", "results"])
    .map(parseAssignedBooking)
    .filter((booking): booking is AssignedBooking => booking !== null);
}

/** Tolerant single-booking parse (wrapped in `booking` or bare). */
export function parseAssignedBookingDetail(data: unknown): AssignedBooking | null {
  const record = asRecord(data);
  if (!record) return null;
  if ("booking" in record) return parseAssignedBooking(record.booking);
  return parseAssignedBooking(record);
}

/* -------------------------------------------------- status / transitions */

/** Operational leg of the booking flow (spec §8.2), used for the stepper. */
export const TOUR_FLOW = [
  "CONFIRMED",
  "GUIDE_EN_ROUTE",
  "GUIDE_ARRIVED",
  "IN_PROGRESS",
  "COMPLETED",
] as const;

/** Statuses where live location sharing is required (spec §11). */
export const LIVE_TOUR_STATUSES = [
  "GUIDE_EN_ROUTE",
  "GUIDE_ARRIVED",
  "IN_PROGRESS",
] as const;

export function isLiveTourStatus(status?: string): boolean {
  return (LIVE_TOUR_STATUSES as readonly string[]).includes(status ?? "");
}

export function isPastTour(booking: AssignedBooking): boolean {
  if (booking.status && (TOUR_FLOW as readonly string[]).includes(booking.status)) {
    return booking.status === "COMPLETED";
  }
  // Non-flow statuses (cancellations, no-show) count as past.
  return true;
}

export interface NextTourAction {
  label: string;
  confirm: string;
  /** POST /bookings/{id}<endpoint> */
  endpoint: string;
}

/** The single next operational action for a status (spec §8.2). */
export function nextTourAction(status?: string): NextTourAction | null {
  switch (status) {
    case "CONFIRMED":
      return {
        label: "En route",
        confirm:
          "Head to the meeting point now? The tourist will see your live location from this point.",
        endpoint: "/en-route",
      };
    case "GUIDE_EN_ROUTE":
      return {
        label: "Arrived",
        confirm: "Confirm you have arrived at the meeting point.",
        endpoint: "/arrived",
      };
    case "GUIDE_ARRIVED":
      return {
        label: "Start tour",
        confirm: "Start the tour now? The booking moves to in progress.",
        endpoint: "/start",
      };
    case "IN_PROGRESS":
      return {
        label: "Complete tour",
        confirm:
          "Complete this tour? This finishes the booking and stops live location sharing.",
        endpoint: "/complete",
      };
    default:
      return null;
  }
}

/** "GUIDE_EN_ROUTE" → "Guide en route". */
export function formatStatus(status?: string): string {
  if (!status) return "Unknown";
  return status
    .toLowerCase()
    .split("_")
    .map((word) => word.charAt(0).toUpperCase() + word.slice(1))
    .join(" ");
}

export function formatDateTime(iso?: string): string {
  if (!iso) return "To be scheduled";
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return iso;
  return date.toLocaleString(undefined, {
    dateStyle: "medium",
    timeStyle: "short",
  });
}

/** Prices are assumed to be major units (e.g. 250.00 GHS). */
export function formatPrice(price?: number, currency?: string): string {
  if (price === undefined || price === null) return "—";
  const code = (currency ?? "GHS").toUpperCase();
  const symbol = code === "GHS" ? "GH₵" : `${code} `;
  return `${symbol}${price.toLocaleString(undefined, {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  })}`;
}
