/**
 * Assigned tours (Phase M, M-16) — GET /me/guide/bookings.
 *
 * The API returns upcoming-first ascending then past descending, so the list
 * is rendered in the order received; re-sorting here would fight the contract.
 */
import { useCallback, useEffect, useState } from "react";
import { FlatList, Pressable, StyleSheet, Text, View } from "react-native";
import { useRouter } from "expo-router";
import { colors, fontSize, radius, space } from "@proguidegh/tokens";
import { useSession, errorMessage } from "@/lib/session";
import {
  formatDateTime,
  formatPrice,
  parseGuideBookings,
  tourStatusLabel,
  type GuideBooking,
} from "@/lib/dispatch";
import { Badge, EmptyState, ErrorState, LoadingState } from "@/lib/ui";

export default function ToursScreen() {
  const { client } = useSession();
  const router = useRouter();
  const [tours, setTours] = useState<GuideBooking[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      const data = await client.api("/me/guide/bookings");
      setTours(parseGuideBookings(data));
      setError(null);
    } catch (err: unknown) {
      setError(errorMessage(err, "Could not load your tours."));
      setTours((current) => current ?? []);
    }
  }, [client]);

  useEffect(() => {
    const t = setTimeout(() => void load(), 0);
    return () => clearTimeout(t);
  }, [load]);

  if (tours === null) return <LoadingState label="Loading your tours…" />;
  if (error && tours.length === 0) {
    return <ErrorState message={error} onRetry={() => void load()} />;
  }

  return (
    <FlatList
      contentContainerStyle={styles.page}
      data={tours}
      keyExtractor={(item) => item.id}
      ListEmptyComponent={
        <EmptyState
          title="No tours yet"
          body="Jobs you accept will appear here with their meeting point and status."
        />
      }
      renderItem={({ item }) => (
        <Pressable
          accessibilityRole="button"
          onPress={() => router.push(`/tours/${item.id}`)}
          style={styles.row}
        >
          <View style={styles.rowHeader}>
            <Text style={styles.reference}>{item.reference}</Text>
            <Badge
              label={tourStatusLabel(item.status)}
              tone={item.status === "COMPLETED" ? "success" : "neutral"}
            />
          </View>
          {item.packageName ? (
            <Text style={styles.title}>{item.packageName}</Text>
          ) : null}
          <Text style={styles.muted}>{formatDateTime(item.startsAt)}</Text>
          {item.touristName ? (
            <Text style={styles.muted}>{item.touristName}</Text>
          ) : null}
          {item.amount !== null ? (
            <Text style={styles.muted}>
              {formatPrice(item.amount, item.currency)}
            </Text>
          ) : null}
        </Pressable>
      )}
    />
  );
}

const styles = StyleSheet.create({
  page: { gap: space[3], padding: space[4] },
  row: {
    backgroundColor: colors.surface,
    borderColor: colors.border,
    borderRadius: radius.md,
    borderWidth: 1,
    gap: space[1],
    padding: space[4],
  },
  rowHeader: {
    alignItems: "center",
    flexDirection: "row",
    gap: space[2],
    justifyContent: "space-between",
  },
  reference: { color: colors.muted, fontSize: fontSize.sm, fontWeight: "600" },
  title: { color: colors.ink, fontSize: fontSize.lg, fontWeight: "600" },
  muted: { color: colors.muted, fontSize: fontSize.sm },
});
