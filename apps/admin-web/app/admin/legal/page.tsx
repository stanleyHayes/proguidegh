"use client";

import { useCallback, useEffect, useState } from "react";
import { Alert, Badge, Button, Card, Input, Textarea } from "@proguidegh/ui";
import { api, ApiError, errorMessage } from "../../lib/api";
import { Unauthorized } from "../../components/Unauthorized";

/**
 * Legal document editor (M-24).
 *
 * Two deliberate constraints, both enforced server-side and mirrored here so
 * the UI cannot imply otherwise:
 *
 * 1. **Saving publishes a new version.** It never edits an existing one.
 *    `consent_records` references (document, version), so rewriting text in
 *    place would silently re-point a user's recorded consent at different
 *    words. The version field is therefore required and must be new.
 * 2. **Publishing is not approving.** A new version goes live as a draft with
 *    a banner on the public page. Approve is a separate, audited action for
 *    when counsel has actually signed off.
 */

const DOCUMENTS = ["terms", "privacy", "location"] as const;
type DocumentKey = (typeof DOCUMENTS)[number];

interface LegalDocument {
  document: string;
  version: string;
  url: string;
  summary?: string;
  body?: string;
  approved: boolean;
  approved_at?: string;
  published_at: string;
}

type LoadState = "loading" | "unauthorized" | "forbidden" | "error" | "ready";

function asRecord(value: unknown): Record<string, unknown> | null {
  return value !== null && typeof value === "object" ? (value as Record<string, unknown>) : null;
}

/** Today as YYYY-MM-DD — the versioning convention already in the table. */
function today(): string {
  return new Date().toISOString().slice(0, 10);
}

export default function LegalPage() {
  const [state, setState] = useState<LoadState>("loading");
  const [docs, setDocs] = useState<LegalDocument[]>([]);
  const [selected, setSelected] = useState<DocumentKey>("terms");
  const [version, setVersion] = useState(today());
  const [summary, setSummary] = useState("");
  const [body, setBody] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    try {
      const data = await api("/admin/legal");
      const list = asRecord(data)?.documents;
      setDocs(Array.isArray(list) ? (list as LegalDocument[]) : []);
      setState("ready");
      setError(null);
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) setState("unauthorized");
      else if (err instanceof ApiError && err.status === 403) setState("forbidden");
      else {
        setError(errorMessage(err, "Could not load legal documents."));
        setState("error");
      }
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  // Load the newest version of whichever document is selected as the starting
  // point for the next one — editors revise, they rarely start from nothing.
  useEffect(() => {
    const latest = docs.find((d) => d.document === selected);
    setSummary(latest?.summary ?? "");
    setBody(latest?.body ?? "");
    setVersion(today());
  }, [selected, docs]);

  async function publish() {
    setBusy(true);
    setError(null);
    setNotice(null);
    try {
      await api(`/admin/legal/${selected}`, {
        method: "POST",
        body: { version, summary, body },
      });
      setNotice(
        `Published ${selected} v${version} as a draft. The public page shows it with a ` +
          "draft banner until you approve it.",
      );
      await load();
    } catch (err) {
      if (err instanceof ApiError && err.status === 409) {
        setError(
          `Version ${version} already exists. Pick a new version — existing versions are ` +
            "never overwritten because users have consented to them.",
        );
      } else {
        setError(errorMessage(err, "Could not publish this version."));
      }
    } finally {
      setBusy(false);
    }
  }

  async function approve(doc: string, ver: string) {
    setBusy(true);
    setError(null);
    setNotice(null);
    try {
      await api(`/admin/legal/${doc}/${encodeURIComponent(ver)}/approve`, { method: "POST" });
      setNotice(`Approved ${doc} v${ver}. The draft banner is now gone from the public page.`);
      await load();
    } catch (err) {
      setError(errorMessage(err, "Could not approve this version."));
    } finally {
      setBusy(false);
    }
  }

  if (state === "loading") return <p>Loading legal documents…</p>;
  if (state === "unauthorized" || state === "forbidden") return <Unauthorized />;

  const history = docs.filter((d) => d.document === selected);

  return (
    <section className="stack">
      <div>
        <h1>Legal documents</h1>
        <p style={{ color: "var(--gg-color-muted)" }}>
          Terms, privacy and location policy. Saving publishes a <strong>new version</strong> —
          existing versions are never rewritten, because users have already consented to
          them. Approving is separate and is what removes the draft banner from
          proguidegh.com.
        </p>
      </div>

      {error ? <Alert tone="error" title="Problem">{error}</Alert> : null}
      {notice ? <Alert tone="success" title="Done">{notice}</Alert> : null}

      <Card>
        <h2>Document</h2>
        <div style={{ display: "flex", gap: "0.5rem", flexWrap: "wrap" }}>
          {DOCUMENTS.map((doc) => (
            <Button
              key={doc}
              onClick={() => setSelected(doc)}
              variant={doc === selected ? "primary" : "secondary"}
            >
              {doc}
            </Button>
          ))}
        </div>
      </Card>

      <Card>
        <h2>Publish a new version</h2>
        <Input
          hint="Convention is the date, YYYY-MM-DD. Must not match an existing version."
          label="Version"
          onChange={(e) => setVersion(e.target.value)}
          value={version}
        />
        <Input
          hint="One line. Shown as the page lede and the search-result description."
          label="Summary"
          onChange={(e) => setSummary(e.target.value)}
          value={summary}
        />
        <Textarea
          hint="Markdown: ## and ### headings, paragraphs, - bullets, 1. numbered lists, **bold**, [links](https://…)."
          label="Body"
          onChange={(e) => setBody(e.target.value)}
          rows={26}
          value={body}
        />
        <Button disabled={busy || version.trim() === "" || body.trim() === ""} onClick={() => void publish()}>
          {busy ? "Publishing…" : "Publish new version"}
        </Button>
      </Card>

      <Card>
        <h2>Version history — {selected}</h2>
        {history.length === 0 ? (
          <p style={{ color: "var(--gg-color-muted)" }}>No versions published yet.</p>
        ) : (
          history.map((d) => (
            <div
              key={d.version}
              style={{
                borderTop: "1px solid var(--gg-color-border)",
                paddingTop: "0.75rem",
                marginTop: "0.75rem",
                display: "flex",
                gap: "1rem",
                alignItems: "center",
                justifyContent: "space-between",
                flexWrap: "wrap",
              }}
            >
              <div>
                <strong>v{d.version}</strong>{" "}
                <Badge tone={d.approved ? "success" : "warning"}>
                  {d.approved ? "Approved" : "Draft"}
                </Badge>
                <div style={{ color: "var(--gg-color-muted)", fontSize: "var(--gg-text-sm)" }}>
                  {d.body ? `${d.body.length.toLocaleString()} characters` : "no body"} ·
                  published {new Date(d.published_at).toLocaleDateString()}
                </div>
              </div>
              {!d.approved ? (
                <Button disabled={busy} onClick={() => void approve(d.document, d.version)} variant="secondary">
                  Approve
                </Button>
              ) : null}
            </div>
          ))
        )}
      </Card>
    </section>
  );
}
