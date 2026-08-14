"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { Alert, Button, Input, Select, Textarea } from "@proguidegh/ui";
import { api, ApiError, errorMessage, unwrap } from "../lib/api";

/** Assumed shape of GET/PATCH /me/tourist-profile (spec §4.1). */
interface TouristProfile {
  full_name?: string;
  nationality?: string;
  phone?: string;
  language?: string;
  emergency_contact_name?: string;
  emergency_contact_phone?: string;
  accessibility_needs?: string;
}

const LANGUAGE_OPTIONS = [
  { value: "", label: "Select a language" },
  { value: "en", label: "English" },
  { value: "tw", label: "Twi" },
  { value: "ee", label: "Ewe" },
  { value: "gaa", label: "Ga" },
  { value: "dag", label: "Dagbani" },
  { value: "ha", label: "Hausa" },
  { value: "fr", label: "French" },
];

type LoadState = "loading" | "unauthenticated" | "error" | "ready";

const EMPTY_PROFILE: Required<TouristProfile> = {
  full_name: "",
  nationality: "",
  phone: "",
  language: "",
  emergency_contact_name: "",
  emergency_contact_phone: "",
  accessibility_needs: "",
};

export default function ProfilePage() {
  const [state, setState] = useState<LoadState>("loading");
  const [loadError, setLoadError] = useState<string | null>(null);
  const [isNewProfile, setIsNewProfile] = useState(false);
  const [profile, setProfile] = useState<Required<TouristProfile>>(EMPTY_PROFILE);
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);

  async function load() {
    setState("loading");
    setLoadError(null);
    try {
      const data = await api<unknown>("/me/tourist-profile");
      const parsed = unwrap<TouristProfile>(data, "profile");
      setProfile({ ...EMPTY_PROFILE, ...stripNulls(parsed) });
      setIsNewProfile(false);
      setState("ready");
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        setState("unauthenticated");
      } else if (err instanceof ApiError && err.status === 404) {
        // No profile yet — start with an empty form (PATCH is assumed to upsert).
        setProfile(EMPTY_PROFILE);
        setIsNewProfile(true);
        setState("ready");
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

  function update(field: keyof Required<TouristProfile>, value: string) {
    setProfile((prev) => ({ ...prev, [field]: value }));
    setSaved(false);
  }

  async function onSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSaving(true);
    setSaveError(null);
    setSaved(false);
    try {
      await api("/me/tourist-profile", { method: "PATCH", body: profile });
      setSaved(true);
      setIsNewProfile(false);
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
        <h1>Your profile</h1>
        <Alert tone="info" title="Sign in required">
          <p>You need to sign in to view and edit your tourist profile.</p>
        </Alert>
        <p>
          <Link className="gg-button gg-button--primary" href="/login">
            Sign in
          </Link>{" "}
          <Link className="gg-button gg-button--secondary" href="/register">
            Create an account
          </Link>
        </p>
      </div>
    );
  }

  if (state === "error") {
    return (
      <div className="stack">
        <h1>Your profile</h1>
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
        <h1 id="profile-heading">Your profile</h1>
        <p className="muted">
          Guides and support use these details during your tours.
        </p>
      </section>

      {isNewProfile ? (
        <Alert tone="info" title="Complete your profile">
          <p>
            You have not saved a profile yet. Fill in the details below to
            finish setting up your account.
          </p>
        </Alert>
      ) : null}

      {saveError ? (
        <Alert tone="error" title="Save failed">
          <p>{saveError}</p>
        </Alert>
      ) : null}

      {saved ? (
        <Alert tone="success" title="Profile saved">
          <p>Your profile details were updated.</p>
        </Alert>
      ) : null}

      <form className="stack" onSubmit={onSubmit}>
        <Input
          label="Full name"
          name="full_name"
          type="text"
          autoComplete="name"
          required
          value={profile.full_name}
          onChange={(e) => update("full_name", e.target.value)}
          disabled={saving}
        />
        <Input
          label="Nationality"
          name="nationality"
          type="text"
          autoComplete="country-name"
          value={profile.nationality}
          onChange={(e) => update("nationality", e.target.value)}
          disabled={saving}
        />
        <Input
          label="Phone number"
          name="phone"
          type="tel"
          autoComplete="tel"
          hint="Include the country code, e.g. +233…"
          value={profile.phone}
          onChange={(e) => update("phone", e.target.value)}
          disabled={saving}
        />
        <Select
          label="Preferred language"
          name="language"
          value={profile.language}
          onChange={(e) => update("language", e.target.value)}
          disabled={saving}
        >
          {LANGUAGE_OPTIONS.map((opt) => (
            <option key={opt.value} value={opt.value}>
              {opt.label}
            </option>
          ))}
        </Select>
        <Input
          label="Emergency contact name"
          name="emergency_contact_name"
          type="text"
          value={profile.emergency_contact_name}
          onChange={(e) => update("emergency_contact_name", e.target.value)}
          disabled={saving}
        />
        <Input
          label="Emergency contact phone"
          name="emergency_contact_phone"
          type="tel"
          value={profile.emergency_contact_phone}
          onChange={(e) => update("emergency_contact_phone", e.target.value)}
          disabled={saving}
        />
        <Textarea
          label="Accessibility needs (optional)"
          name="accessibility_needs"
          hint="Anything guides should know — mobility, hearing, vision or other needs."
          value={profile.accessibility_needs}
          onChange={(e) => update("accessibility_needs", e.target.value)}
          disabled={saving}
        />
        <div>
          <Button type="submit" disabled={saving}>
            {saving ? "Saving…" : "Save profile"}
          </Button>
        </div>
      </form>
    </div>
  );
}

/** Drop null values so controlled inputs always receive strings. */
function stripNulls(profile: TouristProfile): TouristProfile {
  const clean: TouristProfile = {};
  for (const [key, value] of Object.entries(profile)) {
    if (typeof value === "string") {
      clean[key as keyof TouristProfile] = value;
    }
  }
  return clean;
}
