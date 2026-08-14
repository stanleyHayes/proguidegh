# ADR 0009: Native session storage (Expo secure store, bearer transport)

- Status: Accepted
- Date: 2026-08-13
- Spec reference: §15.1 (session model), §31; ADR 0007 consequence clause ("Bearer-token API clients… would need a separate mechanism and ADR"); agent_plan.md Phase M (M-05/M-06/M-07).

## Context

ADR 0007 established the session model for browser clients: short-lived
access tokens plus rotating refresh tokens in HttpOnly, SameSite cookies.
Native mobile clients (Phase M, Expo/React Native) do not participate in
browser cookie jars: `Set-Cookie` is not reliably persisted across app
restarts, and the app must present credentials explicitly on every request.
Without a decision here, native clients would either (a) be forced through
a cookie shim that breaks rotation and logout, or (b) each app would invent
its own token storage, diverging on exactly the properties ADR 0007 exists
to guarantee (rotation, reuse detection, revocation).

## Decision

1. **Transport:** native clients send the access token as
   `Authorization: Bearer <token>` and the refresh token via the
   `X-Refresh-Token` header (or `refresh_token` JSON body field) on
   `/api/v1/auth/refresh` and `/api/v1/auth/logout`. The server accepts all
   transports with identical semantics (implemented in M-05;
   `platform/auth.RefreshFromRequest` header → body → cookie priority).
2. **Storage:** both tokens are stored only in `expo-secure-store`
   (iOS Keychain / Android Keystore-backed EncryptedSharedPreferences).
   Never in AsyncStorage, never in JS memory beyond the session context,
   never logged.
3. **Rotation:** the client replaces BOTH stored tokens on every refresh
   response before making further calls. A refresh that fails with
   `SESSION_REUSE`/401 clears local tokens and routes to login — reuse
   detection stays server-side and transport-agnostic.
4. **Logout / revocation:** logout calls the API with the refresh token via
   header (server revokes the session chain), then deletes local tokens.
   Role removal and compromise flows revoke server-side per ADR 0007; the
   client discovers this on the next 401 and clears state.
5. **MFA:** the MFA challenge flow is transport-identical for native; the
   challenge token is held in memory only.

## Consequences

- One session service on the backend serves web and native with no
  behavioral divergence; rotation/reuse/revocation tests cover both
  transports.
- Token-at-rest security relies on the OS keystore; jailbroken/rooted
  devices are out of threat-model scope for V1 (consistent with §15).
- Cookies remain the web transport; native never sets or reads them.
- EAS/dev-client builds must not bundle real tokens; staging vs production
  API base URLs are build-time config.
