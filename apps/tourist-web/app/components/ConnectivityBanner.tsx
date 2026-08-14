"use client";

import { useEffect, useState } from "react";
import { Alert } from "@proguidegh/ui";

/**
 * Connectivity banner (P8-05 offline/retry UX): warns when the browser goes
 * offline (the service worker serves cached pages, but API calls will fail)
 * and confirms recovery so the user knows to retry the failed action.
 */
export function ConnectivityBanner() {
  const [online, setOnline] = useState(true);
  const [recovered, setRecovered] = useState(false);

  useEffect(() => {
    const handleOffline = () => {
      setOnline(false);
      setRecovered(false);
    };
    const handleOnline = () => {
      setOnline(true);
      setRecovered(true);
    };
    setOnline(navigator.onLine);
    window.addEventListener("offline", handleOffline);
    window.addEventListener("online", handleOnline);
    return () => {
      window.removeEventListener("offline", handleOffline);
      window.removeEventListener("online", handleOnline);
    };
  }, []);

  useEffect(() => {
    if (!recovered) return;
    const timer = setTimeout(() => setRecovered(false), 5000);
    return () => clearTimeout(timer);
  }, [recovered]);

  if (!online) {
    return (
      <Alert tone="error">
        You are offline. Cached pages still open, but actions that need the
        server will fail — retry them once you are back online.
      </Alert>
    );
  }
  if (recovered) {
    return <Alert tone="success">Back online — retry the failed action.</Alert>;
  }
  return null;
}
