import TourClient from "./TourClient";

export default async function TourPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  return <TourClient bookingId={id} />;
}
