import Link from "next/link";
import { Alert } from "@proguidegh/ui";

/**
 * Shown when the API rejects a request with 401/403.
 * Frontend guards are convenience only — the backend enforces RBAC.
 */
export function Unauthorized({ forbidden }: { forbidden?: boolean }) {
  return (
    <div className="stack">
      <Alert tone="error" title={forbidden ? "Not authorized" : "Sign in required"}>
        <p>
          {forbidden
            ? "Your account does not have permission to view this page. Sign in with an authorized admin account."
            : "Your session has expired or you are signed out. Sign in with an authorized admin account to continue."}
        </p>
      </Alert>
      <p>
        <Link className="gg-button gg-button--primary" href="/login">
          Sign in with an authorized account
        </Link>
      </p>
    </div>
  );
}
