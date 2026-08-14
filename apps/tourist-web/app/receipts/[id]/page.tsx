import ReceiptClient from "./ReceiptClient";

export default async function ReceiptPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  return <ReceiptClient bookingId={id} />;
}
