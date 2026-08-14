"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import {
  Alert,
  Button,
  Fieldset,
  Input,
  Select,
  Textarea,
} from "@proguidegh/ui";
import { api, ApiError, errorMessage, unwrap } from "../../lib/api";

/** Assumed shapes (spec §13.4); backend is built in parallel, so stay tolerant. */
interface GuideProfile {
  public_name?: string;
  bio?: string;
  region_id?: string;
  languages?: { code?: string; proficiency?: string }[];
  specialties?: (string | { id?: string; code?: string })[];
}

interface Region {
  id: string;
  code?: string;
  name?: string;
}

interface Specialty {
  id: string;
  code?: string;
  name?: string;
}

interface LanguageEntry {
  code: string;
  proficiency: string;
}

const LANGUAGE_OPTIONS = [
  { code: "en", label: "English" },
  { code: "tw", label: "Twi" },
  { code: "fat", label: "Fante" },
  { code: "gaa", label: "Ga" },
  { code: "ee", label: "Ewe" },
  { code: "dag", label: "Dagbani" },
  { code: "ha", label: "Hausa" },
  { code: "fr", label: "French" },
];

const PROFICIENCY_OPTIONS = [
  { value: "basic", label: "Basic" },
  { value: "conversational", label: "Conversational" },
  { value: "fluent", label: "Fluent" },
  { value: "native", label: "Native" },
];

type LoadState = "loading" | "unauthenticated" | "error" | "ready";

function parseList<T>(data: unknown, key: string): T[] {
  if (Array.isArray(data)) return data as T[];
  if (data !== null && typeof data === "object" && key in data) {
    const list = (data as Record<string, unknown>)[key];
    if (Array.isArray(list)) return list as T[];
  }
  return [];
}

/** Specialties may come back as ids or objects — normalize to ids. */
function parseSpecialtyIds(
  raw: GuideProfile["specialties"],
  catalog: Specialty[],
): string[] {
  if (!Array.isArray(raw)) return [];
  const ids: string[] = [];
  for (const entry of raw) {
    if (typeof entry === "string") {
      // Could be an id or a code; map codes through the catalog when possible.
      const match = catalog.find((s) => s.id === entry || s.code === entry);
      ids.push(match?.id ?? entry);
    } else if (entry && typeof entry === "object") {
      const id = entry.id ?? catalog.find((s) => s.code === entry.code)?.id;
      if (id) ids.push(id);
    }
  }
  return ids;
}

export default function GuideProfilePage() {
  const [state, setState] = useState<LoadState>("loading");
  const [loadError, setLoadError] = useState<string | null>(null);
  const [regions, setRegions] = useState<Region[]>([]);
  const [specialtyCatalog, setSpecialtyCatalog] = useState<Specialty[]>([]);
  const [catalogWarning, setCatalogWarning] = useState<string | null>(null);

  const [publicName, setPublicName] = useState("");
  const [bio, setBio] = useState("");
  const [regionId, setRegionId] = useState("");
  const [languages, setLanguages] = useState<LanguageEntry[]>([]);
  const [specialtyIds, setSpecialtyIds] = useState<string[]>([]);

  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);
  const [languageError, setLanguageError] = useState<string | undefined>();

  async function load() {
    setState("loading");
    setLoadError(null);
    setCatalogWarning(null);
    try {
      const data = await api<unknown>("/me/guide");
      const profile = unwrap<GuideProfile>(data, "profile");

      // Catalogs are non-fatal: the form still works without them.
      let regionList: Region[] = [];
      let specialtyList: Specialty[] = [];
      const catalogResults = await Promise.allSettled([
        api<unknown>("/regions"),
        api<unknown>("/specialties"),
      ]);
      const [regionResult, specialtyResult] = catalogResults;
      if (regionResult.status === "fulfilled") {
        regionList = parseList<Region>(regionResult.value, "regions");
      }
      if (specialtyResult.status === "fulfilled") {
        specialtyList = parseList<Specialty>(
          specialtyResult.value,
          "specialties",
        );
      }
      if (catalogResults.some((r) => r.status === "rejected")) {
        setCatalogWarning(
          "Could not load the region/specialty catalogs. You can still save other fields.",
        );
      }

      setRegions(regionList);
      setSpecialtyCatalog(specialtyList);
      setPublicName(profile.public_name ?? "");
      setBio(profile.bio ?? "");
      setRegionId(profile.region_id ?? "");
      setLanguages(
        Array.isArray(profile.languages)
          ? profile.languages
              .filter((l) => l && typeof l.code === "string")
              .map((l) => ({
                code: l.code as string,
                proficiency: l.proficiency ?? "fluent",
              }))
          : [],
      );
      setSpecialtyIds(parseSpecialtyIds(profile.specialties, specialtyList));
      setState("ready");
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        setState("unauthenticated");
      } else {
        setLoadError(
          errorMessage(err, "Could not load your profile. Please retry."),
        );
        setState("error");
      }
    }
  }

  useEffect(() => {
    void load();
  }, []);

  function markDirty() {
    setSaved(false);
  }

  function toggleLanguage(code: string) {
    setLanguages((prev) =>
      prev.some((l) => l.code === code)
        ? prev.filter((l) => l.code !== code)
        : [...prev, { code, proficiency: "fluent" }],
    );
    setLanguageError(undefined);
    markDirty();
  }

  function setProficiency(code: string, proficiency: string) {
    setLanguages((prev) =>
      prev.map((l) => (l.code === code ? { ...l, proficiency } : l)),
    );
    markDirty();
  }

  function toggleSpecialty(id: string) {
    setSpecialtyIds((prev) =>
      prev.includes(id) ? prev.filter((s) => s !== id) : [...prev, id],
    );
    markDirty();
  }

  async function onSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSaveError(null);
    setSaved(false);
    if (languages.length === 0) {
      setLanguageError("Select at least one language you guide in.");
      return;
    }
    setSaving(true);
    try {
      await api("/me/guide/profile", {
        method: "PATCH",
        body: {
          public_name: publicName,
          bio,
          region_id: regionId,
          languages,
          specialties: specialtyIds,
        },
      });
      setSaved(true);
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        setState("unauthenticated");
        return;
      }
      setSaveError(
        errorMessage(err, "Could not save your profile. Please try again."),
      );
    } finally {
      setSaving(false);
    }
  }

  if (state === "loading") {
    return (
      <div className="stack" aria-busy="true" aria-label="Loading your profile">
        <div className="gg-skeleton" style={{ height: "2rem", width: "40%" }} />
        {Array.from({ length: 5 }, (_, i) => (
          <div key={i} className="gg-skeleton" style={{ height: "2.75rem" }} />
        ))}
      </div>
    );
  }

  if (state === "unauthenticated") {
    return (
      <div className="stack">
        <h1>Public profile</h1>
        <Alert tone="info" title="Sign in required">
          <p>Sign in with your guide account to edit your public profile.</p>
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
        <h1>Public profile</h1>
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

  return (
    <div className="stack" aria-busy={saving}>
      <section aria-labelledby="profile-heading">
        <h1 id="profile-heading">Public profile</h1>
        <p className="muted">
          This is what tourists see in search results. Keep it accurate and up
          to date.
        </p>
      </section>

      {catalogWarning ? (
        <Alert tone="info" title="Catalog unavailable">
          <p>{catalogWarning}</p>
        </Alert>
      ) : null}

      {saveError ? (
        <Alert tone="error" title="Save failed">
          <p>{saveError}</p>
        </Alert>
      ) : null}

      {saved ? (
        <Alert tone="success" title="Saved">
          <p>Your public profile was updated.</p>
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
          onChange={(e) => {
            setPublicName(e.target.value);
            markDirty();
          }}
          disabled={saving}
        />
        <Textarea
          label="Bio"
          name="bio"
          hint="Your experience, specialties and what tourists can expect."
          value={bio}
          onChange={(e) => {
            setBio(e.target.value);
            markDirty();
          }}
          disabled={saving}
        />
        <Select
          label="Primary region"
          name="region_id"
          required
          value={regionId}
          onChange={(e) => {
            setRegionId(e.target.value);
            markDirty();
          }}
          disabled={saving}
        >
          <option value="" disabled>
            Select your primary region
          </option>
          {regions.map((region) => (
            <option key={region.id} value={region.id}>
              {region.name ?? region.code ?? region.id}
            </option>
          ))}
          {regionId &&
          !regions.some((r) => r.id === regionId) ? (
            <option value={regionId}>{regionId} (current)</option>
          ) : null}
        </Select>

        <Fieldset
          legend="Languages you guide in"
          error={languageError}
          hint="Select all that apply and set your proficiency."
        >
          {LANGUAGE_OPTIONS.map((language) => {
            const selected = languages.find((l) => l.code === language.code);
            return (
              <div key={language.code} className="stack">
                <label className="checkbox-row">
                  <input
                    type="checkbox"
                    name="languages"
                    value={language.code}
                    checked={Boolean(selected)}
                    onChange={() => toggleLanguage(language.code)}
                    disabled={saving}
                  />
                  {language.label}
                </label>
                {selected ? (
                  <Select
                    label={`${language.label} proficiency`}
                    name={`proficiency_${language.code}`}
                    value={selected.proficiency}
                    onChange={(e) =>
                      setProficiency(language.code, e.target.value)
                    }
                    disabled={saving}
                  >
                    {PROFICIENCY_OPTIONS.map((opt) => (
                      <option key={opt.value} value={opt.value}>
                        {opt.label}
                      </option>
                    ))}
                  </Select>
                ) : null}
              </div>
            );
          })}
        </Fieldset>

        <Fieldset
          legend="Specialties"
          hint={
            specialtyCatalog.length > 0
              ? "Select the tour specialties you offer."
              : "Specialty catalog unavailable — previously saved specialties are kept on save."
          }
        >
          {specialtyCatalog.length === 0 ? (
            <p className="muted">No specialties to choose from right now.</p>
          ) : (
            specialtyCatalog.map((specialty) => (
              <label key={specialty.id} className="checkbox-row">
                <input
                  type="checkbox"
                  name="specialties"
                  value={specialty.id}
                  checked={specialtyIds.includes(specialty.id)}
                  onChange={() => toggleSpecialty(specialty.id)}
                  disabled={saving}
                />
                {specialty.name ?? specialty.code ?? specialty.id}
              </label>
            ))
          )}
        </Fieldset>

        <div className="nav-actions">
          <Button type="submit" disabled={saving}>
            {saving ? "Saving…" : "Save profile"}
          </Button>
          <Link className="gg-button gg-button--secondary" href="/guide">
            Back to dashboard
          </Link>
        </div>
      </form>
    </div>
  );
}
