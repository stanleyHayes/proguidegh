import { Suspense } from "react";
import { ResetPasswordForm } from "./ResetPasswordForm";

export default function ResetPasswordPage() { return <Suspense fallback={<div className="gg-skeleton" style={{ minHeight: "100dvh" }} />}><ResetPasswordForm /></Suspense>; }
