/**
 * Booking form (Phase M, M-10) — package, start time and guests →
 * debounced server quote → booking creation, then /checkout.
 *
 * Pricing is server-authoritative (§1.2 #5, §14): the quote shown here is
 * informational and recomputed at creation. Creation requires a stable
 * Idempotency-Key per draft so a retry after a timeout can never double-book
 * (§1.2 #9); the key changes only when the inputs change.
 */
import { useEffect, useMemo, useState } from "react";
import {
  Pressable,
  ScrollView,
  StyleSheet,
  Text,
  TextInput,
  View,
} from "react-native";
import { useLocalSearchParams, useRouter } from "expo-router";
import { colors, fontSize, radius, space } from "@proguidegh/tokens";
import { useSession, errorMessage } from "@/lib/session";
import { formatPrice, parsePackages, type TourPackage } from "@/lib/catalog";
import { parseBookingDetail, parseQuote, type Quote } from "@/lib/bookings";
import { createIdempotencyKeeper } from "@/lib/idempotency";
import { Card, ChipSelect, ErrorState, PrimaryButton } from "@/lib/ui";

const DATE_RE = /^\d{4}-\d{2}-\d{2}$/;
const TIME_RE = /^([01]\d|2[0-3]):[0-5]\d$/;

function toStartsAt(date: string, time: string): Date | null {
  if (!DATE_RE.test(date.trim()) || !TIME_RE.test(time.trim())) return null;
  const d = new Date(`${date.trim()}T${time.trim()}:00`);
  return Number.isNaN(d.getTime()) ? null : d;
}

export default function BookScreen() {
  const { id: guideId } = useLocalSearchParams<{ id: string }>();
  const router = useRouter();
  const { client } = useSession();
  const keeper = useMemo(() => createIdempotencyKeeper(), []);

  const [packages, setPackages] = useState<TourPackage[] | null>(null);
  const [packagesError, setPackagesError] = useState<string | null>(null);

  const [packageId, setPackageId] = useState<string | null>(null);
  const [date, setDate] = useState("");
  const [time, setTime] = useState("");
  const [guests, setGuests] = useState(1);
  const [meetingPoint, setMeetingPoint] = useState("");
  const [notes, setNotes] = useState("");

  const [quote, setQuote] = useState<Quote | null>(null);
  const [quoteLoading, setQuoteLoading] = useState(false);
  const [quoteError, setQuoteError] = useState<string | null>(null);

  const [submitting, setSubmitting] = useState(false);
  const [submitError, setSubmitError] = useState<string | null>(null);

  // Load the bookable package catalog once (deferred to satisfy the
  // react-hooks lint rule against synchronous setState in effects).
  useEffect(() => {
    let cancelled = false;
    const t = setTimeout(() => {
      client
        .api("/tour-packages", { anonymous: true })
        .then((data) => {
          if (!cancelled) setPackages(parsePackages(data));
        })
        .catch((err) => {
          if (!cancelled) {
            setPackagesError(errorMessage(err, "Could not load tour packages."));
          }
        });
    }, 0);
    return () => {
      cancelled = true;
      clearTimeout(t);
    };
  }, [client]);

  // Debounced server quote whenever the pricing inputs are complete.
  useEffect(() => {
    let cancelled = false;
    const t = setTimeout(() => {
      const startsAt = toStartsAt(date, time);
      if (!packageId || !startsAt) {
        setQuote(null);
        setQuoteError(null);
        return;
      }
      setQuoteLoading(true);
      client
        .api("/bookings/quote", {
          method: "POST",
          anonymous: true,
          body: {
            package_id: packageId,
            starts_at: startsAt.toISOString(),
            guests,
          },
        })
        .then((data) => {
          if (cancelled) return;
          const parsed = parseQuote(data);
          setQuote(parsed);
          setQuoteError(parsed ? null : "Could not price this trip.");
        })
        .catch((err) => {
          if (cancelled) return;
          setQuote(null);
          setQuoteError(errorMessage(err, "Could not price this trip."));
        })
        .finally(() => {
          if (!cancelled) setQuoteLoading(false);
        });
    }, 500);
    return () => {
      cancelled = true;
      clearTimeout(t);
    };
  }, [client, packageId, date, time, guests]);

  async function confirm() {
    const startsAt = toStartsAt(date, time);
    if (!packageId) {
      setSubmitError("Choose a tour package.");
      return;
    }
    if (!startsAt) {
      setSubmitError("Enter a valid date (YYYY-MM-DD) and time (HH:MM, 24h).");
      return;
    }
    if (startsAt.getTime() <= Date.now()) {
      setSubmitError("The start time must be in the future.");
      return;
    }
    const signature = [
      guideId,
      packageId,
      startsAt.toISOString(),
      guests,
      meetingPoint.trim(),
      notes.trim(),
    ].join("|");
    setSubmitting(true);
    setSubmitError(null);
    try {
      const data = await client.api("/bookings", {
        method: "POST",
        headers: { "Idempotency-Key": keeper(signature) },
        body: {
          package_id: packageId,
          guide_id: guideId,
          starts_at: startsAt.toISOString(),
          guests,
          meeting_point: meetingPoint.trim() === "" ? null : meetingPoint.trim(),
          notes: notes.trim() === "" ? null : notes.trim(),
        },
      });
      const booking = parseBookingDetail(data);
      if (!booking) throw new Error("Unexpected response from the server.");
      router.replace(`/checkout/${booking.id}`);
    } catch (err) {
      setSubmitError(errorMessage(err, "This booking could not be created."));
    } finally {
      setSubmitting(false);
    }
  }

  if (packagesError) {
    return (
      <ErrorState
        message={packagesError}
        onRetry={() => {
          setPackagesError(null);
          setPackages(null);
          client
            .api("/tour-packages", { anonymous: true })
            .then((data) => setPackages(parsePackages(data)))
            .catch((err) =>
              setPackagesError(errorMessage(err, "Could not load tour packages.")),
            );
        }}
      />
    );
  }

  const startsAt = toStartsAt(date, time);

  return (
    <ScrollView contentContainerStyle={styles.page} keyboardShouldPersistTaps="handled">
      {packages ? (
        <Card>
          <ChipSelect
            label="Tour package"
            options={packages.map((p) => ({ id: p.id, name: p.name }))}
            value={packageId}
            onChange={setPackageId}
          />
          {packageId ? (
            <Text style={styles.muted}>
              {(() => {
                const p = packages.find((pkg) => pkg.id === packageId);
                return p
                  ? `${formatPrice(p.basePrice, p.currency)} · ${Math.round(p.durationMinutes / 60)}h`
                  : "";
              })()}
            </Text>
          ) : null}
        </Card>
      ) : (
        <Text style={styles.muted}>Loading packages…</Text>
      )}

      <Card>
        <Text style={styles.sectionTitle}>When</Text>
        <TextInput
          accessibilityLabel="Start date (YYYY-MM-DD)"
          autoCapitalize="none"
          autoCorrect={false}
          keyboardType="numbers-and-punctuation"
          onChangeText={setDate}
          placeholder="YYYY-MM-DD"
          placeholderTextColor={colors.muted}
          style={styles.input}
          value={date}
        />
        <TextInput
          accessibilityLabel="Start time (HH:MM, 24-hour)"
          autoCapitalize="none"
          autoCorrect={false}
          keyboardType="numbers-and-punctuation"
          onChangeText={setTime}
          placeholder="HH:MM (24h)"
          placeholderTextColor={colors.muted}
          style={styles.input}
          value={time}
        />
        {date !== "" && time !== "" && !startsAt ? (
          <Text style={styles.errorText}>
            Enter a valid date (YYYY-MM-DD) and time (HH:MM, 24h).
          </Text>
        ) : null}
        <View style={styles.stepperRow}>
          <Text style={styles.sectionTitle}>Guests</Text>
          <View style={styles.stepper}>
            <Pressable
              accessibilityLabel="Fewer guests"
              accessibilityRole="button"
              onPress={() => setGuests((g) => Math.max(1, g - 1))}
              style={styles.stepButton}
            >
              <Text style={styles.stepButtonLabel}>−</Text>
            </Pressable>
            <Text style={styles.guestCount}>{guests}</Text>
            <Pressable
              accessibilityLabel="More guests"
              accessibilityRole="button"
              onPress={() => setGuests((g) => Math.min(50, g + 1))}
              style={styles.stepButton}
            >
              <Text style={styles.stepButtonLabel}>+</Text>
            </Pressable>
          </View>
        </View>
      </Card>

      <Card>
        <Text style={styles.sectionTitle}>Details</Text>
        <TextInput
          accessibilityLabel="Meeting point"
          onChangeText={setMeetingPoint}
          placeholder="Meeting point (e.g. hotel lobby)"
          placeholderTextColor={colors.muted}
          style={styles.input}
          value={meetingPoint}
        />
        <TextInput
          accessibilityLabel="Notes for the guide"
          multiline
          onChangeText={setNotes}
          placeholder="Notes for your guide (optional)"
          placeholderTextColor={colors.muted}
          style={[styles.input, styles.inputMultiline]}
          value={notes}
        />
      </Card>

      {quoteLoading ? <Text style={styles.muted}>Getting a price…</Text> : null}
      {quoteError ? <Text style={styles.errorText}>{quoteError}</Text> : null}
      {quote ? (
        <Card>
          <Text style={styles.sectionTitle}>Price (server quote)</Text>
          <View style={styles.row}>
            <Text style={styles.rowLabel}>Total</Text>
            <Text style={styles.rowValue}>{formatPrice(quote.amount, quote.currency)}</Text>
          </View>
          {quote.platformFee !== null ? (
            <View style={styles.row}>
              <Text style={styles.muted}>Platform fee</Text>
              <Text style={styles.muted}>{formatPrice(quote.platformFee, quote.currency)}</Text>
            </View>
          ) : null}
          {quote.tourismLevy !== null ? (
            <View style={styles.row}>
              <Text style={styles.muted}>Tourism levy</Text>
              <Text style={styles.muted}>{formatPrice(quote.tourismLevy, quote.currency)}</Text>
            </View>
          ) : null}
          {quote.endsAt ? (
            <Text style={styles.muted}>Ends {quote.endsAt.slice(11, 16)} local</Text>
          ) : null}
          <Text style={styles.disclaimer}>
            The final price is confirmed by the server when the booking is created.
          </Text>
        </Card>
      ) : null}

      {submitError ? <Text style={styles.errorText}>{submitError}</Text> : null}
      <PrimaryButton
        busy={submitting}
        disabled={!packageId || !startsAt}
        label="Confirm booking"
        onPress={confirm}
      />
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  page: { gap: space[3], padding: space[4] },
  sectionTitle: { color: colors.ink, fontSize: fontSize.base, fontWeight: "600" },
  muted: { color: colors.muted, fontSize: fontSize.sm },
  errorText: { color: colors.danger, fontSize: fontSize.sm },
  disclaimer: { color: colors.muted, fontSize: fontSize.sm, fontStyle: "italic" },
  input: {
    borderColor: colors.border,
    borderRadius: radius.md,
    borderWidth: 1,
    color: colors.ink,
    fontSize: fontSize.base,
    minHeight: 44,
    paddingHorizontal: space[3],
  },
  inputMultiline: { minHeight: 88, paddingTop: space[2], textAlignVertical: "top" },
  stepperRow: {
    alignItems: "center",
    flexDirection: "row",
    justifyContent: "space-between",
  },
  stepper: { alignItems: "center", flexDirection: "row", gap: space[3] },
  stepButton: {
    alignItems: "center",
    borderColor: colors.border,
    borderRadius: radius.full,
    borderWidth: 1,
    height: 44,
    justifyContent: "center",
    width: 44,
  },
  stepButtonLabel: { color: colors.primary, fontSize: fontSize.xl, fontWeight: "600" },
  guestCount: {
    color: colors.ink,
    fontSize: fontSize.lg,
    fontWeight: "600",
    minWidth: 24,
    textAlign: "center",
  },
  row: { flexDirection: "row", justifyContent: "space-between" },
  rowLabel: { color: colors.ink, fontSize: fontSize.base, fontWeight: "600" },
  rowValue: { color: colors.ink, fontSize: fontSize.base, fontWeight: "700" },
});
