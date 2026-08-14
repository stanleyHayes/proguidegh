"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { useParams } from "next/navigation";
import { Alert, Badge, Button, Input, Select, Textarea } from "@proguidegh/ui";
import { api, ApiError, errorMessage, unwrap } from "../../../lib/api";
import { Unauthorized } from "../../../components/Unauthorized";
import {
  ALL_STATUSES,
  formatDate,
  formatDateTime,
  normalizeStatus,
  stageLabel,
  statusTone,
} from "../../../lib/certification";

/** Assumed shape of GET /admin/certification/{caseId} (spec §5). */
interface CertificationCase {
  id?: string;
  status?: string;
  opened_at?: string;
}

interface GuideInfo {
  id?: string;
  public_name?: string;
  email?: string;
  region?: string;
  status?: string;
}

interface CaseDocument {
  id: string;
  type?: string;
  status?: string;
  expires_at?: string | null;
  download_url?: string;
  downloadUrl?: string;
  url?: string;
}

interface CaseEvent {
  from_status?: string | null;
  to_status?: string;
  actor?: string;
  reason?: string;
  created_at?: string;
}

interface CaseDetail {
  case?: CertificationCase;
  guide?: GuideInfo;
  documents?: CaseDocument[];
  events?: CaseEvent[];
}

type LoadState = "loading" | "unauthorized" | "forbidden" | "error" | "ready";

function documentDownloadUrl(doc: CaseDocument): string | null {
  return doc.download_url ?? doc.downloadUrl ?? doc.url ?? null;
}

function documentTone(
  status?: string,
): "neutral" | "success" | "warning" | "danger" {
  switch ((status ?? "").toLowerCase()) {
    case "verified":
    case "approved":
      return "success";
    case "rejected":
    case "expired":
      return "danger";
    case "pending":
    case "uploaded":
    case "in_review":
      return "warning";
    default:
      return "neutral";
  }
}

export default function AdminCertificationCasePage() {
  const params = useParams<{ caseId: string }>();
  const caseId = params.caseId;

  const [state, setState] = useState<LoadState>("loading");
  const [loadError, setLoadError] = useState<string | null>(null);
  const [detail, setDetail] = useState<CaseDetail | null>(null);

  const [toStatus, setToStatus] = useState("");
  const [reason, setReason] = useState("");
  const [evidenceRef, setEvidenceRef] = useState("");
  const [reasonError, setReasonError] = useState<string | undefined>();
  const [submitting, setSubmitting] = useState(false);
  const [actionError, setActionError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  const load = useCallback(async () => {
    setState("loading");
    setLoadError(null);
    try {
      const data = unwrap<CaseDetail>(
        await api<unknown>(
          `/admin/certification/${encodeURIComponent(caseId)}`,
        ),
        "certification",
      );
      setDetail(data);
      setState("ready");
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        setState("unauthorized");
      } else if (err instanceof ApiError && err.status === 403) {
        setState("forbidden");
      } else {
        setLoadError(
          errorMessage(err, "Could not load this certification case. Please retry."),
        );
        setState("error");
      }
    }
  }, [caseId]);

  useEffect(() => {
    void load();
  }, [load]);

  async function onTransition(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setActionError(null);
    setNotice(null);
    setReasonError(undefined);

    if (!toStatus) {
      setActionError("Select a target stage first.");
      return;
    }
    if (!reason.trim()) {
      setReasonError("A reason is required — it goes into the audit log.");
      return;
    }

    const current = detail?.case?.status;
    // Confirmation dialog for privileged, audited actions (spec §18.4).
    if (
      !window.confirm(
        `Move this case from “${stageLabel(current)}” to “${stageLabel(
          toStatus,
        )}”?\n\nReason: ${reason.trim()}`,
      )
    ) {
      return;
    }

    setSubmitting(true);
    try {
      await api(`/admin/certification/${encodeURIComponent(caseId)}/transition`, {
        method: "POST",
        body: {
          to_status: toStatus,
          reason: reason.trim(),
          ...(evidenceRef.trim() ? { evidence_ref: evidenceRef.trim() } : {}),
        },
      });
      setNotice(
        `Transition recorded: now at “${stageLabel(toStatus)}”.`,
      );
      setToStatus("");
      setReason("");
      setEvidenceRef("");
      await load();
    } catch (err) {
      if (err instanceof ApiError && (err.status === 401 || err.status === 403)) {
        setState(err.status === 401 ? "unauthorized" : "forbidden");
        return;
      }
      // 409 illegal transition / missing evidence, 422 validation — show the
      // backend's own message, it is the state machine's verdict.
      setActionError(
        errorMessage(err, "The transition was rejected. Please try again."),
      );
    } finally {
      setSubmitting(false);
    }
  }

  if (state === "unauthorized" || state === "forbidden") {
    return (
      <div className="stack">
        <h1>Certification case</h1>
        <Unauthorized forbidden={state === "forbidden"} />
      </div>
    );
  }

  if (state === "loading") {
    return (
      <div className="stack" aria-busy="true" aria-label="Loading certification case">
        <div className="gg-skeleton" style={{ height: "2rem", width: "40%" }} />
        {Array.from({ length: 5 }, (_, i) => (
          <div key={i} className="gg-skeleton" style={{ height: "3rem" }} />
        ))}
      </div>
    );
  }

  if (state === "error" || !detail) {
    return (
      <div className="stack">
        <h1>Certification case</h1>
        <Alert tone="error" title="Something went wrong">
          <p>{loadError ?? "Case not found."}</p>
        </Alert>
        <div className="nav-actions">
          <Button type="button" onClick={() => void load()}>
            Retry
          </Button>
          <Link
            className="gg-button gg-button--secondary"
            href="/admin/certification"
          >
            Back to queue
          </Link>
        </div>
      </div>
    );
  }

  const caseInfo = detail.case ?? {};
  const guide = detail.guide ?? {};
  const documents = Array.isArray(detail.documents) ? detail.documents : [];
  const events = Array.isArray(detail.events) ? detail.events : [];
  const currentNormalized = normalizeStatus(caseInfo.status);
  const targetOptions = ALL_STATUSES.filter(
    (status) => status !== currentNormalized,
  );

  return (
    <div className="stack">
      <section aria-labelledby="case-heading">
        <h1 id="case-heading">Certification case</h1>
        <p className="muted">
          Case {caseInfo.id ?? caseId}
          {caseInfo.opened_at
            ? ` · opened ${formatDate(caseInfo.opened_at)}`
            : ""}
        </p>
        <Badge tone={statusTone(caseInfo.status)}>
          {stageLabel(caseInfo.status)}
        </Badge>
      </section>

      <p>
        <Link href="/admin/certification">← Back to certification queue</Link>
      </p>

      {notice ? (
        <Alert tone="success" title="Transition recorded">
          <p>{notice}</p>
        </Alert>
      ) : null}

      {actionError ? (
        <Alert tone="error" title="Transition rejected">
          <p>{actionError}</p>
        </Alert>
      ) : null}

      <section aria-labelledby="guide-heading" className="stack">
        <h2 id="guide-heading">Guide</h2>
        <div className="gg-table-scroll">
          <table className="gg-table">
            <tbody>
              <tr>
                <th scope="row">Public name</th>
                <td>{guide.public_name ?? "—"}</td>
              </tr>
              <tr>
                <th scope="row">Email</th>
                <td>{guide.email ?? "—"}</td>
              </tr>
              <tr>
                <th scope="row">Region</th>
                <td>{guide.region ?? "—"}</td>
              </tr>
              <tr>
                <th scope="row">Guide status</th>
                <td>{guide.status ?? "—"}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>

      <section aria-labelledby="documents-heading" className="stack">
        <h2 id="documents-heading">Documents</h2>
        {documents.length === 0 ? (
          <Alert tone="info" title="No documents">
            <p>The guide has not uploaded any documents for this case yet.</p>
          </Alert>
        ) : (
          <ul className="pipeline-list" aria-label="Case documents">
            {documents.map((doc) => {
              const href = documentDownloadUrl(doc);
              return (
                <li key={doc.id}>
                  <span>
                    {doc.type ? doc.type.replace(/_/g, " ") : "Document"}
                    {doc.expires_at ? (
                      <span className="muted">
                        {" "}
                        · expires {formatDate(doc.expires_at)}
                      </span>
                    ) : null}
                    {href ? (
                      <>
                        {" "}
                        ·{" "}
                        <a href={href} target="_blank" rel="noopener noreferrer">
                          View
                        </a>
                      </>
                    ) : null}
                  </span>
                  <Badge tone={documentTone(doc.status)}>
                    {doc.status ?? "uploaded"}
                  </Badge>
                </li>
              );
            })}
          </ul>
        )}
      </section>

      <section aria-labelledby="history-heading" className="stack">
        <h2 id="history-heading">History</h2>
        {events.length === 0 ? (
          <p className="muted">No transitions recorded yet.</p>
        ) : (
          <ol className="pipeline-list" aria-label="Case events">
            {events.map((event, index) => (
              <li key={`${event.created_at ?? "event"}-${index}`}>
                <span>
                  {event.from_status
                    ? `${stageLabel(event.from_status)} → `
                    : ""}
                  <strong>{stageLabel(event.to_status)}</strong>
                  {event.reason ? (
                    <span className="muted"> — {event.reason}</span>
                  ) : null}
                  {event.actor ? (
                    <span className="muted"> · by {event.actor}</span>
                  ) : null}
                </span>
                <span className="muted">{formatDateTime(event.created_at)}</span>
              </li>
            ))}
          </ol>
        )}
      </section>

      <section aria-labelledby="transition-heading" className="stack">
        <h2 id="transition-heading">Record a transition</h2>
        <p className="muted">
          The backend state machine validates the target stage; illegal
          transitions are rejected. Reason and evidence are written to the
          audit log.
        </p>
        <form className="stack" onSubmit={onTransition} aria-busy={submitting}>
          <Select
            label="Target stage"
            name="to_status"
            required
            value={toStatus}
            onChange={(e) => setToStatus(e.target.value)}
            disabled={submitting}
          >
            <option value="" disabled>
              Select the next stage
            </option>
            {targetOptions.map((status) => (
              <option key={status} value={status}>
                {stageLabel(status)}
              </option>
            ))}
          </Select>
          <Textarea
            label="Reason"
            name="reason"
            hint="Required. Explain why this transition is being made."
            required
            error={reasonError}
            value={reason}
            onChange={(e) => {
              setReason(e.target.value);
              setReasonError(undefined);
            }}
            disabled={submitting}
          />
          <Input
            label="Evidence reference"
            name="evidence_ref"
            type="text"
            hint="Optional — document id, certificate number or policy reference."
            value={evidenceRef}
            onChange={(e) => setEvidenceRef(e.target.value)}
            disabled={submitting}
          />
          <div>
            <Button type="submit" disabled={submitting || !toStatus}>
              {submitting ? "Recording…" : "Record transition"}
            </Button>
          </div>
        </form>
      </section>
    </div>
  );
}
