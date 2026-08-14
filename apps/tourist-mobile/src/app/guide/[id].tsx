/**
 * Public guide profile (Phase M, M-09) — GET /guides/{id}; 404 means the
 * guide is not currently eligible/public (§10.2). The booking CTA routes to
 * the M-10 booking form.
 */
import { useEffect, useState } from "react";
import { ScrollView, StyleSheet, Text, View } from "react-native";
import { useLocalSearchParams, useRouter } from "expo-router";
import { colors, fontSize, space } from "@proguidegh/tokens";
import { useSession, errorMessage } from "@/lib/session";
import { formatRating, parseGuideDetail, type GuideDetail } from "@/lib/catalog";
import { Badge, Card, ErrorState, LoadingState, PrimaryButton } from "@/lib/ui";
import { GuideReviews } from "@/lib/reviews";

export default function GuideDetailScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const { client } = useSession();
  const router = useRouter();
  const [guide, setGuide] = useState<GuideDetail | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;
    // Deferred to avoid synchronous setState inside the effect body.
    const t = setTimeout(() => {
      setLoading(true);
      client
        .api(`/guides/${id}`)
        .then((data) => {
          if (cancelled) return;
          const parsed = parseGuideDetail(data);
          if (parsed) {
            setGuide(parsed);
            setError(null);
          } else {
            setError("This guide is not currently bookable.");
          }
        })
        .catch((err) => {
          if (!cancelled) {
            setError(errorMessage(err, "This guide is not currently bookable."));
          }
        })
        .finally(() => {
          if (!cancelled) setLoading(false);
        });
    }, 0);
    return () => {
      cancelled = true;
      clearTimeout(t);
    };
  }, [client, id]);

  if (loading) return <LoadingState label="Loading guide profile…" />;
  if (error || !guide) {
    return <ErrorState message={error ?? "Guide not found."} />;
  }

  return (
    <ScrollView contentContainerStyle={styles.page}>
      <View style={styles.header}>
        <Text style={styles.name}>{guide.publicName}</Text>
        <View style={styles.badges}>
          <Badge label="Verified" tone="success" />
          {guide.eliteStatus ? <Badge label="Elite" tone="gold" /> : null}
          {guide.online ? <Badge label="Online now" /> : null}
        </View>
      </View>

      <Text style={styles.rating}>{formatRating(guide.ratingAvg, guide.ratingCount)}</Text>
      {guide.regionName ? <Text style={styles.muted}>{guide.regionName}</Text> : null}

      {guide.bio ? (
        <Card>
          <Text style={styles.sectionTitle}>About</Text>
          <Text style={styles.body}>{guide.bio}</Text>
        </Card>
      ) : null}

      {guide.languages.length > 0 ? (
        <Card>
          <Text style={styles.sectionTitle}>Languages</Text>
          <Text style={styles.body}>{guide.languages.join(", ")}</Text>
        </Card>
      ) : null}

      {guide.specialties.length > 0 ? (
        <Card>
          <Text style={styles.sectionTitle}>Specialties</Text>
          <Text style={styles.body}>{guide.specialties.join(" · ")}</Text>
        </Card>
      ) : null}

      <GuideReviews guideId={id} />

      <PrimaryButton
        label="Book this guide"
        onPress={() => router.push(`/book/${guide.userId}`)}
      />
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  page: { gap: space[3], padding: space[4] },
  header: { gap: space[2] },
  name: { color: colors.ink, fontSize: fontSize["2xl"], fontWeight: "700" },
  badges: { flexDirection: "row", flexWrap: "wrap", gap: space[1] },
  rating: { color: colors.ink, fontSize: fontSize.base, fontWeight: "600" },
  muted: { color: colors.muted, fontSize: fontSize.sm },
  sectionTitle: { color: colors.ink, fontSize: fontSize.base, fontWeight: "600" },
  body: { color: colors.muted, fontSize: fontSize.base, lineHeight: fontSize.base * 1.5 },
});
