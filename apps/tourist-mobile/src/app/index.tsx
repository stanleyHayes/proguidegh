import { ScrollView, StyleSheet, Text } from "react-native";
import { colors, fontSize, space } from "@proguidegh/tokens";
import { useRouter } from "expo-router";
import { PrimaryButton } from "@/lib/ui";

/**
 * Tourist home (Phase M) — entry points into the three things a tourist does:
 * find a guide, check a booking, manage their profile.
 */
export default function HomeScreen() {
  const router = useRouter();

  return (
    <ScrollView contentContainerStyle={styles.page}>
      <Text style={styles.heading}>Find a certified guide</Text>
      <Text style={styles.body}>
        Book vetted, government-certified tour guides across Accra, Cape Coast and
        Kumasi. Transparent pricing, tracked tours, and an SOS button on every trip.
      </Text>

      <PrimaryButton label="Search guides" onPress={() => router.push("/search")} />
      <PrimaryButton label="My bookings" onPress={() => router.push("/bookings")} />
      <PrimaryButton label="Profile" onPress={() => router.push("/profile")} />
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  page: { gap: space[4], padding: space[4] },
  heading: { color: colors.ink, fontSize: fontSize["2xl"], fontWeight: "700" },
  body: {
    color: colors.muted,
    fontSize: fontSize.base,
    lineHeight: fontSize.base * 1.5,
  },
});
