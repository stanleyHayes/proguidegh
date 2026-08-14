import { Suspense } from "react";
import CheckoutClient from "./CheckoutClient";

export default async function CheckoutPage({
  params,
}: {
  params: Promise<{ bookingId: string }>;
}) {
  const { bookingId } = await params;
  return (
    <Suspense
      fallback={
        <div
          className="stack"
          aria-busy="true"
          aria-label="Loading your booking"
        >
          <div
            className="gg-skeleton"
            style={{ height: "2rem", width: "40%" }}
          />
          <div className="gg-skeleton" style={{ height: "12rem" }} />
        </div>
      }
    >
      <CheckoutClient bookingId={bookingId} />
    </Suspense>
  );
}
