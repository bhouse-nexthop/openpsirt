import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import { unwrap } from "../api/queries";

// Where a judgment lands, in three parts. Presenting it as one number is what
// turns a considered decision into a reflex, and the first two parts are not
// choices at all — they follow from the matching rules and are there to be
// told, not agreed to.
export function Reach({
  at,
}: {
  at: { product: string; stream: string; variant: string; vulnerability: string; place: string };
}) {
  const reach = useQuery({
    queryKey: ["reach", at],
    queryFn: async () =>
      unwrap(
        await api.GET(
          "/v1/products/{product}/streams/{stream}/variants/{variant}/findings/{vulnerability}/places/{place}/reach",
          { params: { path: at } },
        ),
      ),
  });

  if (reach.isPending || reach.isError || !reach.data) return null;

  const here = reach.data.here ?? 0;
  const automatic = reach.data.automatic ?? [];
  const differing = reach.data.differing ?? [];

  return (
    <section className="mb-5 rounded-lg border border-[var(--line)] bg-[var(--raised)] p-3 text-sm">
      <h2 className="mb-2 font-semibold">Scope</h2>
      <ul className="flex flex-col gap-1">
        <li>
          <strong>{here}</strong> {here === 1 ? "location" : "locations"} in this build, one decision.
        </li>
        <li>
          <strong>{automatic.length}</strong> other{" "}
          {automatic.length === 1 ? "build" : "builds"} already match, so it reaches them automatically.
        </li>
        <li>
          <strong>{differing.length}</strong>{" "}
          {differing.length === 1 ? "build holds" : "builds hold"} the same issue at a different
          version. Each of those is a separate judgment and none is covered here.
        </li>
      </ul>

      {differing.length > 0 && (
        <details className="mt-2">
          <summary className="cursor-pointer text-[var(--muted)]">Builds at other versions</summary>
          <ul className="mt-1 flex flex-col gap-0.5 text-[var(--muted)]">
            {differing.map((match) => (
              <li key={`${match.stream}-${match.variant}-${match.version}`}>
                {match.stream} / {match.variant} — has {match.version}
              </li>
            ))}
          </ul>
        </details>
      )}
    </section>
  );
}
