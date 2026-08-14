/**
 * Tour operations (Phase M, M-16) — the §8.2 lifecycle a guide drives:
 * en-route → arrived → start → complete.
 *
 * This screen owns the location window. Background collection is authorised
 * for exactly one booking and only while that booking sits in
 * GUIDE_EN_ROUTE..IN_PROGRESS (§11.1); completing or leaving the window
 * revokes it immediately. That coupling lives here rather than in the task so
 * there is one place to check that we never collect outside a live tour.
 */
import { useCallback, useEffect, useState } from "react";
import { ScrollView, StyleSheet, Text, View } from "react-native";
import { useLocalSearchParams } from "expo-router";
import { colors, fontSize, space } from "@proguidegh/tokens";
import { useSession, errorMessage } from "@/lib/session";
import {
  formatDateTime,
  formatPrice,
  isLocationWindow,
  nextTransition,
  parseGuideBookings,
  tourStatusLabel,
  type GuideBooking,
} from "@/lib/dispatch";
import {
  isBackgroundLocationRunning,
  setActiveBooking,
  startBackgroundLocation,
} from "@/lib/location-task";
import { Badge, Card, ErrorState, LoadingState, PrimaryButton } from "@/lib/ui";

export default function TourDetailScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const { client } = useSession();
  const [tour, setTour] = useState<GuideBooking | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loaded, setLoaded] = useState(false);
  const [busy, setBusy] = useState(false);
  const [actionError, setActionError] = useState<string | null>(null);
  const [locationOn, setLocationOn] = useState(false);

  // /me/guide/bookings is the guide-visible view; GET /bookings/{id} is also
  // permitted for the assigned guide but returns the tourist-shaped payload.
  const load = useCallback(async () => {
    try {
      const data = await client.api("/me/guide/bookings");
      const found = parseGuideBookings(data).find((b) => b.id === id) ?? null;
      setTour(found);
      setError(found ? null : "This tour is not assigned to you.");
    } catch (err: unknown) {
      setError(errorMessage(err, "Could not load this tour."));
    } finally {
      setLoaded(true);
    }
  }, [client, id]);

  useEffect(() => {
    const t = setTimeout(() => void load(), 0);
    return () => clearTimeout(t);
  }, [load]);

  // Bind (or release) background collection to this booking whenever the
  // status crosses the §11.1 window boundary.
  useEffect(() => {
    if (!tour) return;
    let cancelled = false;
    const inWindow = isLocationWindow(tour.status);
    void (async () => {
      await setActiveBooking(inWindow ? tour.id : null);
      const running = await isBackgroundLocationRunning();
      if (!cancelled) setLocationOn(inWindow && running);
    })();
    return () => {
      cancelled = true;
    };
  }, [tour]);

  // Leaving the screen must not leave collection authorised.
  useEffect(() => {
    return () => {
      void setActiveBooking(null);
    };
  }, []);

  async function transition(path: string) {
    setBusy(true);
    setActionError(null);
    try {
      await client.api(`/bookings/${id}/${path}`, { method: "POST" });
      if (path === "complete") await setActiveBooking(null);
      await load();
    } catch (err: unknown) {
      setActionError(
        errorMessage(err, "Could not update the tour. Pull down and try again."),
      );
    } finally {
      setBusy(false);
    }
  }

  async function enableLocation() {
    setBusy(true);
    const granted = await startBackgroundLocation();
    if (granted && tour) await setActiveBooking(tour.id);
    setLocationOn(granted);
    if (!granted) {
      setActionError(
        "Location permission is required during a tour so your tourist can find you.",
      );
    }
    setBusy(false);
  }

  if (!loaded) return <LoadingState label="Loading tour…" />;
  if (error || !tour) {
    return <ErrorState message={error ?? "Tour not found."} onRetry={() => void load()} />;
  }

  const next = nextTransition(tour.status);
  const inWindow = isLocationWindow(tour.status);

  return (
    <ScrollView contentContainerStyle={styles.page}>
      <View style={styles.header}>
        <Text style={styles.reference}>{tour.reference}</Text>
        <Badge
          label={tourStatusLabel(tour.status)}
          tone={tour.status === "COMPLETED" ? "success" : "neutral"}
        />
      </View>

      <Card>
        {tour.packageName ? <Text style={styles.title}>{tour.packageName}</Text> : null}
        <Row label="Starts" value={formatDateTime(tour.startsAt)} />
        {tour.endsAt ? <Row label="Ends" value={formatDateTime(tour.endsAt)} /> : null}
        {tour.touristName ? <Row label="Tourist" value={tour.touristName} /> : null}
        {tour.meetingPoint ? (
          <Row label="Meeting point" value={tour.meetingPoint} />
        ) : null}
        {tour.numGuests !== null ? (
          <Row label="Guests" value={String(tour.numGuests)} />
        ) : null}
        {tour.amount !== null ? (
          <Row label="Tour value" value={formatPrice(tour.amount, tour.currency)} />
        ) : null}
      </Card>

      {inWindow ? (
        <Card>
          <Text style={styles.sectionTitle}>
            {locationOn ? "Sharing your location" : "Location sharing is off"}
          </Text>
          <Text style={styles.muted}>
            {locationOn
              ? "Your tourist and operations can see where you are until this tour is complete."
              : "Your tourist cannot see where you are. Turn sharing on so they can find you."}
          </Text>
          {!locationOn ? (
            <PrimaryButton
              busy={busy}
              label="Turn on location sharing"
              onPress={() => void enableLocation()}
            />
          ) : null}
        </Card>
      ) : null}

      {actionError ? <ErrorState message={actionError} /> : null}

      {next ? (
        <PrimaryButton
          busy={busy}
          label={next.label}
          onPress={() => void transition(next.path)}
        />
      ) : (
        <Text style={styles.muted}>
          {tour.status === "COMPLETED"
            ? "This tour is complete. Your earnings appear in your wallet after the T+7 hold."
            : "No further action is available for this tour."}
        </Text>
      )}
    </ScrollView>
  );
}

function Row({ label, value }: { label: string; value: string }) {
  return (
    <View style={styles.row}>
      <Text style={styles.rowLabel}>{label}</Text>
      <Text style={styles.rowValue}>{value}</Text>
    </View>
  );
}

const styles = StyleSheet.create({
  page: { gap: space[3], padding: space[4] },
  header: {
    alignItems: "center",
    flexDirection: "row",
    gap: space[2],
    justifyContent: "space-between",
  },
  reference: { color: colors.muted, fontSize: fontSize.base, fontWeight: "700" },
  title: { color: colors.ink, fontSize: fontSize.lg, fontWeight: "700" },
  sectionTitle: { color: colors.ink, fontSize: fontSize.base, fontWeight: "600" },
  muted: {
    color: colors.muted,
    fontSize: fontSize.sm,
    lineHeight: fontSize.sm * 1.5,
  },
  row: { flexDirection: "row", gap: space[3], justifyContent: "space-between" },
  rowLabel: { color: colors.muted, fontSize: fontSize.sm, flexShrink: 0 },
  rowValue: {
    color: colors.ink,
    flexShrink: 1,
    fontSize: fontSize.sm,
    fontWeight: "600",
    textAlign: "right",
  },
});
