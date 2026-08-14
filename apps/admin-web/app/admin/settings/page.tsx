"use client";

import { useCallback, useEffect, useState } from "react";
import { Alert, Badge, Button, Card } from "@proguidegh/ui";
import { api, ApiError, errorMessage } from "../../lib/api";
import { Unauthorized } from "../../components/Unauthorized";

/**
 * Platform configuration (P8-03, P8-04): versioned notification templates
 * (create a new version, activate to supersede) and the system-settings
 * policy editor. Both are audited server-side.
 */

interface Template {
  id: string;
  key: string;
  version: number;
  channel: string;
  subject: string;
  body: string;
  active: boolean;
}

interface Setting {
  key: string;
  value: unknown;
  updated_at: string;
}

type LoadState = "loading" | "unauthorized" | "forbidden" | "error" | "ready";

function asRecord(value: unknown): Record<string, unknown> | null {
  return value !== null && typeof value === "object"
    ? (value as Record<string, unknown>)
    : null;
}

export default function SettingsPage() {
  const [state, setState] = useState<LoadState>("loading");
  const [templates, setTemplates] = useState<Template[]>([]);
  const [settings, setSettings] = useState<Setting[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [busyId, setBusyId] = useState<string | null>(null);

  const [tKey, setTKey] = useState("");
  const [tChannel, setTChannel] = useState("email");
  const [tSubject, setTSubject] = useState("");
  const [tBody, setTBody] = useState("");

  const load = useCallback(async () => {
    try {
      const [templateData, settingData] = await Promise.all([
        api("/admin/notification-templates"),
        api("/admin/settings"),
      ]);
      const list = asRecord(templateData)?.templates;
      setTemplates(Array.isArray(list) ? (list as Template[]) : []);
      const sList = asRecord(settingData)?.settings;
      setSettings(Array.isArray(sList) ? (sList as Setting[]) : []);
      setState("ready");
      setError(null);
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        setState("unauthorized");
      } else if (err instanceof ApiError && err.status === 403) {
        setState("forbidden");
      } else {
        setError(errorMessage(err, "Could not load configuration."));
        setState("error");
      }
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  async function createTemplate(event: React.FormEvent) {
    event.preventDefault();
    setError(null);
    setNotice(null);
    try {
      await api("/admin/notification-templates", {
        method: "POST",
        body: { key: tKey, channel: tChannel, subject: tSubject, body: tBody },
      });
      setNotice(`New version of “${tKey}” created (inactive until activated).`);
      setTKey("");
      setTSubject("");
      setTBody("");
      await load();
    } catch (err) {
      setError(errorMessage(err, "Could not create the template."));
    }
  }

  async function activate(template: Template) {
    setBusyId(template.id);
    setError(null);
    try {
      await api(`/admin/notification-templates/${template.id}/activate`, {
        method: "POST",
        body: {},
      });
      await load();
    } catch (err) {
      setError(errorMessage(err, "Could not activate the template."));
    } finally {
      setBusyId(null);
    }
  }

  async function editSetting(setting: Setting) {
    const current = typeof setting.value === "string"
      ? setting.value
      : JSON.stringify(setting.value);
    const next = window.prompt(`New value for ${setting.key} (JSON or string)`, current);
    if (next === null) return;
    setBusyId(setting.key);
    setError(null);
    try {
      let value: unknown = next;
      try {
        value = JSON.parse(next);
      } catch {
        value = next;
      }
      await api(`/admin/settings/${encodeURIComponent(setting.key)}`, {
        method: "PUT",
        body: { value },
      });
      await load();
    } catch (err) {
      setError(errorMessage(err, "Could not save the setting."));
    } finally {
      setBusyId(null);
    }
  }

  if (state === "unauthorized" || state === "forbidden") {
    return <Unauthorized />;
  }

  return (
    <div className="stack">
      <section aria-labelledby="settings-heading">
        <h1 id="settings-heading">Platform configuration</h1>
        <p className="muted">
          Versioned notification templates and system settings. Changes are
          audited (spec §1.2).
        </p>
      </section>

      {notice ? <Alert tone="success">{notice}</Alert> : null}
      {error ? <Alert tone="error">{error}</Alert> : null}

      <Card title="New template version">
        <form className="stack" onSubmit={(e) => void createTemplate(e)}>
          <label>
            Key
            <input value={tKey} onChange={(e) => setTKey(e.target.value)} required placeholder="booking.confirmed" />
          </label>
          <label>
            Channel
            <select value={tChannel} onChange={(e) => setTChannel(e.target.value)}>
              <option value="email">email</option>
              <option value="sms">sms</option>
              <option value="push">push</option>
              <option value="in_app">in_app</option>
            </select>
          </label>
          <label>
            Subject
            <input value={tSubject} onChange={(e) => setTSubject(e.target.value)} />
          </label>
          <label>
            Body
            <textarea value={tBody} onChange={(e) => setTBody(e.target.value)} required rows={3} />
          </label>
          <p>
            <Button variant="primary">Create version</Button>
          </p>
        </form>
      </Card>

      <Card title={`Notification templates (${templates.length})`}>
        {templates.length === 0 ? (
          <p className="muted">No templates yet.</p>
        ) : (
          <ul className="stack" aria-label="Templates">
            {templates.map((template) => (
              <li key={template.id} className="stack">
                <p>
                  <Badge tone={template.active ? "success" : "neutral"}>
                    {template.active ? "Active" : "Inactive"}
                  </Badge>{" "}
                  <strong>{template.key}</strong> v{template.version} · {template.channel}
                  {template.subject ? ` · ${template.subject}` : ""}
                </p>
                <p className="muted">{template.body}</p>
                {!template.active ? (
                  <p>
                    <Button
                      variant="secondary"
                      disabled={busyId === template.id}
                      onClick={() => void activate(template)}
                    >
                      Activate
                    </Button>
                  </p>
                ) : null}
              </li>
            ))}
          </ul>
        )}
      </Card>

      <Card title={`System settings (${settings.length})`}>
        {settings.length === 0 ? (
          <p className="muted">No settings.</p>
        ) : (
          <ul className="stack" aria-label="Settings">
            {settings.map((setting) => (
              <li key={setting.key}>
                <p>
                  <strong>{setting.key}</strong>:{" "}
                  <code>
                    {typeof setting.value === "string"
                      ? setting.value
                      : JSON.stringify(setting.value)}
                  </code>{" "}
                  <Button
                    variant="secondary"
                    disabled={busyId === setting.key}
                    onClick={() => void editSetting(setting)}
                  >
                    Edit
                  </Button>
                </p>
              </li>
            ))}
          </ul>
        )}
      </Card>
    </div>
  );
}
