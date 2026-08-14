"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { Alert, Badge, Button, Card } from "@proguidegh/ui";
import { api, ApiError, errorMessage, unwrap } from "../lib/api";
import { stageLabel, statusTone } from "../lib/certification";

/** Assumed shape of GET /me/guide (spec §5, §13.4). */
interface GuideProfile {
  public_name?: string;
  bio?: string;
  region_id?: string;
  languages?: unknown[];
  specialties?: unknown[];
  status?: string;
}

interface CertificationSummary {
  case_id?: string;
  status?: string;
  outstanding?: string[];
}

interface MeGuideResponse {
  profile?: GuideProfile;
  certification?: CertificationSummary;
}

type LoadState = "loading" | "unauthenticated" | "error" | "ready";

/** Rough completeness over the public-profile fields tourists see. */
function profileCompleteness(profile: GuideProfile | null): number {
  if (!profile) return 0;
  const checks = [
    Boolean(profile.public_name),
    Boolean(profile.bio),
    Boolean(profile.region_id),
    Array.isArray(profile.languages) && profile.languages.length > 0,
    Array.isArray(profile.specialties) && profile.specialties.length > 0,
  ];
  return Math.round((checks.filter(Boolean).length / checks.length) * 100);
}

export default function GuideDashboardPage() {
  const [state, setState] = useState<LoadState>("loading");
  const [loadError, setLoadError] = useState<string | null>(null);
  const [profile, setProfile] = useState<GuideProfile | null>(null);
  const [certification, setCertification] =
    useState<CertificationSummary | null>(null);

  async function load() {
    setState("loading");
    setLoadError(null);
    try {
      const data = unwrap<MeGuideResponse>(await api<unknown>("/me/guide"), "guide");
      // Tolerate a bare profile object without the wrapper keys.
      setProfile(data.profile ?? (data as unknown as GuideProfile));
      setCertification(data.certification ?? null);
      setState("ready");
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        setState("unauthenticated");
      } else if (err instanceof ApiError && err.status === 404) {
        // Registered but no guide record yet — still show the dashboard shell.
        setProfile(null);
        setCertification(null);
        setState("ready");
      } else {
        setLoadError(
          errorMessage(err, "Could not load your dashboard. Please retry."),
        );
        setState("error");
      }
    }
  }

  useEffect(() => {
    void load();
  }, []);

  if (state === "loading") {
    return (
      <div className="stack" aria-busy="true" aria-label="Loading your dashboard">
        <div className="gg-skeleton" style={{ height: "2rem", width: "40%" }} />
        <div className="grid grid--cols-3">
          {Array.from({ length: 3 }, (_, i) => (
            <div key={i} className="gg-skeleton" style={{ height: "8rem" }} />
          ))}
        </div>
      </div>
    );
  }

  if (state === "unauthenticated") {
    return (
      <div className="stack">
        <h1>Guide dashboard</h1>
        <Alert tone="info" title="Sign in required">
          <p>Sign in with your guide account to see jobs, tours and earnings.</p>
        </Alert>
        <p className="nav-actions">
          <Link className="gg-button gg-button--primary" href="/login">
            Sign in
          </Link>
          <Link className="gg-button gg-button--secondary" href="/register">
            Register as a guide
          </Link>
        </p>
      </div>
    );
  }

  if (state === "error") {
    return (
      <div className="stack">
        <h1>Guide dashboard</h1>
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

  const certStatus = certification?.status ?? profile?.status ?? null;
  const outstanding = certification?.outstanding ?? [];
  const completeness = profileCompleteness(profile);

  return (
    <div className="stack">
      <section aria-labelledby="dashboard-heading">
        <h1 id="dashboard-heading">
          Guide dashboard{profile?.public_name ? ` — ${profile.public_name}` : ""}
        </h1>
        <p className="muted">
          Your jobs, tours, earnings and training — all in one place.
        </p>
        <Badge tone={certStatus ? statusTone(certStatus) : "warning"}>
          {certStatus
            ? `Certification: ${stageLabel(certStatus)}`
            : "Verification not started"}
        </Badge>
      </section>

      <div className="grid grid--cols-3" aria-label="Key stats">
        <Card title="Open job offers">
          <p className="stat">0</p>
          New offers will appear here with a live countdown.
        </Card>
        <Card title="Upcoming tours">
          <p className="stat">0</p>
          Confirmed tours scheduled for this week.
        </Card>
        <Card title="Wallet balance">
          <p className="stat">GH₵ 0.00</p>
          <Link className="gg-button gg-button--secondary" href="/guide/wallet">
            Open wallet
          </Link>
        </Card>
      </div>

      <section aria-labelledby="next-steps-heading">
        <h2 id="next-steps-heading">Next steps</h2>
        <div className="grid grid--cols-2">
          <Card title="Complete verification">
            {outstanding.length > 0 ? (
              <>
                Outstanding requirements:
                <ul>
                  {outstanding.map((item) => (
                    <li key={item}>{item}</li>
                  ))}
                </ul>
              </>
            ) : (
              "Upload your documents and finish the certification pipeline to start receiving job offers."
            )}
            <p className="nav-actions">
              {certStatus ? (
                <Link
                  className="gg-button gg-button--primary"
                  href="/guide/verification"
                >
                  Continue verification
                </Link>
              ) : (
                <Link className="gg-button gg-button--primary" href="/guide/apply">
                  Start application
                </Link>
              )}
            </p>
          </Card>
          <Card title={`Public profile — ${completeness}% complete`}>
            {completeness < 100
              ? "Tourists see your public name, bio, region, languages and specialties. Fill them in to appear credible in search."
              : "Your public profile is complete."}
            <p className="nav-actions">
              <Link
                className="gg-button gg-button--secondary"
                href="/guide/profile"
              >
                Edit public profile
              </Link>
              <Link
                className="gg-button gg-button--secondary"
                href="/guide/training"
              >
                Training &amp; certificates
              </Link>
            </p>
          </Card>
        </div>
      </section>
    </div>
  );
}
