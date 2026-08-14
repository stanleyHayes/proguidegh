/**
 * My bookings (Phase M, M-11) — GET /me/bookings, newest first,
 * cursor-paginated (§14). Rows route to the booking detail timeline.
 */
import { useCallback, useEffect, useState } from "react";
import { FlatList, Pressable, StyleSheet, Text, View } from "react-native";
import { useRouter } from "expo-router";
import { colors, fontSize, radius, space } from "@proguidegh/tokens";
import { useSession, errorMessage } from "@/lib/session";
import { formatPrice } from "@/lib/catalog";
import {
  formatDateTime,
  parseBookings,
  statusLabel,
  statusTone,
  type BookingSummary,
} from "@/lib/bookings";
import { Badge, EmptyState, ErrorState, LoadingState } from "@/lib/ui";

function asCursor(data: unknown): string | null {
  if (data !== null && typeof data === "object") {
    const c = (data as Record<string, unknown>).next_cursor;
    if (typeof c === "string" && c !== "") return c;
  }
  return null;
}

export default function BookingsScreen() {
  const router = useRouter();
  const { client } = useSession();
  const [bookings, setBookings] = useState<BookingSummary[] | null>(null);
  const [cursor, setCursor] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loadingMore, setLoadingMore] = useState(false);

  const load = useCallback(
    (after: string | null, append: boolean) => {
      if (append) setLoadingMore(true);
      const path = after
        ? `/me/bookings?cursor=${encodeURIComponent(after)}`
        : "/me/bookings";
      return client
        .api(path)
        .then((data) => {
          const page = parseBookings(data);
          setBookings((prev) => (append && prev ? [...prev, ...page] : page));
          setCursor(asCursor(data));
          setError(null);
        })
        .catch((err) => {
          if (!append) {
            setError(errorMessage(err, "Your bookings could not be loaded."));
          }
        })
        .finally(() => setLoadingMore(false));
    },
    [client],
  );

  useEffect(() => {
    const t = setTimeout(() => {
      setBookings(null);
      void load(null, false);
    }, 0);
    return () => clearTimeout(t);
  }, [load]);

  if (error && bookings === null) {
    return <ErrorState message={error} onRetry={() => void load(null, false)} />;
  }
  if (bookings === null) {
    return <LoadingState label="Loading your bookings…" />;
  }
  if (bookings.length === 0) {
    return (
      <EmptyState
        title="No bookings yet"
        body="Search for a certified guide to plan your first tour."
      />
    );
  }

  return (
    <FlatList
      contentContainerStyle={styles.page}
      data={bookings}
      keyExtractor={(item) => item.id}
      ListFooterComponent={
        cursor ? (
          <Pressable
            accessibilityRole="button"
            disabled={loadingMore}
            onPress={() => void load(cursor, true)}
            style={styles.more}
          >
            <Text style={styles.moreLabel}>
              {loadingMore ? "Loading…" : "Load older bookings"}
            </Text>
          </Pressable>
        ) : null
      }
      renderItem={({ item }) => (
        <Pressable
          accessibilityRole="button"
          onPress={() => router.push(`/bookings/${item.id}`)}
          style={styles.row}
        >
          <View style={styles.rowHeader}>
            <Text style={styles.reference}>{item.reference}</Text>
            <Badge label={statusLabel(item.status)} tone={statusTone(item.status)} />
          </View>
          {item.packageName ? (
            <Text style={styles.body}>{item.packageName}</Text>
          ) : null}
          <Text style={styles.muted}>{formatDateTime(item.startsAt)}</Text>
          {item.amount !== null ? (
            <Text style={styles.muted}>{formatPrice(item.amount, item.currency)}</Text>
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
    justifyContent: "space-between",
  },
  reference: { color: colors.ink, fontSize: fontSize.base, fontWeight: "700" },
  body: { color: colors.ink, fontSize: fontSize.base },
  muted: { color: colors.muted, fontSize: fontSize.sm },
  more: { alignItems: "center", minHeight: 44, justifyContent: "center" },
  moreLabel: { color: colors.primary, fontSize: fontSize.base, fontWeight: "600" },
});
