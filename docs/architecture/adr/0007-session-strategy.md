# ADR 0007: Session strategy — short-lived access tokens, rotating refresh, HttpOnly cookies

**Status:** Accepted
**Date:** 2026-08-13
**Spec reference:** §13.1, §15.1, §15.2

## Context

Clients are browser-based Next.js apps (with future native mobile). Sessions must survive page reloads without exposing tokens to XSS, support immediate revocation on compromise or role change, and give privileged staff stricter treatment than tourists.

## Decision

Use short-lived access tokens plus rotating refresh tokens:

- Refresh tokens are stored in `Secure`, `HttpOnly`, `SameSite` cookies for browser clients (§15.1).
- Refresh-session identifiers and revocation state are persisted server-side; every refresh rotates the token and invalidates its predecessor.
- `/api/v1/auth/refresh` rotates; `/api/v1/auth/logout` revokes the session (§13.1).
- Future native mobile clients use secure OS credential storage against the same backend session model (§15.1).
- Admin sessions have a shorter idle timeout than tourist sessions; sessions are suspended/revoked on account compromise or role removal (§15.2).
- MFA is required for Super Admin and finance roles; step-up authentication guards sensitive role, payout-account and refund actions (§15.2).
- Passwords use Argon2id or bcrypt with strong current parameters; login/OTP/reset endpoints are rate-limited (§15.2).

## Consequences

- XSS cannot read tokens from cookie storage; CSRF exposure is bounded by SameSite and must still be covered by origin checks on mutations.
- Token theft of a refresh token is detectable: reuse of a rotated token triggers revocation of the session family.
- Session storage is a new table with revocation semantics; login/logout/rotation paths need dedicated tests including reuse detection.
- Bearer-token API clients, if ever introduced, would need a separate mechanism and ADR.
