/**
 * Guide/booking types and helpers (spec §8.2, §10.1, §13.2–13.3).
 *
 * API shapes are assumed while the backend is built concurrently — parsers
 * tolerate wrapped-or-bare payloads and string-or-object nested values.
 */

import type { BadgeTone } from "@proguidegh/ui";
import { unwrap } from "./api";
import { parseList } from "./catalog";

/** A nested value that may be a plain string or a named object. */
export type MaybeNamed = string | { id?: string; code?: string; name?: string };

/** Assumed shape of GET /guides/search entries and GET /guides/{id}. */
export interface Guide {
  id: string;
  public_name?: string;
  bio?: string;
  rating_avg?: number;
  rating_count?: number;
  elite_status?: boolean;
  region?: MaybeNamed;
  languages?: MaybeNamed[];
  specialties?: MaybeNamed[];
  completed_tours?: number;
}

/** Assumed shape of POST /bookings/quote (amounts in major units, 2dp). */
export interface Quote {
  package?: { id?: string; name?: string; code?: string };
  amount?: number;
  currency?: string;
  platform_fee?: number;
  tourism_levy?: number;
  guide_payable_estimate?: number;
}

/** Assumed shape of booking records (create/detail/history). */
export interface Booking {
  id: string;
  reference?: string;
  status?: string;
  package?: { id?: string; name?: string; code?: string };
  guide?: { id?: string; public_name?: string; name?: string };
  starts_at?: string;
  ends_at?: string;
  amount?: number;
  currency?: string;
  meeting_point?: string;
  guests?: number;
  notes?: string;
}

/** Assumed shape of POST /bookings/{id}/payment-intent (spec §4.5, §13). */
export interface PaymentIntent {
  provider?: string;
  authorization_url?: string;
  reference?: string;
  amount?: number;
  currency?: string;
}

/** Assumed shape of the receipt payload of GET /bookings/{id}/receipt (spec §17). */
export interface Receipt {
  receipt_number?: string;
  issued_at?: string;
  download_url?: string;
  amount?: number;
  currency?: string;
}

/** Happy-path state machine order (spec §8.2), used for the status timeline. */
export const BOOKING_FLOW = [
  "DRAFT",
  "PAYMENT_PENDING",
  "CONFIRMED",
  "GUIDE_EN_ROUTE",
  "GUIDE_ARRIVED",
  "IN_PROGRESS",
  "COMPLETED",
] as const;

/** Exceptional states (spec §8.2), shown as badges rather than timeline steps. */
export const BOOKING_EXCEPTIONS = [
  "PAYMENT_FAILED",
  "CANCELLED_BY_TOURIST",
  "CANCELLED_BY_GUIDE",
  "CANCELLED_BY_ADMIN",
  "NO_SHOW",
  "REFUND_PENDING",
  "REFUNDED",
] as const;

export function guideName(guide?: Guide | Booking["guide"]): string {
  if (!guide) return "Guide";
  if ("public_name" in guide && guide.public_name) return guide.public_name;
  if ("name" in guide && guide.name) return guide.name;
  return "Guide";
}

/** Extract a display label from a string-or-named-object value. */
export function labelOf(value?: MaybeNamed): string {
  if (!value) return "";
  if (typeof value === "string") return value;
  return value.name ?? value.code ?? "";
}

export function labelsOf(values?: MaybeNamed[]): string[] {
  return (values ?? []).map(labelOf).filter(Boolean);
}

export function parseGuides(data: unknown): Guide[] {
  return parseList(data, ["guides", "results", "items"]).filter(
    (entry): entry is Guide =>
      entry !== null && typeof entry === "object" && "id" in entry,
  );
}

export function parseBookings(data: unknown): Booking[] {
  return parseList(data, ["bookings", "items"]).filter(
    (entry): entry is Booking =>
      entry !== null && typeof entry === "object" && "id" in entry,
  );
}

export function bookingPackageName(booking: Booking): string {
  return booking.package?.name ?? booking.package?.code ?? "Tour package";
}

/** Tolerant parser for the payment-intent response (wrapped/bare, snake/camel). */
export function parsePaymentIntent(data: unknown): PaymentIntent {
  const raw = unwrap<unknown>(data, "payment_intent");
  if (raw === null || typeof raw !== "object") return {};
  const record = raw as Record<string, unknown>;
  const authorizationUrl = record.authorization_url ?? record.authorizationUrl;
  return {
    provider: typeof record.provider === "string" ? record.provider : undefined,
    authorization_url:
      typeof authorizationUrl === "string" ? authorizationUrl : undefined,
    reference:
      typeof record.reference === "string" ? record.reference : undefined,
    amount: typeof record.amount === "number" ? record.amount : undefined,
    currency:
      typeof record.currency === "string" ? record.currency : undefined,
  };
}

/** Tolerant parser for the receipt response (wrapped in `receipt` or bare). */
export function parseReceipt(data: unknown): Receipt {
  const raw = unwrap<unknown>(data, "receipt");
  if (raw === null || typeof raw !== "object") return {};
  const record = raw as Record<string, unknown>;
  const receiptNumber = record.receipt_number ?? record.receiptNumber;
  const issuedAt = record.issued_at ?? record.issuedAt;
  const downloadUrl = record.download_url ?? record.downloadUrl;
  return {
    receipt_number:
      typeof receiptNumber === "string" ? receiptNumber : undefined,
    issued_at: typeof issuedAt === "string" ? issuedAt : undefined,
    download_url: typeof downloadUrl === "string" ? downloadUrl : undefined,
    amount: typeof record.amount === "number" ? record.amount : undefined,
    currency:
      typeof record.currency === "string" ? record.currency : undefined,
  };
}

/**
 * True once payment has been collected (spec §4.5) — a receipt should exist
 * or be on its way, so the receipt link is shown and a 404 there is tolerated.
 */
export function isPaidStatus(status?: string): boolean {
  if (!status) return false;
  if (status === "REFUND_PENDING" || status === "REFUNDED") return true;
  const index = (BOOKING_FLOW as readonly string[]).indexOf(status);
  return index >= BOOKING_FLOW.indexOf("CONFIRMED");
}

/** "PAYMENT_PENDING" → "Payment pending". */
export function formatStatus(status?: string): string {
  if (!status) return "Unknown";
  return status
    .toLowerCase()
    .split("_")
    .map((word) => word.charAt(0).toUpperCase() + word.slice(1))
    .join(" ");
}

export function statusTone(status?: string): BadgeTone {
  switch (status) {
    case "CONFIRMED":
    case "COMPLETED":
      return "success";
    case "DRAFT":
    case "PAYMENT_PENDING":
    case "REFUND_PENDING":
      return "warning";
    case "PAYMENT_FAILED":
    case "CANCELLED_BY_TOURIST":
    case "CANCELLED_BY_GUIDE":
    case "CANCELLED_BY_ADMIN":
    case "NO_SHOW":
      return "danger";
    default:
      return "neutral";
  }
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

/** Convert a datetime-local input value to an ISO 8601 string for the API. */
export function localToIso(local: string): string | undefined {
  if (!local) return undefined;
  const date = new Date(local);
  return Number.isNaN(date.getTime()) ? undefined : date.toISOString();
}
