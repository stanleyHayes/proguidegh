/**
 * SOS button (Phase 6 parity with tourist-web; spec §12).
 *
 * Takes a FRESH fix on tap (§12 step 7 — never a cached position) and posts
 * it with the booking id; the button stays retryable on failure. The API
 * requires coordinates, so a device that cannot provide any cannot send an
 * SOS — we say so rather than fabricate a position. Foreground ("when in
 * use") permission is requested at tap time with the reason stated first.
 *
 * Copy names ProGuideGH operations only — never police/emergency services
 * (§12 safety requirement).
 */
import { useState } from "react";
import { Alert, StyleSheet, Text, View } from "react-native";
import * as Location from "expo-location";
import { colors, fontSize, radius, space } from "@proguidegh/tokens";
import { useSession, errorMessage } from "@/lib/session";
import { Card, PrimaryButton } from "@/lib/ui";

const ACTIVE = new Set([
  "CONFIRMED",
  "GUIDE_EN_ROUTE",
  "GUIDE_ARRIVED",
  "IN_PROGRESS",
]);

export function SosButton({
  bookingId,
  status,
}: {
  bookingId: string;
  status: string;
}) {
  const { client } = useSession();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [sent, setSent] = useState(false);

  if (!ACTIVE.has(status)) return null;

  async function trigger() {
    Alert.alert(
      "Send SOS?",
      "ProGuideGH operations will get your current location. This does not contact police or emergency services.",
      [
        { text: "Cancel", style: "cancel" },
        { text: "Send SOS", style: "destructive", onPress: () => void send() },
      ],
    );
  }

  async function send() {
    setBusy(true);
    setError(null);
    try {
      const permission = await Location.requestForegroundPermissionsAsync();
      if (permission.status !== "granted") {
        throw new Error(
          "Location permission is needed so operations can find you. Enable it and try again.",
        );
      }
      const fix = await Location.getCurrentPositionAsync({
        accuracy: Location.Accuracy.High,
      });
      await client.api(`/bookings/${bookingId}/sos`, {
        method: "POST",
        body: {
          latitude: fix.coords.latitude,
          longitude: fix.coords.longitude,
          accuracy_m: fix.coords.accuracy ?? undefined,
        },
      });
      setSent(true);
    } catch (err) {
      setError(errorMessage(err, "The SOS could not be sent. Try again immediately."));
    } finally {
      setBusy(false);
    }
  }

  if (sent) {
    return (
      <Card>
        <Text style={styles.title}>SOS sent</Text>
        <Text style={styles.muted}>
          ProGuideGH operations has your location and will respond. For
          life-threatening emergencies call local emergency services.
        </Text>
      </Card>
    );
  }

  return (
    <View style={styles.box}>
      <Text style={styles.muted}>
        Emergency? SOS alerts ProGuideGH operations with your live location —
        it does not dispatch police or ambulance.
      </Text>
      {error ? <Text style={styles.error}>{error}</Text> : null}
      <PrimaryButton busy={busy} label="Send SOS" onPress={() => void trigger()} />
    </View>
  );
}

const styles = StyleSheet.create({
  box: {
    backgroundColor: colors.surfaceAlt,
    borderColor: colors.danger,
    borderRadius: radius.md,
    borderWidth: 1,
    gap: space[2],
    padding: space[4],
  },
  title: { color: colors.ink, fontSize: fontSize.base, fontWeight: "700" },
  muted: { color: colors.muted, fontSize: fontSize.sm, lineHeight: fontSize.sm * 1.5 },
  error: { color: colors.danger, fontSize: fontSize.sm },
});
