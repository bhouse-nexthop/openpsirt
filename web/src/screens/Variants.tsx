import { useQuery } from "@tanstack/react-query";
import { Link, useParams } from "react-router-dom";
import { api } from "../api/client";
import { unwrap } from "../api/queries";
import { Empty } from "../ui/Empty";
import { Failed } from "../ui/Failed";
import { Crumbs } from "../ui/Crumbs";

// What this release was actually built as — a subset of what the product
// declares. A release predating a variant has never been filed against it and
// does not list it, which is what keeps something introduced later from
// appearing to have shipped years ago.
export function Variants() {
  const { product = "", stream = "" } = useParams();
  const variants = useQuery({
    queryKey: ["variants", product, stream],
    queryFn: async () =>
      unwrap(
        await api.GET("/v1/products/{product}/streams/{stream}/variants", {
          params: { path: { product, stream } },
        }),
      ),
  });

  if (variants.isPending) return <p className="text-sm text-muted">Loading…</p>;
  if (variants.isError) {
    return <Failed error={variants.error} what="The variants could not be read." />;
  }

  const items = variants.data?.items ?? [];
  return (
    <div>
      <Crumbs product={product} stream={stream} />
      <h1 className="mb-4 text-lg font-semibold tracking-tight">Variants</h1>
      {items.length === 0 ? (
        <Empty
          title="Nothing has been scanned here."
          detail="A variant appears once a build has filed a scan against it."
        />
      ) : (
        <ul className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
          {items.map((variant) => (
            <li key={variant.name}>
              <Link
                to={
                  `/products/${encodeURIComponent(product)}` +
                  `/streams/${encodeURIComponent(stream)}` +
                  `/variants/${encodeURIComponent(variant.name)}/findings`
                }
                className="block rounded-lg border border-edge bg-raised px-4 py-3 hover:border-accent"
              >
                <span className="font-medium">{variant.name}</span>
                {variant.customer_facing === false && (
                  <span className="mt-0.5 block text-sm text-muted">not customer-facing</span>
                )}
              </Link>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
