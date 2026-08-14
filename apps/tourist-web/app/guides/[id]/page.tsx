import GuideClient from "./GuideClient";

export default async function GuidePage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  return <GuideClient guideId={id} />;
}
