/**
 * Certification pipeline constants shared by the admin screens (spec §5).
 * The state machine itself is enforced by the backend — this is display-only.
 */

export const PIPELINE_STAGES = [
  "APPLIED",
  "IDENTITY_PENDING",
  "IDENTITY_VERIFIED",
  "BACKGROUND_CHECK_PENDING",
  "BACKGROUND_VERIFIED",
  "TRAINING",
  "EXAM_PENDING",
  "CERTIFIED",
  "INSURANCE_ACTIVE",
  "ACTIVE",
] as const;

export const EXCEPTION_STATUSES = [
  "REJECTED",
  "SUSPENDED",
  "EXPIRED",
  "REQUIRES_RETRAINING",
] as const;

export const ALL_STATUSES = [...PIPELINE_STAGES, ...EXCEPTION_STATUSES];

const STAGE_LABELS: Record<string, string> = {
  APPLIED: "Application received",
  IDENTITY_PENDING: "Identity review pending",
  IDENTITY_VERIFIED: "Identity verified",
  BACKGROUND_CHECK_PENDING: "Background check pending",
  BACKGROUND_VERIFIED: "Background verified",
  TRAINING: "Mandatory training",
  EXAM_PENDING: "Certification exam pending",
  CERTIFIED: "Certified",
  INSURANCE_ACTIVE: "Insurance active",
  ACTIVE: "Active",
  REJECTED: "Rejected",
  SUSPENDED: "Suspended",
  EXPIRED: "Expired",
  REQUIRES_RETRAINING: "Requires retraining",
};

/** Backend may return any casing; compare normalized. */
export function normalizeStatus(status?: string | null): string {
  return (status ?? "").trim().toUpperCase();
}

export function stageLabel(status?: string | null): string {
  const normalized = normalizeStatus(status);
  return (
    STAGE_LABELS[normalized] ??
    normalized
      .toLowerCase()
      .replace(/_/g, " ")
      .replace(/^\w/, (c) => c.toUpperCase())
  );
}

export function statusTone(
  status?: string | null,
): "neutral" | "success" | "warning" | "danger" {
  switch (normalizeStatus(status)) {
    case "CERTIFIED":
    case "INSURANCE_ACTIVE":
    case "ACTIVE":
    case "IDENTITY_VERIFIED":
    case "BACKGROUND_VERIFIED":
      return "success";
    case "REJECTED":
    case "SUSPENDED":
    case "EXPIRED":
      return "danger";
    case "REQUIRES_RETRAINING":
    case "APPLIED":
    case "IDENTITY_PENDING":
    case "BACKGROUND_CHECK_PENDING":
    case "TRAINING":
    case "EXAM_PENDING":
      return "warning";
    default:
      return "neutral";
  }
}

export function formatDate(value?: string | null): string {
  if (!value) return "—";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleDateString();
}

export function formatDateTime(value?: string | null): string {
  if (!value) return "—";
  const date = new Date(value);
  return Number.isNaN(date.getTime())
    ? value
    : `${date.toLocaleDateString()} ${date.toLocaleTimeString([], {
        hour: "2-digit",
        minute: "2-digit",
      })}`;
}
