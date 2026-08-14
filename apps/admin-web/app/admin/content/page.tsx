"use client";

import { useCallback, useEffect, useState } from "react";
import { Alert, Button, Card, Input, Textarea } from "@proguidegh/ui";
import { api, ApiError, errorMessage } from "../../lib/api";
import { Unauthorized } from "../../components/Unauthorized";

/**
 * Marketing site content (Phase M, M-26).
 *
 * Writes the `marketing.site` key through the existing audited settings
 * endpoint, so every edit to public-facing copy is attributable in the same
 * way a role change is. The public website reads the same key and falls back
 * to its built-in launch copy when nothing is published.
 *
 * The editor is field-based for the parts editors change often (hero, stats,
 * contact, FAQ) and raw JSON for the long-form destination entries — a
 * hand-rolled repeater for nested highlight lists would be more surface area
 * than it is worth before anyone has asked for it.
 */

const SETTING_KEY = "marketing.site";

interface Stat {
  value: string;
  label: string;
  source?: string;
}

interface FaqItem {
  question: string;
  answer: string;
}

interface Hero {
  eyebrow: string;
  headline: string;
  subhead: string;
}

interface Contact {
  supportEmail: string;
  privacyEmail: string;
  phone: string;
  address: string;
}

interface Content {
  hero?: Partial<Hero>;
  stats?: { verified?: boolean; items?: Stat[] };
  faq?: FaqItem[];
  contact?: Partial<Contact>;
  destinations?: unknown;
  [key: string]: unknown;
}

type LoadState = "loading" | "unauthorized" | "forbidden" | "error" | "ready";

function asRecord(value: unknown): Record<string, unknown> | null {
  return value !== null && typeof value === "object" ? (value as Record<string, unknown>) : null;
}

export default function ContentPage() {
  const [state, setState] = useState<LoadState>("loading");
  const [content, setContent] = useState<Content>({});
  const [destinationsJson, setDestinationsJson] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

  const load = useCallback(async () => {
    try {
      const data = await api("/admin/settings");
      const list = asRecord(data)?.settings;
      const row = Array.isArray(list)
        ? (list as { key: string; value: unknown }[]).find((s) => s.key === SETTING_KEY)
        : undefined;
      const value = (asRecord(row?.value) ?? {}) as Content;
      setContent(value);
      setDestinationsJson(
        value.destinations ? JSON.stringify(value.destinations, null, 2) : "",
      );
      setState("ready");
      setError(null);
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) setState("unauthorized");
      else if (err instanceof ApiError && err.status === 403) setState("forbidden");
      else {
        setError(errorMessage(err, "Could not load marketing content."));
        setState("error");
      }
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  function setHero(field: keyof Hero, value: string) {
    setContent((c) => ({ ...c, hero: { ...(c.hero ?? {}), [field]: value } }));
  }

  function setContactField(field: keyof Contact, value: string) {
    setContent((c) => ({ ...c, contact: { ...(c.contact ?? {}), [field]: value } }));
  }

  function setStat(index: number, field: keyof Stat, value: string) {
    setContent((c) => {
      const items = [...(c.stats?.items ?? [])];
      items[index] = { ...items[index], [field]: value } as Stat;
      return { ...c, stats: { ...(c.stats ?? {}), items } };
    });
  }

  function setFaq(index: number, field: keyof FaqItem, value: string) {
    setContent((c) => {
      const faq = [...(c.faq ?? [])];
      faq[index] = { ...faq[index], [field]: value } as FaqItem;
      return { ...c, faq };
    });
  }

  async function save() {
    setSaving(true);
    setError(null);
    setNotice(null);
    try {
      let destinations: unknown = content.destinations;
      if (destinationsJson.trim() !== "") {
        try {
          destinations = JSON.parse(destinationsJson);
        } catch {
          setError("Destinations JSON is not valid. Fix it before saving.");
          setSaving(false);
          return;
        }
      }
      await api(`/admin/settings/${encodeURIComponent(SETTING_KEY)}`, {
        method: "PUT",
        body: { value: { ...content, destinations } },
      });
      setNotice(
        "Saved. The website picks this up within five minutes — no deploy needed.",
      );
      await load();
    } catch (err) {
      setError(errorMessage(err, "Could not save marketing content."));
    } finally {
      setSaving(false);
    }
  }

  if (state === "loading") return <p>Loading marketing content…</p>;
  if (state === "unauthorized" || state === "forbidden") return <Unauthorized />;

  const stats = content.stats?.items ?? [];
  const faq = content.faq ?? [];

  return (
    <section className="stack">
      <div>
        <h1>Marketing site content</h1>
        <p style={{ color: "var(--gg-color-muted)" }}>
          Edits here change proguidegh.com. Every save is audited against your account.
          Fields left empty fall back to the site&apos;s built-in launch copy.
        </p>
      </div>

      {error ? <Alert tone="error" title="Problem">{error}</Alert> : null}
      {notice ? <Alert tone="success" title="Saved">{notice}</Alert> : null}

      <Card>
        <h2>Hero</h2>
        <Input
          label="Eyebrow"
          onChange={(e) => setHero("eyebrow", e.target.value)}
          value={content.hero?.eyebrow ?? ""}
        />
        <Textarea
          label="Headline"
          onChange={(e) => setHero("headline", e.target.value)}
          rows={2}
          value={content.hero?.headline ?? ""}
        />
        <Textarea
          label="Subheading"
          onChange={(e) => setHero("subhead", e.target.value)}
          rows={4}
          value={content.hero?.subhead ?? ""}
        />
      </Card>

      <Card>
        <h2>Statistics band</h2>
        <Alert tone="info" title="Publishing our own numbers">
          The site shows externally sourced Ghana tourism figures until someone
          confirms our own. Only tick “verified” once Finance or Marketing has checked
          the figures — Appendix D treats unearned claims as a launch blocker.
        </Alert>
        <label style={{ display: "flex", gap: "0.5rem", alignItems: "center", marginTop: "0.75rem" }}>
          <input
            checked={content.stats?.verified === true}
            onChange={(e) =>
              setContent((c) => ({
                ...c,
                stats: { ...(c.stats ?? {}), verified: e.target.checked },
              }))
            }
            type="checkbox"
          />
          These figures are verified platform numbers
        </label>

        {stats.map((stat, index) => (
          <div key={index} style={{ borderTop: "1px solid var(--gg-color-border)", paddingTop: "0.75rem", marginTop: "0.75rem" }}>
            <Input
              label={`Value ${index + 1}`}
              onChange={(e) => setStat(index, "value", e.target.value)}
              value={stat.value ?? ""}
            />
            <Input
              label="Label"
              onChange={(e) => setStat(index, "label", e.target.value)}
              value={stat.label ?? ""}
            />
            <Input
              label="Source (shown as a citation)"
              onChange={(e) => setStat(index, "source", e.target.value)}
              value={stat.source ?? ""}
            />
          </div>
        ))}
      </Card>

      <Card>
        <h2>Questions</h2>
        {faq.length === 0 ? (
          <p style={{ color: "var(--gg-color-muted)" }}>
            Nothing published — the site is showing its default questions.
          </p>
        ) : null}
        {faq.map((item, index) => (
          <div key={index} style={{ borderTop: "1px solid var(--gg-color-border)", paddingTop: "0.75rem", marginTop: "0.75rem" }}>
            <Input
              label={`Question ${index + 1}`}
              onChange={(e) => setFaq(index, "question", e.target.value)}
              value={item.question ?? ""}
            />
            <Textarea
              label="Answer"
              onChange={(e) => setFaq(index, "answer", e.target.value)}
              rows={4}
              value={item.answer ?? ""}
            />
          </div>
        ))}
        <Button
          onClick={() =>
            setContent((c) => ({ ...c, faq: [...(c.faq ?? []), { question: "", answer: "" }] }))
          }
          variant="secondary"
        >
          Add a question
        </Button>
      </Card>

      <Card>
        <h2>Contact details</h2>
        <Input
          label="Support email"
          onChange={(e) => setContactField("supportEmail", e.target.value)}
          value={content.contact?.supportEmail ?? ""}
        />
        <Input
          label="Privacy email"
          onChange={(e) => setContactField("privacyEmail", e.target.value)}
          value={content.contact?.privacyEmail ?? ""}
        />
        <Input
          label="Phone"
          onChange={(e) => setContactField("phone", e.target.value)}
          value={content.contact?.phone ?? ""}
        />
        <Input
          label="Address"
          onChange={(e) => setContactField("address", e.target.value)}
          value={content.contact?.address ?? ""}
        />
      </Card>

      <Card>
        <h2>Destinations</h2>
        <p style={{ color: "var(--gg-color-muted)" }}>
          Edited as JSON: each entry has slug, city, region, tagline, blurb, highlights
          and bestFor. Leave empty to keep the site&apos;s defaults.
        </p>
        <Textarea
          label="Destinations JSON"
          onChange={(e) => setDestinationsJson(e.target.value)}
          rows={16}
          value={destinationsJson}
        />
      </Card>

      <div>
        <Button disabled={saving} onClick={() => void save()}>
          {saving ? "Saving…" : "Publish changes"}
        </Button>
      </div>
    </section>
  );
}
