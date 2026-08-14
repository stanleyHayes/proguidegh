/**
 * Background location task (Phase M, M-15).
 *
 * This file is the entire reason ProGuideGH went native instead of PWA
 * (decision D5): dispatch and in-tour tracking need position updates while the
 * app is backgrounded, which no iOS PWA can provide.
 *
 * Registered at module load — TaskManager requires the task to be defined in
 * the global scope before the JS bundle finishes evaluating, so that the OS can
 * relaunch the app headlessly into it.
 *
 * Privacy rules enforced here, not just in the UI:
 * - Updates are POSTed only while a booking id is set. Going offline or
 *   finishing a tour clears it, and the task then discards every fix instead of
 *   sending it. Collection outside the active window would be both a §11.1
 *   violation and an app-store rejection.
 * - Nothing is persisted to device storage; a fix that cannot be delivered is
 *   dropped, not queued. Stale positions are worse than none for dispatch.
 */
import * as Location from "expo-location";
import * as TaskManager from "expo-task-manager";
import * as SecureStore from "expo-secure-store";
import Constants from "expo-constants";
import { colors } from "@proguidegh/tokens";

export const LOCATION_TASK = "proguidegh-guide-location";

const ACCESS_KEY = "pgh_access_token";
/** Booking currently authorising background collection; null = collect nothing. */
const ACTIVE_BOOKING_KEY = "pgh_active_booking_id";

function apiUrl(): string {
  const extra = Constants.expoConfig?.extra as { apiUrl?: string } | undefined;
  return (extra?.apiUrl ?? "http://localhost:8080").replace(/\/+$/, "");
}

/**
 * Authorise background collection for one booking. Called when a tour enters
 * the §11.1 window; pass null the moment it leaves.
 */
export async function setActiveBooking(bookingId: string | null): Promise<void> {
  if (bookingId === null) {
    await SecureStore.deleteItemAsync(ACTIVE_BOOKING_KEY);
    return;
  }
  await SecureStore.setItemAsync(ACTIVE_BOOKING_KEY, bookingId);
}

export async function getActiveBooking(): Promise<string | null> {
  return SecureStore.getItemAsync(ACTIVE_BOOKING_KEY);
}

interface LocationTaskData {
  locations?: Location.LocationObject[];
}

TaskManager.defineTask<LocationTaskData>(LOCATION_TASK, async ({ data, error }) => {
  if (error || !data?.locations?.length) return;

  // No active tour ⇒ we are not permitted to collect. Drop the fix.
  const bookingId = await SecureStore.getItemAsync(ACTIVE_BOOKING_KEY).catch(
    () => null,
  );
  if (!bookingId) return;

  const token = await SecureStore.getItemAsync(ACCESS_KEY).catch(() => null);
  if (!token) return;

  const latest = data.locations[data.locations.length - 1];
  if (!latest) return;

  await fetch(`${apiUrl()}/api/v1/bookings/${bookingId}/location`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${token}`,
    },
    body: JSON.stringify({
      latitude: latest.coords.latitude,
      longitude: latest.coords.longitude,
      accuracy_m: latest.coords.accuracy ?? undefined,
      heading:
        latest.coords.heading !== null && latest.coords.heading >= 0
          ? latest.coords.heading
          : undefined,
      speed_mps:
        latest.coords.speed !== null && latest.coords.speed >= 0
          ? latest.coords.speed
          : undefined,
      captured_at: new Date(latest.timestamp).toISOString(),
    }),
    // A dropped fix is fine — the next one supersedes it. Never queue.
  }).catch(() => undefined);
});

/**
 * Start background updates. Requires foreground permission first, then
 * background: iOS will not grant "Always" unless "When In Use" is held.
 * Returns false if the guide declined, so callers can surface the rationale
 * screen rather than silently running with no tracking.
 */
export async function startBackgroundLocation(): Promise<boolean> {
  const foreground = await Location.requestForegroundPermissionsAsync();
  if (foreground.status !== "granted") return false;

  const background = await Location.requestBackgroundPermissionsAsync();
  if (background.status !== "granted") return false;

  const already = await Location.hasStartedLocationUpdatesAsync(LOCATION_TASK);
  if (already) return true;

  await Location.startLocationUpdatesAsync(LOCATION_TASK, {
    accuracy: Location.Accuracy.Balanced,
    timeInterval: 15000,
    distanceInterval: 25,
    pausesUpdatesAutomatically: false,
    // Android requires a visible, persistent notification while collecting in
    // the background. Play Store rejects silent background location.
    foregroundService: {
      notificationTitle: "ProGuideGH is sharing your location",
      notificationBody:
        "Your tourist and the operations team can see you while your tour is active.",
      notificationColor: colors.primary,
    },
  });
  return true;
}

export async function stopBackgroundLocation(): Promise<void> {
  await setActiveBooking(null);
  const started = await Location.hasStartedLocationUpdatesAsync(
    LOCATION_TASK,
  ).catch(() => false);
  if (started) {
    await Location.stopLocationUpdatesAsync(LOCATION_TASK).catch(() => undefined);
  }
}

export async function isBackgroundLocationRunning(): Promise<boolean> {
  return Location.hasStartedLocationUpdatesAsync(LOCATION_TASK).catch(() => false);
}
