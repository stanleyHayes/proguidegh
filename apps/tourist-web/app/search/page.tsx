import SearchClient from "./SearchClient";

/** Query params arriving from the landing-page search form (and deep links). */
function first(value: string | string[] | undefined): string {
  return typeof value === "string" ? value : "";
}

export default async function SearchPage({
  searchParams,
}: {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}) {
  const params = await searchParams;
  return (
    <SearchClient
      initialDestination={first(params.destination)}
      initialDate={first(params.date)}
    />
  );
}
