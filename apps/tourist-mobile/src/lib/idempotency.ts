/**
 * Idempotency keys for replay-safe mutations (Spec §1.2 #9, P4-03).
 *
 * The API requires an Idempotency-Key on booking creation and payment
 * initiation. The key must stay STABLE across retries of the same logical
 * action — that is the whole point: a retry after a timeout must return the
 * original booking rather than create a second one. Generate once per
 * user intent, hold it, and only mint a new one when the inputs change.
 */
import { randomUUID } from "expo-crypto";

export function newIdempotencyKey(): string {
  return randomUUID();
}

/**
 * Holds one key per input signature. Calling with the same signature returns
 * the same key (safe retry); a different signature mints a new one, because
 * reusing a key with a changed payload is a 409 conflict, not a replay.
 */
export function createIdempotencyKeeper() {
  let signature: string | null = null;
  let key: string | null = null;
  return function keyFor(nextSignature: string): string {
    if (signature !== nextSignature || key === null) {
      signature = nextSignature;
      key = newIdempotencyKey();
    }
    return key;
  };
}
