/**
 * Guide search (Phase M, M-09) — parity with tourist-web /search:
 * region/specialty/language/min-rating/elite filters against
 * GET /guides/search (§10.1–10.2). Package catalog from /tour-packages
 * shown above results.
 */
import { useCallback, useEffect, useState } from "react";
import { FlatList, StyleSheet, Text, View } from "react-native";
import { useRouter } from "expo-router";
import { colors, fontSize, space } from "@proguidegh/tokens";
import { useSession, errorMessage } from "@/lib/session";
import {
  formatDuration,
  formatPrice,
  formatRating,
  parseGuides,
  parseNamedOptions,
  parsePackages,
  type GuideSummary,
  type NamedOption,
  type TourPackage,
} from "@/lib/catalog";
import {
  Badge,
  Card,
  ChipSelect,
  EmptyState,
  ErrorState,
  LoadingState,
} from "@/lib/ui";

const LANGUAGE_OPTIONS: NamedOption[] = [
  { id: "en", name: "English" },
  { id: "fr", name: "French" },
  { id: "tw", name: "Twi" },
  { id: "ee", name: "Ewe" },
  { id: "ga", name: "Ga" },
  { id: "zh", name: "Mandarin" },
];

export default function SearchScreen() {
  const { client } = useSession();
  const router = useRouter();
  const [regions, setRegions] = useState<NamedOption[]>([]);
  const [specialties, setSpecialties] = useState<NamedOption[]>([]);
  const [packages, setPackages] = useState<TourPackage[]>([]);
  const [regionId, setRegionId] = useState<string | null>(null);
  const [specialty, setSpecialty] = useState<string | null>(null);
  const [language, setLanguage] = useState<string | null>(null);
  const [minRating, setMinRating] = useState<string | null>(null);
  const [eliteOnly, setEliteOnly] = useState(false);
  const [guides, setGuides] = useState<GuideSummary[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    Promise.allSettled([
      client.api("/regions"),
      client.api("/specialties"),
      client.api("/tour-packages"),
    ]).then(([r, s, p]) => {
      if (cancelled) return;
      if (r.status === "fulfilled") setRegions(parseNamedOptions(r.value, "regions"));
      if (s.status === "fulfilled") setSpecialties(parseNamedOptions(s.value, "specialties"));
      if (p.status === "fulfilled") setPackages(parsePackages(p.value));
    });
    return () => {
      cancelled = true;
    };
  }, [client]);

  const search = useCallback(async () => {
    setError(null);
    setGuides(null);
    try {
      const params = new URLSearchParams();
      if (regionId) params.set("region_id", regionId);
      if (specialty) params.set("specialty", specialty);
      if (language) params.set("language", language);
      if (minRating) params.set("min_rating", minRating);
      if (eliteOnly) params.set("elite", "true");
      const qs = params.toString();
      const data = await client.api(`/guides/search${qs ? `?${qs}` : ""}`);
      setGuides(parseGuides(data));
    } catch (err) {
      setError(errorMessage(err, "Search failed. Check your connection."));
      setGuides([]);
    }
  }, [client, regionId, specialty, language, minRating, eliteOnly]);

  useEffect(() => {
    // Deferred to avoid synchronous setState inside the effect body.
    const t = setTimeout(() => void search(), 0);
    return () => clearTimeout(t);
  }, [search]);

  return (
    <FlatList
      contentContainerStyle={styles.page}
      data={guides}
      keyExtractor={(g) => g.userId}
      ListHeaderComponent={
        <View style={styles.header}>
          <Text style={styles.heading}>Find your guide</Text>

          {packages.length > 0 ? (
            <View style={styles.packages}>
              {packages.map((p) => (
                <Card key={p.id}>
                  <Text style={styles.packageName}>{p.name}</Text>
                  <Text style={styles.muted}>
                    {formatDuration(p.durationMinutes)} · {formatPrice(p.basePrice, p.currency)}
                  </Text>
                </Card>
              ))}
            </View>
          ) : null}

          <ChipSelect label="Region" options={regions} value={regionId} onChange={setRegionId} />
          <ChipSelect label="Specialty" options={specialties} value={specialty} onChange={setSpecialty} />
          <ChipSelect label="Language" options={LANGUAGE_OPTIONS} value={language} onChange={setLanguage} />
          <ChipSelect
            label="Minimum rating"
            options={[
              { id: "4.5", name: "4.5+" },
              { id: "4.0", name: "4.0+" },
              { id: "3.0", name: "3.0+" },
            ]}
            value={minRating}
            onChange={setMinRating}
          />
          <ChipSelect
            label="Elite guides"
            options={[{ id: "elite", name: "Elite only" }]}
            value={eliteOnly ? "elite" : null}
            onChange={(v) => setEliteOnly(v !== null)}
          />
        </View>
      }
      ListEmptyComponent={
        guides === null && !error ? (
          <LoadingState label="Searching certified guides…" />
        ) : error ? (
          <ErrorState message={error} onRetry={() => void search()} />
        ) : (
          <EmptyState
            title="No guides match"
            body="Widen your filters or try another region."
          />
        )
      }
      renderItem={({ item }) => (
        <Card>
          <View style={styles.cardHeader}>
            <Text style={styles.guideName}>{item.publicName}</Text>
            <View style={styles.badges}>
              <Badge label="Verified" tone="success" />
              {item.eliteStatus ? <Badge label="Elite" tone="gold" /> : null}
              {item.online ? <Badge label="Online now" /> : null}
            </View>
          </View>
          <Text style={styles.muted}>{formatRating(item.ratingAvg, item.ratingCount)}</Text>
          {item.regionName ? <Text style={styles.muted}>{item.regionName}</Text> : null}
          {item.languages.length > 0 ? (
            <Text style={styles.muted}>Languages: {item.languages.join(", ")}</Text>
          ) : null}
          {item.specialties.length > 0 ? (
            <Text style={styles.muted}>{item.specialties.join(" · ")}</Text>
          ) : null}
          <Text
            accessibilityRole="link"
            onPress={() => router.push(`/guide/${item.userId}`)}
            style={styles.viewLink}
          >
            View profile →
          </Text>
        </Card>
      )}
    />
  );
}

const styles = StyleSheet.create({
  page: { gap: space[3], padding: space[4] },
  header: { gap: space[3] },
  heading: { color: colors.ink, fontSize: fontSize["2xl"], fontWeight: "700" },
  packages: { gap: space[2] },
  packageName: { color: colors.ink, fontSize: fontSize.base, fontWeight: "600" },
  muted: { color: colors.muted, fontSize: fontSize.sm },
  cardHeader: { gap: space[1] },
  guideName: { color: colors.ink, fontSize: fontSize.lg, fontWeight: "700" },
  badges: { flexDirection: "row", flexWrap: "wrap", gap: space[1] },
  viewLink: {
    color: colors.primary,
    fontSize: fontSize.base,
    fontWeight: "600",
    minHeight: 44,
    textAlignVertical: "center",
  },
});
