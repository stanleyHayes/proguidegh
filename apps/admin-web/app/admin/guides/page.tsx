"use client";

import { useEffect, useState } from "react";
import { Alert, Badge, Button } from "@proguidegh/ui";
import { api, ApiError, errorMessage } from "../../lib/api";
import { Unauthorized } from "../../components/Unauthorized";

/** Assumed shape of GET /admin/guides entries (spec §13.6). */
interface AdminGuide {
  id: string;
  public_name?: string;
  email?: string;
  region?: string;
  status?: string;
  languages?: string[];
  created_at?: string;
}

type LoadState = "loading" | "unauthorized" | "forbidden" | "error" | "ready";

function parseGuides(data: unknown): AdminGuide[] {
  if (Array.isArray(data)) return data as AdminGuide[];
  if (data !== null && typeof data === "object" && "guides" in data) {
    const guides = (data as { guides: unknown }).guides;
    if (Array.isArray(guides)) return guides as AdminGuide[];
  }
  return [];
}

function formatDate(value?: string): string {
  if (!value) return "—";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleDateString();
}

function statusTone(status?: string): "neutral" | "success" | "warning" | "danger" {
  switch (status) {
    case "active":
    case "certified":
      return "success";
    case "pending":
    case "in_review":
      return "warning";
    case "rejected":
    case "suspended":
      return "danger";
    default:
      return "neutral";
  }
}

export default function AdminGuidesPage() {
  const [state, setState] = useState<LoadState>("loading");
  const [loadError, setLoadError] = useState<string | null>(null);
  const [guides, setGuides] = useState<AdminGuide[]>([]);

  async function load() {
    setState("loading");
    setLoadError(null);
    try {
      const data = await api<unknown>("/admin/guides");
      setGuides(parseGuides(data));
      setState("ready");
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        setState("unauthorized");
      } else if (err instanceof ApiError && err.status === 403) {
        setState("forbidden");
      } else {
        setLoadError(errorMessage(err, "Could not load guides. Please retry."));
        setState("error");
      }
    }
  }

  useEffect(() => {
    void load();
  }, []);

  if (state === "unauthorized" || state === "forbidden") {
    return (
      <div className="stack">
        <h1>Guide directory</h1>
        <Unauthorized forbidden={state === "forbidden"} />
      </div>
    );
  }

  return (
    <div className="stack">
      <section aria-labelledby="guides-heading">
        <h1 id="guides-heading">Guide directory</h1>
        <p className="muted">
          All guide applicants and certified guides. Certification queues get
          their own screen in a later phase.
        </p>
      </section>

      {state === "error" ? (
        <>
          <Alert tone="error" title="Something went wrong">
            <p>{loadError}</p>
          </Alert>
          <div>
            <Button type="button" onClick={() => void load()}>
              Retry
            </Button>
          </div>
        </>
      ) : null}

      {state === "loading" ? (
        <div className="stack" aria-busy="true" aria-label="Loading guides">
          {Array.from({ length: 5 }, (_, i) => (
            <div key={i} className="gg-skeleton" style={{ height: "3rem" }} />
          ))}
        </div>
      ) : null}

      {state === "ready" && guides.length === 0 ? (
        <Alert tone="info" title="No guides yet">
          <p>
            Applications submitted through the guide app will appear here.
          </p>
        </Alert>
      ) : null}

      {state === "ready" && guides.length > 0 ? (
        <div className="gg-table-scroll">
          <table className="gg-table">
            <thead>
              <tr>
                <th scope="col">Public name</th>
                <th scope="col">Email</th>
                <th scope="col">Region</th>
                <th scope="col">Status</th>
                <th scope="col">Applied</th>
              </tr>
            </thead>
            <tbody>
              {guides.map((guide) => (
                <tr key={guide.id}>
                  <td>{guide.public_name ?? "—"}</td>
                  <td>{guide.email ?? "—"}</td>
                  <td>{guide.region ?? "—"}</td>
                  <td>
                    <Badge tone={statusTone(guide.status)}>
                      {guide.status ?? "unknown"}
                    </Badge>
                  </td>
                  <td>{formatDate(guide.created_at)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : null}
    </div>
  );
}
