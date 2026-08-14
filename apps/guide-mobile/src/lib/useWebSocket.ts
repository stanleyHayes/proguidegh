/**
 * Live channel for the guide app (Phase M, M-14; spec §11, §18.4).
 *
 * Differs from the web app's hook in one important way: there are no cookies
 * on React Native, and RN's WebSocket cannot set an Authorization header, so
 * the access token rides in `?token=` — which the server explicitly supports
 * (realtime.go). Tokens are short-lived (15 min).
 *
 * The REST list is the primary representation and the socket is an upgrade
 * (spec §20: "map is never the sole representation"). Whenever the socket is
 * not live we fall back to polling, so a guide never misses a job because a
 * proxy silently dropped the upgrade.
 */
import { useEffect, useRef, useState } from "react";
import { apiBaseUrl, currentAccessToken } from "@/lib/session";

export type LiveStatus = "connecting" | "live" | "polling";

export interface UseWebSocketOptions {
  /** Socket path, e.g. "/ws/guide". */
  path: string;
  onMessage: (data: unknown) => void;
  /** REST fallback, polled whenever the socket is not live. */
  onPoll?: () => void | Promise<void>;
  pollIntervalMs?: number;
  maxReconnectAttempts?: number;
  enabled?: boolean;
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

  // Latest callbacks in refs so reconnects never capture stale closures and
  // the connection effect does not re-run on every render. Synced in an
  // effect rather than during render — a ref write in the render body is
  // unsafe under concurrent rendering.
  const onMessageRef = useRef(onMessage);
  const onPollRef = useRef(onPoll);

  useEffect(() => {
    onMessageRef.current = onMessage;
  }, [onMessage]);

  useEffect(() => {
    onPollRef.current = onPoll;
  }, [onPoll]);

  useEffect(() => {
    if (!enabled) return undefined;

    let socket: WebSocket | null = null;
    let attempts = 0;
    let gaveUp = false;
    let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
    let disposed = false;

    async function connect() {
      if (disposed) return;
      // Yield first so the effect body performs no synchronous setState.
      await Promise.resolve();
      if (disposed) return;
      setStatus("connecting");
      // Re-read the token on every attempt: a reconnect after a long
      // background pause may need the rotated one.
      const token = await currentAccessToken().catch(() => null);
      if (disposed) return;
      if (!token) {
        scheduleReconnect();
        return;
      }
      const base = apiBaseUrl().replace(/^http/, "ws");
      try {
        socket = new WebSocket(`${base}${path}?token=${encodeURIComponent(token)}`);
      } catch {
        scheduleReconnect();
        return;
      }

      socket.onopen = () => {
        attempts = 0;
        setStatus("live");
      };

      socket.onmessage = (event: { data: unknown }) => {
        try {
          onMessageRef.current(JSON.parse(String(event.data)));
        } catch {
          // Malformed frame; the polling fallback keeps the list fresh.
        }
      };

      socket.onclose = () => {
        socket = null;
        scheduleReconnect();
      };

      socket.onerror = () => {
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
        void connect();
      }, delay);
    }

    void connect();

    const pollTimer = setInterval(() => {
      if (disposed) return;
      if (!gaveUp && socket && socket.readyState === 1) return;
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
