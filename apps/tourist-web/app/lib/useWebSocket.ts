/**
 * Small reusable live-channel hook (spec §11, §18.4).
 *
 * Duplicated per app on purpose: apps must not share code outside packages/.
 * Keep this file identical across apps.
 *
 * Behaviour:
 * - Opens a WebSocket (cookies ride along on the upgrade request).
 * - Auto-reconnects with exponential backoff up to `maxReconnectAttempts`.
 * - When the socket is not live (connecting, reconnecting or given up),
 *   invokes `onPoll` every `pollIntervalMs` as the REST fallback — the list
 *   fallback is the primary representation, the push channel is an upgrade.
 */

import { useEffect, useRef, useState } from "react";
import { API_BASE_URL } from "./api";

export type LiveStatus = "connecting" | "live" | "polling";

export interface UseWebSocketOptions {
  /** Socket path, e.g. "/ws/guide". */
  path: string;
  /** Handle one parsed JSON message from the socket. */
  onMessage: (data: unknown) => void;
  /** REST fallback polled while the socket is unavailable. */
  onPoll?: () => void | Promise<void>;
  pollIntervalMs?: number;
  maxReconnectAttempts?: number;
  /** Set false to close the channel and stop polling. */
  enabled?: boolean;
}

/** Derive the WS base URL from the HTTP API base (http→ws, https→wss). */
export function wsUrl(path: string): string {
  return `${API_BASE_URL.replace(/^http/, "ws")}${path}`;
}

export function useWebSocket({
  path,
  onMessage,
  onPoll,
  pollIntervalMs = 5000,
  maxReconnectAttempts = 5,
  enabled = true,
}: UseWebSocketOptions): LiveStatus {
  const [status, setStatus] = useState<LiveStatus>("connecting");

  // Keep the latest callbacks in refs so reconnects never go stale and the
  // connection effect does not re-run on every render.
  const onMessageRef = useRef(onMessage);
  const onPollRef = useRef(onPoll);
  onMessageRef.current = onMessage;
  onPollRef.current = onPoll;

  useEffect(() => {
    if (!enabled || typeof WebSocket === "undefined") {
      setStatus(enabled ? "polling" : "connecting");
      return undefined;
    }

    let socket: WebSocket | null = null;
    let attempts = 0;
    let gaveUp = false;
    let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
    let disposed = false;

    function connect() {
      if (disposed) return;
      setStatus("connecting");
      try {
        socket = new WebSocket(wsUrl(path));
      } catch {
        scheduleReconnect();
        return;
      }

      socket.onopen = () => {
        attempts = 0;
        setStatus("live");
      };

      socket.onmessage = (event) => {
        try {
          const data: unknown = JSON.parse(String(event.data));
          onMessageRef.current(data);
        } catch {
          // Ignore malformed frames; the polling fallback keeps data fresh.
        }
      };

      socket.onclose = () => {
        socket = null;
        scheduleReconnect();
      };

      socket.onerror = () => {
        // onclose follows onerror in practice; close defensively otherwise.
        socket?.close();
      };
    }

    function scheduleReconnect() {
      if (disposed || reconnectTimer) return;
      attempts += 1;
      if (attempts > maxReconnectAttempts) {
        gaveUp = true;
        setStatus("polling");
        return;
      }
      setStatus("polling");
      const delay = Math.min(500 * 2 ** (attempts - 1), 10000);
      reconnectTimer = setTimeout(() => {
        reconnectTimer = null;
        connect();
      }, delay);
    }

    connect();

    // REST fallback: poll whenever the socket is not live.
    const pollTimer = setInterval(() => {
      if (disposed) return;
      if (!gaveUp && socket && socket.readyState === WebSocket.OPEN) return;
      void onPollRef.current?.();
    }, pollIntervalMs);

    return () => {
      disposed = true;
      if (reconnectTimer) clearTimeout(reconnectTimer);
      clearInterval(pollTimer);
      socket?.close();
    };
  }, [path, pollIntervalMs, maxReconnectAttempts, enabled]);

  return status;
}
