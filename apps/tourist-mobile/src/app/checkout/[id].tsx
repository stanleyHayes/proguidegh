/**
 * Checkout (Phase M, M-10) — payment initiation opens the provider-hosted
 * page in an in-app browser; the app NEVER collects card or MoMo details
 * (§1.2 #6 — non-negotiable). After the browser closes we poll the booking
 * until the provider webhook flips it out of PAYMENT_PENDING.
 */
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { ScrollView, StyleSheet, Text, View } from "react-native";
import { useLocalSearchParams } from "expo-router";
import * as WebBrowser from "expo-web-browser";
import { colors, fontSize, space } from "@proguidegh/tokens";
import { useSession, errorMessage } from "@/lib/session";
import { formatPrice } from "@/lib/catalog";
import {
  formatDateTime,
  parseBookingDetail,
  parsePaymentIntent,
  statusLabel,
  statusTone,
  type BookingDetail,
} from "@/lib/bookings";
import { createIdempotencyKeeper } from "@/lib/idempotency";
import { Badge, Card, ErrorState, LoadingState, PrimaryButton } from "@/lib/ui";

const POLL_ATTEMPTS = 15;
const POLL_INTERVAL_MS = 2000;

export default function CheckoutScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const { client } = useSession();
  const keeper = useMemo(() => createIdempotencyKeeper(), []);

  const [booking, setBooking] = useState<BookingDetail | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [paying, setPaying] = useState(false);
  const [payError, setPayError] = useState<string | null>(null);
  const [provider, setProvider] = useState<string | null>(null);
  const [waiting, setWaiting] = useState(false);
  const mounted = useRef(true);

  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);

  const load = useCallback(() => {
    setLoading(true);
    client
      .api(`/bookings/${id}`)
      .then((data) => {
        if (!mounted.current) return;
        const parsed = parseBookingDetail(data);
        if (parsed) {
          setBooking(parsed);
          setError(null);
        } else {
          setError("This booking could not be loaded.");
        }
      })
      .catch((err) => {
        if (mounted.current) {
          setError(errorMessage(err, "This booking could not be loaded."));
        }
      })
      .finally(() => {
        if (mounted.current) setLoading(false);
      });
  }, [client, id]);

  useEffect(() => {
    const t = setTimeout(load, 0);
    return () => clearTimeout(t);
  }, [load]);

  async function pollUntilSettled(): Promise<void> {
    setWaiting(true);
    try {
      for (let attempt = 0; attempt < POLL_ATTEMPTS; attempt++) {
        await new Promise((resolve) => setTimeout(resolve, POLL_INTERVAL_MS));
        if (!mounted.current) return;
        try {
          const data = await client.api(`/bookings/${id}`);
          const parsed = parseBookingDetail(data);
          if (parsed && mounted.current) {
            setBooking(parsed);
            if (parsed.status !== "PAYMENT_PENDING") return;
          }
        } catch {
          // Transient network error — keep polling.
        }
      }
    } finally {
      if (mounted.current) setWaiting(false);
    }
  }

  async function pay() {
    if (!booking) return;
    setPaying(true);
    setPayError(null);
    try {
      const data = await client.api(`/bookings/${booking.id}/payment-intent`, {
        method: "POST",
        headers: { "Idempotency-Key": keeper(`pay|${booking.id}`) },
      });
      const intent = parsePaymentIntent(data);
      if (mounted.current) setProvider(intent.provider);
      if (!intent.authorizationUrl) {
        throw new Error("The payment provider did not return a checkout page.");
      }
      await WebBrowser.openBrowserAsync(intent.authorizationUrl);
      await pollUntilSettled();
    } catch (err) {
      if (mounted.current) {
        setPayError(errorMessage(err, "Payment could not be started."));
      }
    } finally {
      if (mounted.current) setPaying(false);
    }
  }

  if (loading) return <LoadingState label="Loading your booking…" />;
  if (error || !booking) {
    return (
      <ErrorState message={error ?? "Booking not found."} onRetry={load} />
    );
  }

  const settled = booking.status !== "PAYMENT_PENDING";
  const paid = settled && booking.status !== "CANCELLED";

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
        {booking.amount !== null ? (
          <Text style={styles.amount}>
            {formatPrice(booking.amount, booking.currency)}
          </Text>
        ) : null}
        {provider === "mock" ? <Badge label="Test payment" tone="gold" /> : null}
      </Card>

      {!settled ? (
        <>
          <Text style={styles.muted}>
            You will complete payment on the provider&apos;s secure page. This app
            never sees your card or mobile money details.
          </Text>
          {payError ? <Text style={styles.errorText}>{payError}</Text> : null}
          {waiting ? (
            <Text style={styles.muted}>Confirming your payment…</Text>
          ) : null}
          <PrimaryButton
            busy={paying || waiting}
            label={payError ? "Try payment again" : "Pay now"}
            onPress={pay}
          />
        </>
      ) : paid ? (
        <Card>
          <Text style={styles.sectionTitle}>Payment confirmed</Text>
          <Text style={styles.body}>
            Booking {booking.reference} is confirmed. Your guide and receipt
            will appear under My bookings.
          </Text>
        </Card>
      ) : (
        <Card>
          <Text style={styles.sectionTitle}>Booking cancelled</Text>
          <Text style={styles.body}>
            This booking was cancelled and will not be charged.
          </Text>
        </Card>
      )}
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
});
