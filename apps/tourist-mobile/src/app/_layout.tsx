import { useEffect } from "react";
import { Stack, useRouter, useSegments } from "expo-router";
import { StatusBar } from "expo-status-bar";
import { SafeAreaProvider } from "react-native-safe-area-context";
import { colors, fontSize } from "@proguidegh/tokens";
import { SessionProvider, useSession } from "@/lib/session";
import { OnlineProvider } from "@/lib/offline";

/**
 * Root layout for the ProGuideGH tourist app (Phase M, M-07/M-12).
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
        headerStyle: { backgroundColor: colors.primary },
        headerTintColor: colors.surface,
        headerTitleStyle: { fontSize: fontSize.lg, fontWeight: "600" },
        contentStyle: { backgroundColor: colors.surface },
      }}
    >
      <Stack.Screen name="index" options={{ title: "ProGuideGH" }} />
      <Stack.Screen
        name="login"
        options={{ title: "Sign in", headerShown: false }}
      />
      <Stack.Screen name="search" options={{ title: "Find a guide" }} />
      <Stack.Screen name="guide/[id]" options={{ title: "Guide profile" }} />
      <Stack.Screen name="book/[id]" options={{ title: "Book a tour" }} />
      <Stack.Screen
        name="checkout/[id]"
        options={{ title: "Checkout", gestureEnabled: false }}
      />
      <Stack.Screen name="bookings/index" options={{ title: "My bookings" }} />
      <Stack.Screen name="bookings/[id]" options={{ title: "Booking" }} />
      <Stack.Screen name="profile" options={{ title: "Profile" }} />
      <Stack.Screen name="privacy" options={{ title: "Privacy & data" }} />
    </Stack>
  );
}

export default function RootLayout() {
  return (
    <SafeAreaProvider>
      <StatusBar style="light" />
      <SessionProvider>
        <OnlineProvider>
          <RouteGate />
        </OnlineProvider>
      </SessionProvider>
    </SafeAreaProvider>
  );
}
