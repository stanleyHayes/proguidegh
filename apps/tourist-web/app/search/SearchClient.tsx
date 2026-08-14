"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import Link from "next/link";
import { Alert, Badge, Button, Card, Input, Select } from "@proguidegh/ui";
import { api, errorMessage } from "../lib/api";
import {
  LANGUAGE_OPTIONS,
  formatDuration,
  formatPrice,
  packageName,
  parseNamedOptions,
  parsePackages,
  type NamedOption,
  type TourPackage,
} from "../lib/catalog";
import {
  guideName,
  labelOf,
  labelsOf,
  localToIso,
  parseGuides,
  type Guide,
} from "../lib/bookings";

type LoadState = "loading" | "error" | "ready";

interface Filters {
  regionId: string;
  specialtyId: string;
  language: string;
  minRating: string;
  elite: boolean;
  at: string; // datetime-local value
}

const EMPTY_FILTERS: Filters = {
  regionId: "",
  specialtyId: "",
  language: "",
  minRating: "",
  elite: false,
  at: "",
};

const MIN_RATING_OPTIONS = [
  { value: "", label: "Any rating" },
  { value: "3", label: "3+ stars" },
  { value: "4", label: "4+ stars" },
  { value: "4.5", label: "4.5+ stars" },
];

function ratingLabel(guide: Guide): string {
  if (!guide.rating_avg || guide.rating_avg <= 0) return "No ratings yet";
  const avg = guide.rating_avg.toFixed(1);
  const count = guide.rating_count ?? 0;
  return `Rated ${avg} out of 5 from ${count} review${count === 1 ? "" : "s"}`;
}

export default function SearchClient({
  initialDestination,
  initialDate,
}: {
  initialDestination: string;
  initialDate: string;
}) {
  const [filters, setFilters] = useState<Filters>(() => ({
    ...EMPTY_FILTERS,
    // Landing form submits a date only; default to a 09:00 start.
    at: initialDate ? `${initialDate}T09:00` : "",
  }));
  const [regions, setRegions] = useState<NamedOption[]>([]);
  const [specialties, setSpecialties] = useState<NamedOption[]>([]);

  const [guidesState, setGuidesState] = useState<LoadState>("loading");
  const [guidesError, setGuidesError] = useState<string | null>(null);
  const [guides, setGuides] = useState<Guide[]>([]);
  const [searched, setSearched] = useState(false);

  const [packagesState, setPackagesState] = useState<LoadState>("loading");
  const [packagesError, setPackagesError] = useState<string | null>(null);
  const [packages, setPackages] = useState<TourPackage[]>([]);

  const destinationApplied = useRef(false);

  const searchGuides = useCallback(async (active: Filters) => {
    setGuidesState("loading");
    setGuidesError(null);
    try {
      const params = new URLSearchParams();
      if (active.regionId) params.set("region_id", active.regionId);
      if (active.specialtyId) params.set("specialty", active.specialtyId);
      if (active.language) params.set("language", active.language);
      if (active.minRating) params.set("min_rating", active.minRating);
      if (active.elite) params.set("elite", "true");
      const iso = localToIso(active.at);
      if (iso) params.set("date", iso);
      const query = params.toString();
      const data = await api<unknown>(
        `/guides/search${query ? `?${query}` : ""}`,
      );
      setGuides(parseGuides(data));
      setGuidesState("ready");
      setSearched(true);
    } catch (err) {
      setGuidesError(
        errorMessage(err, "Could not search guides. Please retry."),
      );
      setGuidesState("error");
    }
  }, []);

  const loadPackages = useCallback(async () => {
    setPackagesState("loading");
    setPackagesError(null);
    try {
      const data = await api<unknown>("/tour-packages");
      setPackages(parsePackages(data));
      setPackagesState("ready");
    } catch (err) {
      setPackagesError(
        errorMessage(err, "Could not load the tour catalog. Please retry."),
      );
      setPackagesState("error");
    }
  }, []);

  // Reference data and first unfiltered search.
  useEffect(() => {
    void api<unknown>("/regions")
      .then((data) => parseNamedOptions(data, ["regions", "items"]))
      .then((options) => {
        setRegions(options);
        // Match the landing-page destination text to a region, once.
        if (!destinationApplied.current && initialDestination) {
          destinationApplied.current = true;
          const needle = initialDestination.trim().toLowerCase();
          const match = options.find((o) =>
            o.name.toLowerCase().includes(needle),
          );
          if (match) {
            setFilters((prev) => {
              const next = { ...prev, regionId: match.id };
              void searchGuides(next);
              return next;
            });
          }
        }
      })
      .catch(() => {
        // Filters degrade gracefully — the selects just stay empty.
      });
    void api<unknown>("/specialties")
      .then((data) => parseNamedOptions(data, ["specialties", "items"]))
      .then(setSpecialties)
      .catch(() => {});
    void searchGuides({
      ...EMPTY_FILTERS,
      at: initialDate ? `${initialDate}T09:00` : "",
    });
    void loadPackages();
  }, []);

  function update<K extends keyof Filters>(key: K, value: Filters[K]) {
    setFilters((prev) => ({ ...prev, [key]: value }));
  }

  function onSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    void searchGuides(filters);
  }

  function onClear() {
    setFilters(EMPTY_FILTERS);
    void searchGuides(EMPTY_FILTERS);
  }

  return (
    <div className="stack">
      <section aria-labelledby="search-heading">
        <h1 id="search-heading">Find your guide</h1>
        <p className="muted">
          Every guide in these results is certified and verified. Filter by
          region, specialty, language, rating or date.
        </p>
      </section>

      <form
        role="search"
        aria-label="Filter guides"
        className="stack"
        onSubmit={onSubmit}
      >
        <div className="filter-grid">
          <Select
            label="Region"
            name="region_id"
            value={filters.regionId}
            onChange={(e) => update("regionId", e.target.value)}
          >
            <option value="">All regions</option>
            {regions.map((region) => (
              <option key={region.id} value={region.id}>
                {region.name}
              </option>
            ))}
          </Select>
          <Select
            label="Specialty"
            name="specialty"
            value={filters.specialtyId}
            onChange={(e) => update("specialtyId", e.target.value)}
          >
            <option value="">All specialties</option>
            {specialties.map((specialty) => (
              <option key={specialty.id} value={specialty.id}>
                {specialty.name}
              </option>
            ))}
          </Select>
          <Select
            label="Language"
            name="language"
            value={filters.language}
            onChange={(e) => update("language", e.target.value)}
          >
            <option value="">Any language</option>
            {LANGUAGE_OPTIONS.map((opt) => (
              <option key={opt.value} value={opt.value}>
                {opt.label}
              </option>
            ))}
          </Select>
          <Select
            label="Minimum rating"
            name="min_rating"
            value={filters.minRating}
            onChange={(e) => update("minRating", e.target.value)}
          >
            {MIN_RATING_OPTIONS.map((opt) => (
              <option key={opt.value} value={opt.value}>
                {opt.label}
              </option>
            ))}
          </Select>
          <Input
            label="Date & time"
            name="date"
            type="datetime-local"
            value={filters.at}
            onChange={(e) => update("at", e.target.value)}
          />
          <div className="field">
            <span className="gg-field__label" id="elite-label">
              Elite guides
            </span>
            <label className="checkbox-row" htmlFor="elite">
              <input
                id="elite"
                name="elite"
                type="checkbox"
                aria-describedby="elite-label"
                checked={filters.elite}
                onChange={(e) => update("elite", e.target.checked)}
              />
              Elite status only
            </label>
          </div>
        </div>
        <div className="nav-actions">
          <Button type="submit">Search guides</Button>
          <Button type="button" variant="secondary" onClick={onClear}>
            Clear filters
          </Button>
        </div>
      </form>

      <section aria-labelledby="results-heading" className="stack">
        <h2 id="results-heading">Guides</h2>

        {guidesState === "loading" ? (
          <div
            className="grid grid--cols-3"
            aria-busy="true"
            aria-label="Loading guides"
          >
            {Array.from({ length: 6 }, (_, i) => (
              <div key={i} className="gg-skeleton" style={{ height: "11rem" }} />
            ))}
          </div>
        ) : null}

        {guidesState === "error" ? (
          <>
            <Alert tone="error" title="Search failed">
              <p>{guidesError}</p>
            </Alert>
            <div>
              <Button type="button" onClick={() => void searchGuides(filters)}>
                Retry
              </Button>
            </div>
          </>
        ) : null}

        {guidesState === "ready" && guides.length === 0 ? (
          <Alert tone="info" title="No guides match">
            <p>
              {searched
                ? "No guides match these filters right now — widen your filters or try a different date."
                : "No guides are available yet. Check back soon — certified guides are being onboarded."}
            </p>
          </Alert>
        ) : null}

        {guidesState === "ready" && guides.length > 0 ? (
          <div className="grid grid--cols-3" aria-label="Matching guides">
            {guides.map((guide) => {
              const languages = labelsOf(guide.languages);
              const specialtyLabels = labelsOf(guide.specialties);
              const region = labelOf(guide.region);
              return (
                <Card key={guide.id} title={guideName(guide)}>
                  <div className="stack" style={{ gap: "var(--gg-space-3)" }}>
                    <div className="badge-row">
                      <Badge tone="success">Verified</Badge>
                      {guide.elite_status ? (
                        <Badge tone="warning">Elite guide</Badge>
                      ) : null}
                    </div>
                    <p className="stars" aria-label={ratingLabel(guide)}>
                      <span aria-hidden="true">★</span>{" "}
                      {guide.rating_avg && guide.rating_avg > 0
                        ? guide.rating_avg.toFixed(1)
                        : "—"}{" "}
                      <span className="muted">
                        ({guide.rating_count ?? 0} reviews)
                      </span>
                    </p>
                    {region ? <p className="muted">{region}</p> : null}
                    {languages.length > 0 ? (
                      <p className="muted">Speaks {languages.join(", ")}</p>
                    ) : null}
                    {specialtyLabels.length > 0 ? (
                      <div className="badge-row">
                        {specialtyLabels.map((specialty) => (
                          <Badge key={specialty} tone="neutral">
                            {specialty}
                          </Badge>
                        ))}
                      </div>
                    ) : null}
                    <p className="muted">
                      {guide.completed_tours ?? 0} completed tours
                    </p>
                    <p>
                      <Link
                        className="gg-button gg-button--secondary"
                        href={`/guides/${guide.id}`}
                      >
                        View guide
                      </Link>
                    </p>
                  </div>
                </Card>
              );
            })}
          </div>
        ) : null}
      </section>

      <section aria-labelledby="packages-heading" className="stack">
        <h2 id="packages-heading">Tour packages</h2>
        <p className="muted">
          Official tour packages with transparent pricing. Every tour is led by
          a certified guide.
        </p>

        {packagesState === "loading" ? (
          <div
            className="grid grid--cols-3"
            aria-busy="true"
            aria-label="Loading tour packages"
          >
            {Array.from({ length: 3 }, (_, i) => (
              <div key={i} className="gg-skeleton" style={{ height: "9rem" }} />
            ))}
          </div>
        ) : null}

        {packagesState === "error" ? (
          <>
            <Alert tone="error" title="Something went wrong">
              <p>{packagesError}</p>
            </Alert>
            <div>
              <Button type="button" onClick={() => void loadPackages()}>
                Retry
              </Button>
            </div>
          </>
        ) : null}

        {packagesState === "ready" && packages.length === 0 ? (
          <Alert tone="info" title="No packages yet">
            <p>
              The tour catalog is empty right now. Check back soon — certified
              guides are being onboarded.
            </p>
          </Alert>
        ) : null}

        {packagesState === "ready" && packages.length > 0 ? (
          <div className="grid grid--cols-3" aria-label="Tour packages">
            {packages.map((pkg) => (
              <Card key={pkg.id} title={packageName(pkg)}>
                <p>
                  <strong>{formatPrice(pkg.base_price, pkg.currency)}</strong>
                </p>
                <p className="muted">{formatDuration(pkg.duration_minutes)}</p>
                <Badge tone="neutral">Certified guide included</Badge>
              </Card>
            ))}
          </div>
        ) : null}
      </section>
    </div>
  );
}
