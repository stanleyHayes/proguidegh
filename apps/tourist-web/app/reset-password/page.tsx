import { Suspense } from "react";
import { ResetPasswordForm } from "./ResetPasswordForm";

export default function ResetPasswordPage() {
  return (
    <Suspense
      fallback={
        <div className="stack" aria-busy="true" aria-label="Loading">
          <div className="gg-skeleton" style={{ height: "2rem", width: "50%" }} />
          <div className="gg-skeleton" style={{ height: "2.75rem" }} />
          <div className="gg-skeleton" style={{ height: "2.75rem" }} />
        </div>
      }
    >
      <ResetPasswordForm />
    </Suspense>
  );
}
