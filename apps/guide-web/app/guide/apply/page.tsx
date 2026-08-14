"use client";

import { useState } from "react";
import Link from "next/link";
import {
  Alert,
  Button,
  Fieldset,
  Input,
  Select,
  Textarea,
} from "@proguidegh/ui";
import { api, ApiError, errorMessage } from "../../lib/api";

// Placeholder option lists until the catalog/regions API lands (Phase 2).
const REGION_OPTIONS = [
  { value: "", label: "Select your primary region" },
  { value: "greater-accra", label: "Greater Accra" },
  { value: "ashanti", label: "Ashanti" },
  { value: "central", label: "Central" },
  { value: "eastern", label: "Eastern" },
  { value: "western", label: "Western" },
  { value: "western-north", label: "Western North" },
  { value: "volta", label: "Volta" },
  { value: "oti", label: "Oti" },
  { value: "bono", label: "Bono" },
  { value: "bono-east", label: "Bono East" },
  { value: "ahafo", label: "Ahafo" },
  { value: "northern", label: "Northern" },
  { value: "savannah", label: "Savannah" },
  { value: "north-east", label: "North East" },
  { value: "upper-east", label: "Upper East" },
  { value: "upper-west", label: "Upper West" },
];

const LANGUAGE_OPTIONS = [
  "English",
  "Twi",
  "Fante",
  "Ga",
  "Ewe",
  "Dagbani",
  "Hausa",
  "Nzema",
  "French",
];

export default function GuideApplyPage() {
  const [publicName, setPublicName] = useState("");
  const [bio, setBio] = useState("");
  const [region, setRegion] = useState("");
  const [languages, setLanguages] = useState<string[]>([]);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [unauthenticated, setUnauthenticated] = useState(false);
  const [languageError, setLanguageError] = useState<string | undefined>();
  const [done, setDone] = useState(false);

  function toggleLanguage(language: string) {
    setLanguages((prev) =>
      prev.includes(language)
        ? prev.filter((l) => l !== language)
        : [...prev, language],
    );
    setLanguageError(undefined);
  }

  async function onSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError(null);
    if (languages.length === 0) {
      setLanguageError("Select at least one language you guide in.");
      return;
    }
    setPending(true);
    try {
      await api("/guides/apply", {
        method: "POST",
        body: {
          public_name: publicName,
          bio,
          region,
          languages,
        },
      });
      setDone(true);
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        setUnauthenticated(true);
        return;
      }
      setError(
        errorMessage(err, "Could not submit your application. Please try again."),
      );
    } finally {
      setPending(false);
    }
  }

  if (unauthenticated) {
    return (
      <div className="stack">
        <h1>Guide application</h1>
        <Alert tone="info" title="Sign in required">
          <p>You need to sign in with your guide account to apply.</p>
        </Alert>
        <p>
          <Link className="gg-button gg-button--primary" href="/login">
            Sign in
          </Link>{" "}
          <Link className="gg-button gg-button--secondary" href="/register">
            Register as a guide
          </Link>
        </p>
      </div>
    );
  }

  if (done) {
    return (
      <div className="stack">
        <section aria-labelledby="applied-heading">
          <h1 id="applied-heading">Application received</h1>
        </section>
        <Alert tone="success" title="Pipeline opened">
          <p>
            Your application was received and your certification pipeline is
            now open at the &ldquo;Application received&rdquo; stage. Next,
            upload your verification documents so the certification team can
            start reviewing.
          </p>
        </Alert>
        <p className="nav-actions">
          <Link
            className="gg-button gg-button--primary"
            href="/guide/verification"
          >
            Continue to verification
          </Link>
          <Link className="gg-button gg-button--secondary" href="/guide/profile">
            Complete your public profile
          </Link>
        </p>
      </div>
    );
  }

  return (
    <div className="stack" aria-busy={pending}>
      <section aria-labelledby="apply-heading">
        <h1 id="apply-heading">Guide application</h1>
        <p className="muted">
          Tell us who you are and where you guide. Certification happens after
          document review and mandatory training.
        </p>
      </section>

      {error ? (
        <Alert tone="error" title="Application failed">
          <p>{error}</p>
        </Alert>
      ) : null}

      <form className="stack" onSubmit={onSubmit}>
        <Input
          label="Public display name"
          name="public_name"
          type="text"
          hint="Shown to tourists in search results."
          required
          value={publicName}
          onChange={(e) => setPublicName(e.target.value)}
          disabled={pending}
        />
        <Textarea
          label="Bio"
          name="bio"
          hint="Your experience, specialties and what tourists can expect."
          required
          value={bio}
          onChange={(e) => setBio(e.target.value)}
          disabled={pending}
        />
        <Select
          label="Primary region"
          name="region"
          hint="Region options are placeholders pending the catalog service."
          required
          value={region}
          onChange={(e) => setRegion(e.target.value)}
          disabled={pending}
        >
          {REGION_OPTIONS.map((opt) => (
            <option key={opt.value} value={opt.value}>
              {opt.label}
            </option>
          ))}
        </Select>
        <Fieldset
          legend="Languages you guide in"
          error={languageError}
          hint="Select all that apply."
        >
          {LANGUAGE_OPTIONS.map((language) => (
            <label key={language} className="checkbox-row">
              <input
                type="checkbox"
                name="languages"
                value={language}
                checked={languages.includes(language)}
                onChange={() => toggleLanguage(language)}
                disabled={pending}
              />
              {language}
            </label>
          ))}
        </Fieldset>
        <div>
          <Button type="submit" disabled={pending}>
            {pending ? "Submitting…" : "Submit application"}
          </Button>
        </div>
      </form>
    </div>
  );
}
