/**
 * Booking / payment / receipt types and tolerant parsers (Phase M, M-10–M-11).
 *
 * Money is never computed here. Every amount displayed comes from the server's
 * quote or booking snapshot (Spec §1.2 — server-authoritative pricing); this
 * module only formats what it is given.
 */

export type BookingStatus =
  | "PAYMENT_PENDING"
  | "CONFIRMED"
  | "GUIDE_ASSIGNED"
  | "GUIDE_EN_ROUTE"
  | "GUIDE_ARRIVED"
  | "IN_PROGRESS"
  | "COMPLETED"
  | "CANCELLED"
  | "REFUNDED"
  | "DISPUTED";

/** Price breakdown exactly as the server computed it (§14, §27). */
export interface QuoteBreakdown {
  amount: number;
  platformFee: number | null;
  tourismLevy: number | null;
  guidePayableEstimate: number | null;
  currency: string;
}

export interface Quote extends QuoteBreakdown {
  packageId: string;
  packageName: string | null;
  startsAt: string | null;
  endsAt: string | null;
  guests: number | null;
}

export interface BookingSummary {
  id: string;
  reference: string;
  status: BookingStatus;
  packageName: string | null;
  startsAt: string | null;
  endsAt: string | null;
  amount: number | null;
  currency: string;
  guideName: string | null;
  meetingPoint: string | null;
  numGuests: number | null;
}

export interface BookingEvent {
  status: string;
  at: string | null;
  reason: string | null;
}

export interface BookingDetail extends BookingSummary {
  events: BookingEvent[];
}

export interface PaymentIntent {
  paymentId: string | null;
  reference: string | null;
  authorizationUrl: string | null;
  /** "mock" in non-production builds — drives the test-payment badge. */
  provider: string | null;
}

export interface Receipt {
  number: string | null;
  issuedAt: string | null;
  downloadUrl: string;
  expiresIn: number | null;
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

const BOOKING_STATUSES: ReadonlySet<string> = new Set<BookingStatus>([
  "PAYMENT_PENDING",
  "CONFIRMED",
  "GUIDE_ASSIGNED",
  "GUIDE_EN_ROUTE",
  "GUIDE_ARRIVED",
  "IN_PROGRESS",
  "COMPLETED",
  "CANCELLED",
  "REFUNDED",
  "DISPUTED",
]);

function status(value: unknown): BookingStatus {
  const raw = str(value)?.toUpperCase() ?? "";
  return BOOKING_STATUSES.has(raw)
    ? (raw as BookingStatus)
    : "PAYMENT_PENDING";
}

export function parseQuote(data: unknown): Quote | null {
  const rec = asRecord(asRecord(data)?.quote ?? data);
  if (!rec) return null;
  const pkg = asRecord(rec.package);
  const price = asRecord(rec.price);
  const amount = num(price?.amount) ?? num(rec.amount);
  if (amount === null) return null;
  return {
    packageId: str(rec.package_id) ?? str(pkg?.id) ?? "",
    packageName: str(pkg?.name) ?? str(rec.package_name),
    startsAt: str(rec.starts_at),
    endsAt: str(rec.ends_at),
    guests: num(rec.guests) ?? num(rec.num_guests),
    amount,
    platformFee: num(price?.platform_fee) ?? num(rec.platform_fee),
    tourismLevy: num(price?.tourism_levy) ?? num(rec.tourism_levy),
    guidePayableEstimate:
      num(price?.guide_payable_estimate) ?? num(rec.guide_payable_estimate),
    currency: str(price?.currency) ?? str(rec.currency) ?? "GHS",
  };
}

function parseBookingRecord(rec: Record<string, unknown>): BookingSummary | null {
  const id = str(rec.id);
  if (!id) return null;
  return {
    id,
    reference: str(rec.reference) ?? id.slice(0, 8),
    status: status(rec.status),
    packageName: str(rec.package_name) ?? str(asRecord(rec.package)?.name),
    startsAt: str(rec.starts_at),
    endsAt: str(rec.ends_at),
    amount: num(rec.amount),
    currency: str(rec.currency) ?? "GHS",
    guideName: str(rec.guide_name) ?? str(asRecord(rec.guide)?.public_name),
    meetingPoint: str(rec.meeting_point),
    numGuests: num(rec.num_guests) ?? num(rec.guests),
  };
}

export function parseBookings(data: unknown): BookingSummary[] {
  return asList(data, "bookings", "items", "results")
    .map((entry) => {
      const rec = asRecord(entry);
      return rec ? parseBookingRecord(rec) : null;
    })
    .filter((v): v is BookingSummary => v !== null);
}

export function parseBookingDetail(data: unknown): BookingDetail | null {
  const outer = asRecord(data);
  const rec = asRecord(outer?.booking ?? data);
  if (!rec) return null;
  const base = parseBookingRecord(rec);
  if (!base) return null;
  const events = asList(outer?.events ?? rec.events, "events")
    .map((entry) => {
      const e = asRecord(entry);
      if (!e) return null;
      const s = str(e.status) ?? str(e.to_status);
      if (!s) return null;
      return {
        status: s,
        at: str(e.at) ?? str(e.created_at) ?? str(e.occurred_at),
        reason: str(e.reason),
      } satisfies BookingEvent;
    })
    .filter((v): v is BookingEvent => v !== null);
  return { ...base, events };
}

export function parsePaymentIntent(data: unknown): PaymentIntent {
  const rec = asRecord(asRecord(data)?.payment ?? data);
  return {
    paymentId: str(rec?.id) ?? str(rec?.payment_id),
    reference: str(rec?.reference) ?? str(rec?.provider_reference),
    authorizationUrl: str(rec?.authorization_url),
    provider: str(rec?.provider),
  };
}

export function parseReceipt(data: unknown): Receipt | null {
  const outer = asRecord(data);
  const rec = asRecord(outer?.receipt ?? data);
  // The API returns download_url/expires_in as siblings of `receipt`.
  const downloadUrl = str(outer?.download_url) ?? str(rec?.download_url);
  if (!downloadUrl) return null;
  return {
    number: str(rec?.receipt_number) ?? str(rec?.number),
    issuedAt: str(rec?.issued_at),
    downloadUrl,
    expiresIn: num(outer?.expires_in) ?? num(rec?.expires_in),
  };
}

/** Human label for a booking status (Spec §8.2). */
export function statusLabel(value: BookingStatus): string {
  switch (value) {
    case "PAYMENT_PENDING":
      return "Awaiting payment";
    case "CONFIRMED":
      return "Confirmed";
    case "GUIDE_ASSIGNED":
      return "Guide assigned";
    case "GUIDE_EN_ROUTE":
      return "Guide on the way";
    case "GUIDE_ARRIVED":
      return "Guide arrived";
    case "IN_PROGRESS":
      return "Tour in progress";
    case "COMPLETED":
      return "Completed";
    case "CANCELLED":
      return "Cancelled";
    case "REFUNDED":
      return "Refunded";
    case "DISPUTED":
      return "Disputed";
  }
}

export function statusTone(
  value: BookingStatus,
): "neutral" | "success" | "gold" {
  if (value === "COMPLETED" || value === "CONFIRMED") return "success";
  if (value === "CANCELLED" || value === "REFUNDED" || value === "DISPUTED") {
    return "neutral";
  }
  if (value === "PAYMENT_PENDING") return "gold";
  return "success";
}

/** A receipt is only offered once money has actually settled. */
export function hasReceipt(value: BookingStatus): boolean {
  return value !== "PAYMENT_PENDING" && value !== "CANCELLED";
}

export function formatDateTime(iso: string | null): string {
  if (!iso) return "—";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString(undefined, {
    day: "numeric",
    month: "short",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}
