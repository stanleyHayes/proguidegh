/**
 * Booking detail (Phase M, M-11) — GET /bookings/{id} renders the summary
 * plus the status timeline (booking_events). The receipt is fetched from
 * GET /bookings/{id}/receipt and opened via its short-lived signed URL in
 * the system browser — the file is never cached to disk (§17, stop
 * condition 8: files are never public and never persisted client-side).
 */
import { useCallback, useEffect, useState } from "react";
import { ScrollView, StyleSheet, Text, View } from "react-native";
import { useLocalSearchParams } from "expo-router";
import * as WebBrowser from "expo-web-browser";
import { colors, fontSize, space } from "@proguidegh/tokens";
import { useSession, errorMessage } from "@/lib/session";
import { formatPrice } from "@/lib/catalog";
import {
  formatDateTime,
  hasReceipt,
  parseBookingDetail,
  parseReceipt,
  statusLabel,
  statusTone,
  type BookingDetail,
} from "@/lib/bookings";
import { Badge, Card, ErrorState, LoadingState, PrimaryButton } from "@/lib/ui";
import { SosButton } from "@/lib/sos";
import { ReviewForm } from "@/lib/review";

export default function BookingDetailScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const { client } = useSession();
  const [booking, setBooking] = useState<BookingDetail | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [receiptBusy, setReceiptBusy] = useState(false);
  const [receiptError, setReceiptError] = useState<string | null>(null);

  const load = useCallback(() => {
    setLoading(true);
    client
      .api(`/bookings/${id}`)
      .then((data) => {
        const parsed = parseBookingDetail(data);
        if (parsed) {
          setBooking(parsed);
          setError(null);
        } else {
          setError("This booking could not be loaded.");
        }
      })
      .catch((err) => setError(errorMessage(err, "This booking could not be loaded.")))
      .finally(() => setLoading(false));
  }, [client, id]);

  useEffect(() => {
    const t = setTimeout(load, 0);
    return () => clearTimeout(t);
  }, [load]);

  async function openReceipt() {
    if (!booking) return;
    setReceiptBusy(true);
    setReceiptError(null);
    try {
      const data = await client.api(`/bookings/${booking.id}/receipt`);
      const receipt = parseReceipt(data);
      if (!receipt) {
        throw new Error("No receipt has been issued for this booking yet.");
      }
      await WebBrowser.openBrowserAsync(receipt.downloadUrl);
    } catch (err) {
      setReceiptError(errorMessage(err, "The receipt could not be opened."));
    } finally {
      setReceiptBusy(false);
    }
  }

  if (loading) return <LoadingState label="Loading booking…" />;
  if (error || !booking) {
    return <ErrorState message={error ?? "Booking not found."} onRetry={load} />;
  }

  return (
    <ScrollView contentContainerStyle={styles.page}>
      <Card>
        <View style={styles.headerRow}>
          <Text style={styles.reference}>{booking.reference}</Text>
          <Badge label={statusLabel(booking.status)} tone={statusTone(booking.status)} />
        </View>
        {booking.packageName ? (
          <Text style={styles.body}>{booking.packageName}</Text>
        ) : null}
        <Text style={styles.muted}>{formatDateTime(booking.startsAt)}</Text>
        {booking.numGuests !== null ? (
          <Text style={styles.muted}>
            {booking.numGuests} guest{booking.numGuests === 1 ? "" : "s"}
          </Text>
        ) : null}
        {booking.meetingPoint ? (
          <Text style={styles.muted}>Meet at: {booking.meetingPoint}</Text>
        ) : null}
        {booking.guideName ? (
          <Text style={styles.muted}>Guide: {booking.guideName}</Text>
        ) : null}
        {booking.amount !== null ? (
          <Text style={styles.amount}>{formatPrice(booking.amount, booking.currency)}</Text>
        ) : null}
      </Card>

      {booking.events.length > 0 ? (
        <Card>
          <Text style={styles.sectionTitle}>Timeline</Text>
          {booking.events.map((event, index) => (
            <View key={`${event.status}-${event.at ?? index}`} style={styles.event}>
              <View style={styles.eventDot} />
              <View style={styles.eventBody}>
                <Text style={styles.eventStatus}>{event.status.replaceAll("_", " ")}</Text>
                <Text style={styles.muted}>{formatDateTime(event.at)}</Text>
                {event.reason ? <Text style={styles.muted}>{event.reason}</Text> : null}
              </View>
            </View>
          ))}
        </Card>
      ) : null}

      {hasReceipt(booking.status) ? (
        <>
          {receiptError ? <Text style={styles.errorText}>{receiptError}</Text> : null}
          <PrimaryButton
            busy={receiptBusy}
            label="View receipt"
            onPress={openReceipt}
          />
        </>
      ) : (
        <Text style={styles.muted}>
          A receipt is issued once payment is confirmed.
        </Text>
      )}

      <SosButton bookingId={booking.id} status={booking.status} />

      {booking.status === "COMPLETED" ? <ReviewForm bookingId={booking.id} /> : null}
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  page: { gap: space[3], padding: space[4] },
  headerRow: {
    alignItems: "center",
    flexDirection: "row",
    justifyContent: "space-between",
  },
  reference: { color: colors.ink, fontSize: fontSize.lg, fontWeight: "700" },
  sectionTitle: { color: colors.ink, fontSize: fontSize.base, fontWeight: "600" },
  body: { color: colors.ink, fontSize: fontSize.base },
  muted: { color: colors.muted, fontSize: fontSize.sm },
  amount: { color: colors.ink, fontSize: fontSize.xl, fontWeight: "700" },
  errorText: { color: colors.danger, fontSize: fontSize.sm },
  event: { flexDirection: "row", gap: space[2] },
  eventDot: {
    backgroundColor: colors.primary,
    borderRadius: 4,
    height: 8,
    marginTop: 6,
    width: 8,
  },
  eventBody: { flex: 1, gap: 2 },
  eventStatus: {
    color: colors.ink,
    fontSize: fontSize.sm,
    fontWeight: "600",
    textTransform: "capitalize",
  },
});
