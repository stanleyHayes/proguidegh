/**
 * Public reviews list for a guide profile (Phase 6; spec §13.5). The API
 * exposes no tourist identity — rating, tags, body and date only. Failures
 * degrade to silence: a missing review list must never block booking.
 */
import { useEffect, useState } from "react";
import { StyleSheet, Text, View } from "react-native";
import { colors, fontSize, space } from "@proguidegh/tokens";
import { useSession } from "@/lib/session";
import { Badge, Card } from "@/lib/ui";

interface PublicReview {
  id: string;
  rating: number;
  body: string | null;
  tags: string[];
  createdAt: string | null;
}

function asRecord(value: unknown): Record<string, unknown> | null {
  return value !== null && typeof value === "object"
    ? (value as Record<string, unknown>)
    : null;
}

function parseReviews(data: unknown): PublicReview[] {
  const list = asRecord(data)?.reviews;
  if (!Array.isArray(list)) return [];
  return list
    .map((entry) => asRecord(entry))
    .filter((r): r is Record<string, unknown> => r !== null && typeof r.id === "string")
    .map((r) => ({
      id: r.id as string,
      rating: Number(r.rating ?? 0),
      body: typeof r.body === "string" ? r.body : null,
      tags: Array.isArray(r.tags) ? (r.tags as string[]) : [],
      createdAt: typeof r.created_at === "string" ? r.created_at : null,
    }));
}

export function GuideReviews({ guideId }: { guideId: string }) {
  const { client } = useSession();
  const [reviews, setReviews] = useState<PublicReview[]>([]);

  useEffect(() => {
    let cancelled = false;
    const t = setTimeout(() => {
      client
        .api(`/guides/${guideId}/reviews`, { anonymous: true })
        .then((data) => {
          if (!cancelled) setReviews(parseReviews(data));
        })
        .catch(() => {
          // Degrade silently — reviews are enhancement, not a blocker.
        });
    }, 0);
    return () => {
      cancelled = true;
      clearTimeout(t);
    };
  }, [client, guideId]);

  if (reviews.length === 0) return null;

  return (
    <Card>
      <Text style={styles.title}>Reviews ({reviews.length})</Text>
      {reviews.map((review) => (
        <View key={review.id} style={styles.review}>
          <Text style={styles.stars} accessibilityLabel={`Rated ${review.rating} out of 5`}>
            {"★".repeat(review.rating)}
            {"☆".repeat(Math.max(0, 5 - review.rating))}
            {review.createdAt
              ? `  ·  ${new Date(review.createdAt).toLocaleDateString(undefined, {
                  day: "numeric",
                  month: "short",
                  year: "numeric",
                })}`
              : ""}
          </Text>
          {review.tags.length > 0 ? (
            <View style={styles.badges}>
              {review.tags.map((tag) => (
                <Badge key={tag} label={tag} />
              ))}
            </View>
          ) : null}
          {review.body ? <Text style={styles.body}>{review.body}</Text> : null}
        </View>
      ))}
    </Card>
  );
}

const styles = StyleSheet.create({
  title: { color: colors.ink, fontSize: fontSize.base, fontWeight: "600" },
  review: { gap: space[1] },
  stars: { color: colors.gold, fontSize: fontSize.sm },
  badges: { flexDirection: "row", flexWrap: "wrap", gap: space[1] },
  body: { color: colors.ink, fontSize: fontSize.sm, lineHeight: fontSize.sm * 1.5 },
});
