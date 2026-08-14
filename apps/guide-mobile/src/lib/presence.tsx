/**
 * Guide presence (Phase M, M-13) — POST /me/guide/availability {online}.
 *
 * Presence is a Redis marker with a 300s TTL (P5-01): if the heartbeat stops,
 * the guide silently disappears from dispatch. That is the correct failure
 * mode — a crashed or backgrounded app must not keep receiving jobs — but it
 * means the heartbeat is load-bearing, not decorative. We beat well inside the
 * TTL so one dropped request does not drop the guide offline.
 */
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import type { ReactNode } from "react";
import { AppState } from "react-native";
import { useSession, errorMessage } from "@/lib/session";

/** TTL is 300s server-side; refresh at 90s to survive two failures. */
const HEARTBEAT_MS = 90_000;

interface Presence {
  online: boolean;
  busy: boolean;
  error: string | null;
  setOnline: (next: boolean) => Promise<void>;
}

const PresenceContext = createContext<Presence | null>(null);

export function PresenceProvider({ children }: { children: ReactNode }) {
  const { client, status } = useSession();
  const [online, setOnlineState] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const onlineRef = useRef(false);

  const push = useCallback(
    async (next: boolean) => {
      await client.api("/me/guide/availability", {
        method: "POST",
        body: { online: next },
      });
    },
    [client],
  );

  const setOnline = useCallback(
    async (next: boolean) => {
      setBusy(true);
      setError(null);
      try {
        await push(next);
        onlineRef.current = next;
        setOnlineState(next);
      } catch (err: unknown) {
        setError(
          errorMessage(
            err,
            next
              ? "Could not go online. Check your connection and try again."
              : "Could not go offline.",
          ),
        );
      } finally {
        setBusy(false);
      }
    },
    [push],
  );

  // Heartbeat while online. A failed beat is not surfaced as an error: the
  // next one may succeed, and the TTL gives us two chances before dispatch
  // drops us.
  useEffect(() => {
    if (!online || status !== "authenticated") return;
    const timer = setInterval(() => {
      void push(true).catch(() => undefined);
    }, HEARTBEAT_MS);
    return () => clearInterval(timer);
  }, [online, status, push]);

  // Returning to the foreground after a long pause may have outlived the TTL,
  // so beat immediately rather than waiting up to 90s to become discoverable.
  useEffect(() => {
    const sub = AppState.addEventListener("change", (next) => {
      if (next === "active" && onlineRef.current) {
        void push(true).catch(() => undefined);
      }
    });
    return () => sub.remove();
  }, [push]);

  const value = useMemo<Presence>(
    () => ({ online, busy, error, setOnline }),
    [online, busy, error, setOnline],
  );

  return (
    <PresenceContext.Provider value={value}>{children}</PresenceContext.Provider>
  );
}

export function usePresence(): Presence {
  const presence = useContext(PresenceContext);
  if (!presence) throw new Error("usePresence outside PresenceProvider");
  return presence;
}
