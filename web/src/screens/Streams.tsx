import { useQuery } from "@tanstack/react-query";
import { Link, useParams } from "react-router-dom";
import { api } from "../api/client";
import { unwrap } from "../api/queries";
import { Empty } from "../ui/Empty";
import { Failed } from "../ui/Failed";
import { Crumbs } from "../ui/Crumbs";

// A branch and a tag are different shapes of thing — one moves and is rebuilt,
// one never changes again — so they are labelled rather than blended into a
// single list of names.
export function Streams() {
  const { product = "" } = useParams();
  const streams = useQuery({
    queryKey: ["streams", product],
    queryFn: async () =>
      unwrap(
        await api.GET("/v1/products/{product}/streams", {
          params: { path: { product } },
        }),
      ),
  });

  if (streams.isPending) return <p className="text-sm text-[var(--muted)]">Loading…</p>;
  if (streams.isError) {
    return <Failed error={streams.error} what="The branches and tags could not be read." />;
  }

  const items = streams.data?.items ?? [];
  return (
    <div>
      <Crumbs product={product} />
      <h1 className="mb-4 text-lg font-semibold tracking-tight">Branches and tags</h1>
      {items.length === 0 ? (
        <Empty
          title="Nothing is declared here yet."
          detail="A branch or a tag is declared before a scan can be filed against it."
        />
      ) : (
        <ul className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
          {items.map((stream) => (
            <li key={stream.name}>
              <Link
                to={`/products/${encodeURIComponent(product)}/streams/${encodeURIComponent(stream.name)}`}
                className="block rounded-lg border border-[var(--line)] bg-[var(--surface)] px-4 py-3 hover:border-[var(--accent)]"
              >
                <span className="flex items-center gap-2">
                  <span className="font-medium">{stream.name}</span>
                  <Kind kind={stream.kind} />
                </span>
                {stream.parent && (
                  <span className="mt-0.5 block text-sm text-[var(--muted)]">cut from {stream.parent}</span>
                )}
              </Link>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

function Kind({ kind }: { kind?: string }) {
  return (
    <span className="rounded bg-[var(--raised)] px-1.5 py-0.5 text-xs text-[var(--muted)] ring-1 ring-inset ring-edge">
      {kind === "tag" ? "tag" : "branch"}
    </span>
  );
}
