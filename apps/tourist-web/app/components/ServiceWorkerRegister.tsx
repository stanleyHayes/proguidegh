"use client";

import { useEffect } from "react";

/** Registers /sw.js after load; no-ops where service workers are unsupported. */
export function ServiceWorkerRegister() {
  useEffect(() => {
    if (!("serviceWorker" in navigator)) return;
    navigator.serviceWorker.register("/sw.js").catch(() => {
      // Offline support is progressive enhancement — never block the app.
    });
  }, []);
  return null;
}
