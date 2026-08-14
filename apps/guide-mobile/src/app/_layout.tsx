import { useEffect } from "react";
import { Stack, useRouter, useSegments } from "expo-router";
import { StatusBar } from "expo-status-bar";
import { SafeAreaProvider } from "react-native-safe-area-context";
import { colors, fontSize } from "@proguidegh/tokens";
import { SessionProvider, useSession } from "@/lib/session";
import { PresenceProvider } from "@/lib/presence";
// Side-effect import: TaskManager.defineTask must run during bundle
// evaluation so the OS can relaunch the app headlessly into the background
// location task (M-15). Removing this import silently disables tracking.
import "@/lib/location-task";

/**
 * Root layout for the ProGuideGH guide app (Phase M, M-07/M-13).
 *
 * Every route except /login is protected: unauthenticated sessions are
 * redirected to /login, and authenticated users landing on /login are sent
 * home. While the session restores from expo-secure-store no redirect fires.
 */
function RouteGate() {
  const { status } = useSession();
  const segments = useSegments();
  const router = useRouter();

  useEffect(() => {
    if (status === "loading") return;
    const onLogin = segments[0] === "login";
    if (status === "unauthenticated" && !onLogin) {
      router.replace("/login");
    } else if (status === "authenticated" && onLogin) {
      router.replace("/");
    }
  }, [status, segments, router]);

  return (
    <Stack
      screenOptions={{
        headerStyle: { backgroundColor: colors.primaryStrong },
        headerTintColor: colors.surface,
        headerTitleStyle: { fontSize: fontSize.lg, fontWeight: "700" },
        headerShadowVisible: false,
        contentStyle: { backgroundColor: colors.surfaceAlt },
      }}
    >
      <Stack.Screen name="index" options={{ title: "ProGuideGH Guides" }} />
      <Stack.Screen
        name="login"
        options={{ title: "Sign in", headerShown: false }}
      />
      <Stack.Screen name="jobs" options={{ title: "Job offers" }} />
      <Stack.Screen name="tours/index" options={{ title: "My tours" }} />
      <Stack.Screen name="tours/[id]" options={{ title: "Tour" }} />
      <Stack.Screen
        name="location-permission"
        options={{ title: "Location sharing" }}
      />
      <Stack.Screen name="privacy" options={{ title: "Privacy & data" }} />
    </Stack>
  );
}

export default function RootLayout() {
  return (
    <SafeAreaProvider>
      <StatusBar style="light" />
      <SessionProvider>
        <PresenceProvider>
          <RouteGate />
        </PresenceProvider>
      </SessionProvider>
    </SafeAreaProvider>
  );
}
