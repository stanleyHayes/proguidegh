"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import Link from "next/link";
import { Alert, Badge, Button, Select } from "@proguidegh/ui";
import { api, ApiError, errorMessage, unwrap } from "../../lib/api";
import {
  PIPELINE_STAGES,
  formatDate,
  formatDateTime,
  isExceptionStatus,
  stageIndex,
  stageLabel,
  statusTone,
} from "../../lib/certification";

/** Assumed shapes of GET /me/guide and GET /me/guide/certification (spec §5, §13.4). */
interface CertificationSummary {
  case_id?: string;
  status?: string;
  outstanding?: string[];
}

interface GuideDocument {
  id: string;
  type?: string;
  status?: string;
  expires_at?: string | null;
}

interface MeGuideResponse {
  profile?: { public_name?: string };
  certification?: CertificationSummary;
  documents?: GuideDocument[];
}

interface CertificationEvent {
  from_status?: string | null;
  to_status?: string;
  actor?: string;
  reason?: string;
  created_at?: string;
}

interface CertificationDetail {
  case?: { id?: string; status?: string; opened_at?: string };
  events?: CertificationEvent[];
}

const DOCUMENT_TYPES = [
  { value: "national_id", label: "National ID / Ghana Card" },
  { value: "passport", label: "Passport" },
  { value: "certification", label: "Tourism certificate" },
  { value: "background_check", label: "Background check" },
  { value: "insurance", label: "Insurance evidence" },
  { value: "other", label: "Other supporting document" },
];

type UploadStatus = "registering" | "uploading" | "done" | "error";

interface UploadItem {
  id: number;
  fileName: string;
  documentType: string;
  status: UploadStatus;
  error?: string;
}

/** Assumed response of POST /guides/documents. */
interface DocumentUploadTarget {
  upload_url?: string;
  uploadUrl?: string;
  document_id?: string;
}

type LoadState = "loading" | "unauthenticated" | "error" | "ready";

function documentTypeLabel(type?: string): string {
  return (
    DOCUMENT_TYPES.find((t) => t.value === type)?.label ??
    (type ? type.replace(/_/g, " ") : "Document")
  );
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

let nextUploadId = 1;

export default function GuideVerificationPage() {
  const [state, setState] = useState<LoadState>("loading");
  const [loadError, setLoadError] = useState<string | null>(null);
  const [certification, setCertification] =
    useState<CertificationSummary | null>(null);
  const [openedAt, setOpenedAt] = useState<string | null>(null);
  const [events, setEvents] = useState<CertificationEvent[]>([]);
  const [documents, setDocuments] = useState<GuideDocument[]>([]);
  const [documentType, setDocumentType] = useState(
    DOCUMENT_TYPES[0]?.value ?? "",
  );
  const [selectedFile, setSelectedFile] = useState<File | null>(null);
  const [uploads, setUploads] = useState<UploadItem[]>([]);
  const [formError, setFormError] = useState<string | null>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);

  const load = useCallback(async () => {
    setState("loading");
    setLoadError(null);
    try {
      const meData = unwrap<MeGuideResponse>(await api<unknown>("/me/guide"), "guide");
      setCertification(meData.certification ?? null);
      setDocuments(Array.isArray(meData.documents) ? meData.documents : []);

      try {
        const detail = unwrap<CertificationDetail>(
          await api<unknown>("/me/guide/certification"),
          "certification",
        );
        if (detail.case?.status) {
          setCertification((prev) => ({
            case_id: prev?.case_id ?? detail.case?.id,
            status: detail.case?.status ?? prev?.status,
            outstanding: prev?.outstanding ?? [],
          }));
        }
        setOpenedAt(detail.case?.opened_at ?? null);
        setEvents(Array.isArray(detail.events) ? detail.events : []);
      } catch (detailErr) {
        // 404 = no certification case opened yet — not fatal for this page.
        if (!(detailErr instanceof ApiError && detailErr.status === 404)) {
          throw detailErr;
        }
        setOpenedAt(null);
        setEvents([]);
      }
      setState("ready");
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        setState("unauthenticated");
      } else {
        setLoadError(
          errorMessage(
            err,
            "Could not load your certification status. Please retry.",
          ),
        );
        setState("error");
      }
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  function patchUpload(id: number, patch: Partial<UploadItem>) {
    setUploads((prev) =>
      prev.map((u) => (u.id === id ? { ...u, ...patch } : u)),
    );
  }

  async function onUpload(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setFormError(null);
    if (!selectedFile) {
      setFormError("Choose a document file first.");
      return;
    }
    const id = nextUploadId++;
    const item: UploadItem = {
      id,
      fileName: selectedFile.name,
      documentType: documentType,
      status: "registering",
    };
    setUploads((prev) => [item, ...prev]);

    try {
      // 1) Register the document and obtain a signed upload URL.
      const registered = unwrap<DocumentUploadTarget>(
        await api("/guides/documents", {
          method: "POST",
          body: {
            type: documentType,
            content_type: selectedFile.type || "application/octet-stream",
          },
        }),
        "document",
      );
      const uploadUrl = registered.upload_url ?? registered.uploadUrl;
      if (!uploadUrl) {
        throw new Error("The API did not return an upload URL.");
      }

      // 2) PUT the file bytes straight to storage (signed URL — no cookies).
      patchUpload(id, { status: "uploading" });
      const put = await fetch(uploadUrl, {
        method: "PUT",
        headers: {
          "Content-Type": selectedFile.type || "application/octet-stream",
        },
        body: selectedFile,
      });
      if (!put.ok) {
        throw new Error(`Upload to storage failed with status ${put.status}.`);
      }

      patchUpload(id, { status: "done" });
      setSelectedFile(null);
      if (fileInputRef.current) fileInputRef.current.value = "";
      // Refresh so the new document appears in the server-side list.
      void load();
    } catch (err) {
      patchUpload(id, {
        status: "error",
        error: errorMessage(
          err,
          err instanceof Error
            ? err.message
            : "Upload failed. Check your connection and try again.",
        ),
      });
    }
  }

  if (state === "loading") {
    return (
      <div className="stack" aria-busy="true" aria-label="Loading verification status">
        <div className="gg-skeleton" style={{ height: "2rem", width: "50%" }} />
        {Array.from({ length: 4 }, (_, i) => (
          <div key={i} className="gg-skeleton" style={{ height: "3rem" }} />
        ))}
      </div>
    );
  }

  if (state === "unauthenticated") {
    return (
      <div className="stack">
        <h1>Verification</h1>
        <Alert tone="info" title="Sign in required">
          <p>
            Sign in with your guide account to track your certification and
            upload documents.
          </p>
        </Alert>
        <p>
          <Link className="gg-button gg-button--primary" href="/login">
            Sign in
          </Link>
        </p>
      </div>
    );
  }

  if (state === "error") {
    return (
      <div className="stack">
        <h1>Verification &amp; certification</h1>
        <Alert tone="error" title="Something went wrong">
          <p>{loadError}</p>
        </Alert>
        <div>
          <Button type="button" onClick={() => void load()}>
            Retry
          </Button>
        </div>
      </div>
    );
  }

  const currentStatus = certification?.status ?? null;
  const currentIndex = stageIndex(currentStatus);
  const outstanding = certification?.outstanding ?? [];

  return (
    <div className="stack">
      <section aria-labelledby="verification-heading">
        <h1 id="verification-heading">Verification &amp; certification</h1>
        <p className="muted">
          Track your certification pipeline and upload the required documents.
        </p>
        {currentStatus ? (
          <Badge tone={statusTone(currentStatus)}>
            {stageLabel(currentStatus)}
          </Badge>
        ) : null}
      </section>

      <section aria-labelledby="pipeline-heading" className="stack">
        <h2 id="pipeline-heading">Pipeline status</h2>

        {!currentStatus ? (
          <Alert tone="info" title="No application yet">
            <p>
              Your certification pipeline opens when you submit your guide
              application.
            </p>
            <p>
              <Link
                className="gg-button gg-button--primary"
                href="/guide/apply"
              >
                Start application
              </Link>
            </p>
          </Alert>
        ) : null}

        {currentStatus && isExceptionStatus(currentStatus) ? (
          <Alert
            tone={currentStatus === "REJECTED" ? "error" : "info"}
            title={stageLabel(currentStatus)}
          >
            <p>
              Your certification is currently in the &ldquo;
              {stageLabel(currentStatus)}&rdquo; state. Check the history below
              for the reason, or contact support if you believe this is a
              mistake.
            </p>
          </Alert>
        ) : null}

        {currentStatus && currentIndex >= 0 ? (
          <>
            {openedAt ? (
              <p className="muted">Pipeline opened {formatDate(openedAt)}.</p>
            ) : null}
            <ol className="pipeline-list" aria-label="Certification stages">
              {PIPELINE_STAGES.map((stage, index) => {
                const state =
                  index < currentIndex
                    ? "done"
                    : index === currentIndex
                      ? "current"
                      : "upcoming";
                return (
                  <li
                    key={stage}
                    aria-current={state === "current" ? "step" : undefined}
                  >
                    <span>{stageLabel(stage)}</span>
                    <Badge
                      tone={
                        state === "done"
                          ? "success"
                          : state === "current"
                            ? "warning"
                            : "neutral"
                      }
                    >
                      {state === "done"
                        ? "Done"
                        : state === "current"
                          ? "Current"
                          : "Pending"}
                    </Badge>
                  </li>
                );
              })}
            </ol>
          </>
        ) : null}
      </section>

      {outstanding.length > 0 ? (
        <section aria-labelledby="outstanding-heading" className="stack">
          <h2 id="outstanding-heading">Outstanding requirements</h2>
          <Alert tone="info">
            <p>Complete these to move to the next stage:</p>
          </Alert>
          <ul className="pipeline-list">
            {outstanding.map((item) => (
              <li key={item}>
                <span>{item}</span>
                <Badge tone="warning">Required</Badge>
              </li>
            ))}
          </ul>
        </section>
      ) : null}

      {events.length > 0 ? (
        <section aria-labelledby="history-heading" className="stack">
          <h2 id="history-heading">History</h2>
          <ol className="pipeline-list" aria-label="Certification events">
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
        </section>
      ) : null}

      <section aria-labelledby="documents-heading" className="stack">
        <h2 id="documents-heading">Documents</h2>

        {documents.length === 0 ? (
          <p className="muted">
            No documents on file yet. Required: a national ID and a profile
            photo before review can start.
          </p>
        ) : (
          <ul className="pipeline-list" aria-label="Uploaded documents">
            {documents.map((doc) => (
              <li key={doc.id}>
                <span>
                  {documentTypeLabel(doc.type)}
                  {doc.expires_at ? (
                    <span className="muted">
                      {" "}
                      · expires {formatDate(doc.expires_at)}
                    </span>
                  ) : null}
                </span>
                <Badge tone={documentTone(doc.status)}>
                  {doc.status ?? "uploaded"}
                </Badge>
              </li>
            ))}
          </ul>
        )}

        <h3>Upload a document</h3>

        {formError ? (
          <Alert tone="error" title="Upload failed">
            <p>{formError}</p>
          </Alert>
        ) : null}

        <form className="stack" onSubmit={onUpload}>
          <Select
            label="Document type"
            name="document_type"
            value={documentType}
            onChange={(e) => setDocumentType(e.target.value)}
          >
            {DOCUMENT_TYPES.map((opt) => (
              <option key={opt.value} value={opt.value}>
                {opt.label}
              </option>
            ))}
          </Select>
          <div className="gg-field">
            <label className="gg-field__label" htmlFor="document-file">
              File
            </label>
            <input
              id="document-file"
              className="gg-field__control"
              type="file"
              ref={fileInputRef}
              onChange={(e) => setSelectedFile(e.target.files?.[0] ?? null)}
              accept="image/*,application/pdf"
            />
          </div>
          <div>
            <Button type="submit" disabled={!selectedFile}>
              Upload document
            </Button>
          </div>
        </form>

        {uploads.length > 0 ? (
          <ul className="pipeline-list" aria-label="Uploads this session">
            {uploads.map((upload) => (
              <li key={upload.id}>
                <span>
                  {upload.fileName}
                  <span className="muted">
                    {" "}
                    ·{" "}
                    {DOCUMENT_TYPES.find((t) => t.value === upload.documentType)
                      ?.label ?? upload.documentType}
                  </span>
                  {upload.status === "error" && upload.error ? (
                    <span className="gg-field__error"> — {upload.error}</span>
                  ) : null}
                </span>
                <Badge
                  tone={
                    upload.status === "done"
                      ? "success"
                      : upload.status === "error"
                        ? "danger"
                        : "neutral"
                  }
                >
                  {upload.status === "registering"
                    ? "Preparing…"
                    : upload.status === "uploading"
                      ? "Uploading…"
                      : upload.status === "done"
                        ? "Uploaded"
                        : "Failed"}
                </Badge>
              </li>
            ))}
          </ul>
        ) : null}
      </section>
    </div>
  );
}
