import { Suspense } from "react";
import { AccountWorkspace } from "../../components/AccountWorkspace";

export default function AccountPage() {
  return <Suspense fallback={<div className="gg-skeleton" style={{ height: "24rem" }} />}><AccountWorkspace /></Suspense>;
}
