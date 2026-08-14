/**
 * Session management for the tourist app (Phase M, M-07).
 *
 * Tokens live only in expo-secure-store (iOS Keychain / Android Keystore)
 * per ADR 0009 — never AsyncStorage, never logs. Refresh rotation and
 * reuse detection happen server-side; on an unrecoverable 401 the client
 * clears state and routes to /login via onSessionExpired.
 */
import { createContext, useContext, useEffect, useMemo, useState } from "react";
import type { ReactNode } from "react";
import * as SecureStore from "expo-secure-store";
import Constants from "expo-constants";
import {
  createApiClient,
  errorMessage,
  type ApiClient,
  type TokenPair,
} from "@proguidegh/api-client";

const ACCESS_KEY = "pgh_access_token";
const REFRESH_KEY = "pgh_refresh_token";

/** expo-secure-store backed TokenStore (ADR 0009 §2). */
const secureTokenStore = {
  async getTokens(): Promise<TokenPair | null> {
    const [accessToken, refreshToken] = await Promise.all([
      SecureStore.getItemAsync(ACCESS_KEY),
      SecureStore.getItemAsync(REFRESH_KEY),
    ]);
    return accessToken && refreshToken ? { accessToken, refreshToken } : null;
  },
  async setTokens(tokens: TokenPair): Promise<void> {
    await Promise.all([
      SecureStore.setItemAsync(ACCESS_KEY, tokens.accessToken),
      SecureStore.setItemAsync(REFRESH_KEY, tokens.refreshToken),
    ]);
  },
  async clear(): Promise<void> {
    await Promise.all([
      SecureStore.deleteItemAsync(ACCESS_KEY),
      SecureStore.deleteItemAsync(REFRESH_KEY),
    ]);
  },
};

function resolveApiUrl(): string {
  // EXPO_PUBLIC_* is inlined at build time, which is how EAS injects
  // per-environment config (`eas secret:create`, or apps/*/.env.production
  // for a local production build). app.json's extra.apiUrl stays as the
  // fallback so a plain `expo start` still works with no env file.
  const fromEnv = process.env.EXPO_PUBLIC_API_URL;
  if (fromEnv) return fromEnv.replace(/\/+$/, "");
  const extra = Constants.expoConfig?.extra as { apiUrl?: string } | undefined;
  return (extra?.apiUrl ?? "http://localhost:8080").replace(/\/+$/, "");
}

export type SessionStatus = "loading" | "unauthenticated" | "authenticated";

interface Session {
  status: SessionStatus;
  client: ApiClient;
  signIn: (email: string, password: string) => Promise<void>;
  /** Complete an MFA step-up after signIn reports mfaRequired. */
  signInMfa: (challenge: string, code: string) => Promise<void>;
  signOut: () => Promise<void>;
}

const SessionContext = createContext<Session | null>(null);

export function SessionProvider({ children }: { children: ReactNode }) {
  const [status, setStatus] = useState<SessionStatus>("loading");

  const client = useMemo<ApiClient>(
    () =>
      createApiClient({
        baseUrl: resolveApiUrl(),
        tokenStore: secureTokenStore,
        onSessionExpired: () => setStatus("unauthenticated"),
      }),
    [],
  );

  useEffect(() => {
    let cancelled = false;
    client
      .hasSession()
      .then((present) => {
        if (!cancelled) setStatus(present ? "authenticated" : "unauthenticated");
      })
      .catch(() => {
        if (!cancelled) setStatus("unauthenticated");
      });
    return () => {
      cancelled = true;
    };
  }, [client]);

  const session = useMemo<Session>(
    () => ({
      status,
      client,
      async signIn(email, password) {
        const result = await client.login({ email, password });
        if (result.mfaRequired) {
          throw Object.assign(new Error("MFA_REQUIRED"), {
            challenge: result.challenge ?? "",
          });
        }
        setStatus("authenticated");
      },
      async signInMfa(challenge, code) {
        await client.loginMfa(challenge, code);
        setStatus("authenticated");
      },
      async signOut() {
        await client.logout();
        setStatus("unauthenticated");
      },
    }),
    [status, client],
  );

  return (
    <SessionContext.Provider value={session}>
      {children}
    </SessionContext.Provider>
  );
}

export function useSession(): Session {
  const session = useContext(SessionContext);
  if (!session) throw new Error("useSession outside SessionProvider");
  return session;
}

export { errorMessage };
