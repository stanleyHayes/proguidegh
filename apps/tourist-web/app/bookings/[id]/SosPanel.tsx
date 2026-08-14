"use client";

import { useState } from "react";
import { Alert, Button, Card } from "@proguidegh/ui";
import { api, errorMessage } from "../../lib/api";

/**
 * SOS panel (spec §12). Visible only while the booking is active.
 *
 * §12 step 7: immediately send the freshest coordinates. We take a fresh
 * fix on tap — never a cached one — and the button stays retryable on
 * failure (step 7: "retry if network unavailable"). The API requires
 * coordinates, so when the device cannot provide any, the SOS cannot be
 * sent and we say so plainly rather than sending a fake position.
 *
 * The copy deliberately names ProGuideGH operations — never police or
 * emergency services (§12 safety requirement).
 */

const ACTIVE = new Set([
  "CONFIRMED",
  "GUIDE_EN_ROUTE",
  "GUIDE_ARRIVED",
  "IN_PROGRESS",
]);

function freshestPosition(): Promise<GeolocationPosition> {
  return new Promise((resolve, reject) => {
    if (!("geolocation" in navigator)) {
      reject(new Error("no-geolocation"));
      return;
    }
    navigator.geolocation.getCurrentPosition(resolve, reject, {
      enableHighAccuracy: true,
      timeout: 10000,
      maximumAge: 0, // freshest fix only — §12 step 7
    });
  });
}

export default function SosPanel({
  bookingId,
  status,
}: {
  bookingId: string;
  status?: string;
}) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [sent, setSent] = useState<string | null>(null);

  if (!status || !ACTIVE.has(status)) return null;

  async function trigger() {
    const confirmed = window.confirm(
      "Send an SOS to ProGuideGH operations with your current location?",
    );
    if (!confirmed) return;

    setBusy(true);
    setError(null);
    setSent(null);
    try {
      let position: GeolocationPosition;
      try {
        position = await freshestPosition();
      } catch {
        throw new Error(
          "We could not get your location. Enable location services and try again — an SOS needs your coordinates.",
        );
      }
      const data = await api<{ message?: string }>(`/bookings/${bookingId}/sos`, {
        method: "POST",
        body: {
          latitude: position.coords.latitude,
          longitude: position.coords.longitude,
          accuracy_m: position.coords.accuracy ?? undefined,
        },
      });
      setSent(
        data.message ??
          "Your SOS has been sent to ProGuideGH operations with your location.",
      );
    } catch (err) {
      setError(
        errorMessage(err, "The SOS could not be sent. Try again immediately."),
      );
    } finally {
      setBusy(false);
    }
  }

  return (
    <Card title="Emergency">
      {sent ? (
        <Alert tone="success" title="SOS sent">
          <p>{sent}</p>
        </Alert>
      ) : (
        <>
          <p className="muted">
            Alerts ProGuideGH operations with your live location. For
            life-threatening emergencies call local emergency services first —
            this button does not dispatch police or ambulance.
          </p>
          {error ? <Alert tone="error">{error}</Alert> : null}
          <Button type="button" disabled={busy} onClick={() => void trigger()}>
            {busy ? "Getting your location…" : "Send SOS"}
          </Button>
        </>
      )}
    </Card>
  );
}
