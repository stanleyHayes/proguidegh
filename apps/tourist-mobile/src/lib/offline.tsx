/**
 * Connectivity state + offline banner (Phase M, M-12; Spec §20 requires
 * explicit offline/retry states).
 *
 * This app has no offline write queue. That was a deliberate call: queuing a
 * booking created against a quote fetched an hour ago would replay a stale,
 * server-authoritative price and race the availability check (Spec §1.2).
 * Offline therefore means read-what-you-have and block writes with a clear
 * reason — never a silent failure, never an optimistic success.
 */
import { createContext, useContext } from "react";
import type { ReactNode } from "react";
import { StyleSheet, Text, View } from "react-native";
import * as Network from "expo-network";
import { colors, fontSize, space } from "@proguidegh/tokens";

/** True when the device has a usable internet connection. */
export function useIsOnline(): boolean {
  const state = Network.useNetworkState();
  // Undefined while the first probe is in flight — assume online so the UI
  // does not flash an offline banner on every cold start.
  if (state.isInternetReachable === false) return false;
  if (state.isConnected === false) return false;
  return true;
}

const OnlineContext = createContext<boolean>(true);

export function OnlineProvider({ children }: { children: ReactNode }) {
  const online = useIsOnline();
  return (
    <OnlineContext.Provider value={online}>
      {!online ? <OfflineBanner /> : null}
      {children}
    </OnlineContext.Provider>
  );
}

/** Read connectivity without re-subscribing in every screen. */
export function useOnline(): boolean {
  return useContext(OnlineContext);
}

export function OfflineBanner() {
  return (
    <View accessibilityLiveRegion="polite" style={styles.banner}>
      <Text style={styles.text}>
        You are offline. Showing the last information loaded — booking and
        payment need a connection.
      </Text>
    </View>
  );
}

const styles = StyleSheet.create({
  banner: {
    backgroundColor: colors.warning,
    paddingHorizontal: space[4],
    paddingVertical: space[2],
  },
  text: {
    color: colors.surface,
    fontSize: fontSize.sm,
    lineHeight: fontSize.sm * 1.4,
  },
});
