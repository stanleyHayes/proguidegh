/**
 * Guide dashboard (Phase M, M-13) — the online/offline switch that puts a
 * guide into the dispatch pool (P5-01) plus entry points to jobs and tours.
 */
import { ScrollView, StyleSheet, Text, View } from "react-native";
import { useRouter } from "expo-router";
import { colors, fontSize, radius, space } from "@proguidegh/tokens";
import { useSession } from "@/lib/session";
import { usePresence } from "@/lib/presence";
import { Card, ErrorState, PrimaryButton } from "@/lib/ui";

export default function DashboardScreen() {
  const { signOut } = useSession();
  const { online, busy, error, setOnline } = usePresence();
  const router = useRouter();

  return (
    <ScrollView contentContainerStyle={styles.page}>
      <Card>
        <View style={styles.statusRow}>
          <View style={[styles.dot, online ? styles.dotOnline : styles.dotOffline]} />
          <Text style={styles.status}>{online ? "You are online" : "You are offline"}</Text>
        </View>
        <Text style={styles.muted}>
          {online
            ? "You can receive nearby tour offers. Keep the app installed and signed in — going offline stops new offers immediately."
            : "Go online to start receiving nearby tour offers. Your location is only shared once you are online or on a tour."}
        </Text>
        {error ? <ErrorState message={error} /> : null}
        <PrimaryButton
          busy={busy}
          label={online ? "Go offline" : "Go online"}
          onPress={() => void setOnline(!online)}
        />
      </Card>

      <PrimaryButton label="Job offers" onPress={() => router.push("/jobs")} />
      <PrimaryButton label="My tours" onPress={() => router.push("/tours")} />
      <PrimaryButton
        label="Location sharing"
        onPress={() => router.push("/location-permission")}
      />
      <PrimaryButton
        label="Privacy & data"
        onPress={() => router.push("/privacy")}
      />
      <PrimaryButton label="Sign out" onPress={() => void signOut()} />
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  page: { gap: space[3], padding: space[4] },
  statusRow: { alignItems: "center", flexDirection: "row", gap: space[2] },
  dot: { borderRadius: radius.full, height: 12, width: 12 },
  dotOnline: { backgroundColor: colors.success },
  dotOffline: { backgroundColor: colors.muted },
  status: { color: colors.ink, fontSize: fontSize.lg, fontWeight: "700" },
  muted: {
    color: colors.muted,
    fontSize: fontSize.sm,
    lineHeight: fontSize.sm * 1.5,
  },
});
