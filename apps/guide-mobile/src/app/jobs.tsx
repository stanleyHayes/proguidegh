/**
 * Job offers (Phase M, M-14) — live feed over /ws/guide with GET
 * /me/guide/offers as the catch-up and fallback path.
 *
 * Accepting is one tap and the server arbitrates: §10.3 step 4 says the first
 * valid acceptance wins, so this screen deliberately does NOT pre-filter on a
 * locally computed expiry, and does not optimistically remove the row. It
 * sends the accept and reports exactly what came back — 409 (another guide
 * won, or you have an overlapping tour) and 410 (expired) are normal outcomes
 * with their own messages, not generic failures.
 */
import { useCallback, useEffect, useState } from "react";
import { FlatList, Pressable, StyleSheet, Text, View } from "react-native";
import { useRouter } from "expo-router";
import { colors, fontSize, radius, space } from "@proguidegh/tokens";
import { ApiError } from "@proguidegh/api-client";
import { useSession, errorMessage } from "@/lib/session";
import { usePresence } from "@/lib/presence";
import { useWebSocket } from "@/lib/useWebSocket";
import {
  formatDateTime,
  formatPrice,
  parseOffers,
  secondsUntil,
  type Offer,
} from "@/lib/dispatch";
import { Badge, Card, EmptyState, ErrorState, LoadingState, PrimaryButton } from "@/lib/ui";

/** Turn an accept failure into the reason a guide actually needs to read. */
function acceptFailureMessage(err: unknown): string {
  if (err instanceof ApiError) {
    if (err.status === 410) return "That offer expired before you accepted it.";
    if (err.status === 409) {
      return "Another guide took this job, or it overlaps a tour you already have.";
    }
    if (err.status === 404) return "That offer is no longer available.";
    return err.message;
  }
  return errorMessage(err, "Could not accept this job.");
}

export default function JobsScreen() {
  const { client } = useSession();
  const { online } = usePresence();
  const router = useRouter();
  const [offers, setOffers] = useState<Offer[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [acceptingId, setAcceptingId] = useState<string | null>(null);
  const [acceptError, setAcceptError] = useState<string | null>(null);
  const [tick, setTick] = useState(0);

  const load = useCallback(async () => {
    try {
      const data = await client.api("/me/guide/offers");
      setOffers(parseOffers(data));
      setError(null);
    } catch (err: unknown) {
      setError(errorMessage(err, "Could not load your job offers."));
      setOffers((current) => current ?? []);
    }
  }, [client]);

  useEffect(() => {
    const t = setTimeout(() => void load(), 0);
    return () => clearTimeout(t);
  }, [load]);

  // Re-render once a second so the countdown moves. Display only — expiry is
  // decided server-side.
  useEffect(() => {
    const timer = setInterval(() => setTick((n) => n + 1), 1000);
    return () => clearInterval(timer);
  }, []);

  const liveStatus = useWebSocket({
    path: "/ws/guide",
    enabled: online,
    // Any dispatch frame invalidates the list; refetch rather than trying to
    // reconstruct state from deltas we cannot verify.
    onMessage: () => void load(),
    onPoll: load,
  });

  async function accept(offer: Offer) {
    setAcceptingId(offer.id);
    setAcceptError(null);
    try {
      await client.api(`/offers/${offer.id}/accept`, { method: "POST" });
      await load();
      router.push(`/tours/${offer.bookingId}`);
    } catch (err: unknown) {
      setAcceptError(acceptFailureMessage(err));
      await load();
    } finally {
      setAcceptingId(null);
    }
  }

  async function decline(offer: Offer) {
    setAcceptingId(offer.id);
    try {
      await client.api(`/offers/${offer.id}/decline`, { method: "POST" });
    } catch {
      // Declining is advisory; the offer expires on its own regardless.
    } finally {
      setAcceptingId(null);
      await load();
    }
  }

  if (!online) {
    return (
      <EmptyState
        title="You are offline"
        body="Go online from the dashboard to start receiving nearby job offers."
      />
    );
  }
  if (offers === null) return <LoadingState label="Loading job offers…" />;
  if (error && offers.length === 0) {
    return <ErrorState message={error} onRetry={() => void load()} />;
  }

  return (
    <FlatList
      contentContainerStyle={styles.page}
      data={offers}
      extraData={tick}
      keyExtractor={(item) => item.id}
      ListHeaderComponent={
        <View style={styles.header}>
          <Badge
            label={liveStatus === "live" ? "Live" : "Reconnecting — polling"}
            tone={liveStatus === "live" ? "success" : "neutral"}
          />
          {acceptError ? <ErrorState message={acceptError} /> : null}
        </View>
      }
      ListEmptyComponent={
        <EmptyState
          title="No offers right now"
          body="You are online and discoverable. New jobs near you will appear here."
        />
      }
      renderItem={({ item }) => {
        const remaining = secondsUntil(item.expiresAt);
        return (
          <Card>
            <View style={styles.rowHeader}>
              <Text style={styles.reference}>{item.reference}</Text>
              {item.expiresAt ? (
                <Badge
                  label={remaining > 0 ? `${remaining}s left` : "Expiring"}
                  tone={remaining > 20 ? "success" : "gold"}
                />
              ) : null}
            </View>
            {item.packageName ? (
              <Text style={styles.title}>{item.packageName}</Text>
            ) : null}
            <Text style={styles.muted}>{formatDateTime(item.startsAt)}</Text>
            {item.meetingPoint ? (
              <Text style={styles.muted}>{item.meetingPoint}</Text>
            ) : null}
            {item.numGuests !== null ? (
              <Text style={styles.muted}>
                {item.numGuests} guest{item.numGuests === 1 ? "" : "s"}
              </Text>
            ) : null}
            {item.amount !== null ? (
              <Text style={styles.amount}>
                {formatPrice(item.amount, item.currency)}
              </Text>
            ) : null}
            <PrimaryButton
              busy={acceptingId === item.id}
              label="Accept job"
              onPress={() => void accept(item)}
            />
            <Pressable
              accessibilityRole="button"
              disabled={acceptingId === item.id}
              onPress={() => void decline(item)}
              style={styles.decline}
            >
              <Text style={styles.declineLabel}>Decline</Text>
            </Pressable>
          </Card>
        );
      }}
    />
  );
}

const styles = StyleSheet.create({
  page: { gap: space[3], padding: space[4] },
  header: { gap: space[2] },
  rowHeader: {
    alignItems: "center",
    flexDirection: "row",
    gap: space[2],
    justifyContent: "space-between",
  },
  reference: { color: colors.muted, fontSize: fontSize.sm, fontWeight: "600" },
  title: { color: colors.ink, fontSize: fontSize.lg, fontWeight: "700" },
  muted: { color: colors.muted, fontSize: fontSize.sm },
  amount: { color: colors.ink, fontSize: fontSize.xl, fontWeight: "700" },
  decline: {
    alignItems: "center",
    borderColor: colors.border,
    borderRadius: radius.md,
    borderWidth: 1,
    justifyContent: "center",
    minHeight: 44,
  },
  declineLabel: { color: colors.ink, fontSize: fontSize.base, fontWeight: "600" },
});
