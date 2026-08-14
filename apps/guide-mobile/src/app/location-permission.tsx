/**
 * Location sharing rationale (Phase M, M-15).
 *
 * This screen is a store-review requirement, not a nicety. Apple (5.1.1) and
 * Google (Background Location policy) both reject apps that request background
 * location without an in-app explanation of what is collected, when, and why —
 * shown BEFORE the OS prompt. §M.4 note 4 requires it to ship with M-15 rather
 * than be retrofitted into the store listing at M-18.
 *
 * It is also the honest place to state the limits we actually enforce in
 * lib/location-task.ts: collection only inside an active tour window.
 */
import { useCallback, useEffect, useState } from "react";
import { Linking, ScrollView, StyleSheet, Text, View } from "react-native";
import { colors, fontSize, radius, space } from "@proguidegh/tokens";
import {
  isBackgroundLocationRunning,
  startBackgroundLocation,
  stopBackgroundLocation,
} from "@/lib/location-task";
import { Card, ErrorState, PrimaryButton } from "@/lib/ui";

export default function LocationPermissionScreen() {
  const [running, setRunning] = useState<boolean | null>(null);
  const [busy, setBusy] = useState(false);
  const [denied, setDenied] = useState(false);

  const refresh = useCallback(() => {
    void isBackgroundLocationRunning().then(setRunning);
  }, []);

  useEffect(() => {
    const t = setTimeout(refresh, 0);
    return () => clearTimeout(t);
  }, [refresh]);

  async function enable() {
    setBusy(true);
    setDenied(false);
    const granted = await startBackgroundLocation();
    if (!granted) setDenied(true);
    refresh();
    setBusy(false);
  }

  async function disable() {
    setBusy(true);
    await stopBackgroundLocation();
    refresh();
    setBusy(false);
  }

  return (
    <ScrollView contentContainerStyle={styles.page}>
      <Text style={styles.heading}>Location sharing</Text>

      <Card>
        <Text style={styles.sectionTitle}>What we collect</Text>
        <Bullet text="Your position, roughly every 15 seconds or 25 metres." />
        <Bullet text="Only while you are online, or on a tour that has started." />
        <Bullet text="Never when you are offline, and never between tours." />
      </Card>

      <Card>
        <Text style={styles.sectionTitle}>Who can see it</Text>
        <Bullet text="The tourist on your active tour, so they can find you." />
        <Bullet text="ProGuideGH operations, for dispatch and emergency response." />
        <Bullet text="Nobody else. Your movement history is not shown publicly or sold." />
      </Card>

      <Card>
        <Text style={styles.sectionTitle}>Why background access</Text>
        <Text style={styles.body}>
          Your phone locks and your screen turns off during a tour. Without
          background access your tourist loses sight of you exactly when it matters
          most — while you are travelling to meet them, and during an emergency.
        </Text>
      </Card>

      {denied ? (
        <ErrorState
          message={
            "Location permission was not granted. You can still take bookings, but you will not " +
            "receive dispatched jobs. Enable “Always” location for ProGuideGH in Settings."
          }
        />
      ) : null}

      {running === true ? (
        <>
          <Text style={styles.active}>Background sharing is on.</Text>
          <PrimaryButton
            busy={busy}
            label="Turn off location sharing"
            onPress={() => void disable()}
          />
        </>
      ) : (
        <PrimaryButton
          busy={busy}
          label="Enable location sharing"
          onPress={() => void enable()}
        />
      )}

      <PrimaryButton
        label="Open phone settings"
        onPress={() => void Linking.openSettings()}
      />
    </ScrollView>
  );
}

function Bullet({ text }: { text: string }) {
  return (
    <View style={styles.bullet}>
      <View style={styles.dot} />
      <Text style={styles.body}>{text}</Text>
    </View>
  );
}

const styles = StyleSheet.create({
  page: { gap: space[3], padding: space[4] },
  heading: { color: colors.ink, fontSize: fontSize["2xl"], fontWeight: "700" },
  sectionTitle: { color: colors.ink, fontSize: fontSize.base, fontWeight: "600" },
  body: {
    color: colors.muted,
    flexShrink: 1,
    fontSize: fontSize.sm,
    lineHeight: fontSize.sm * 1.5,
  },
  bullet: { flexDirection: "row", gap: space[2] },
  dot: {
    backgroundColor: colors.primary,
    borderRadius: radius.full,
    height: 6,
    marginTop: 7,
    width: 6,
  },
  active: { color: colors.success, fontSize: fontSize.base, fontWeight: "600" },
});
