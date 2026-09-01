import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useParams } from "react-router-dom";
import { api } from "../api/client";
import { unwrap } from "../api/queries";
import { Empty } from "../ui/Empty";
import { Failed } from "../ui/Failed";
import { Crumbs } from "../ui/Crumbs";
import { Declare, Field } from "../ui/Declare";
import { useWho } from "../app/session";

// What this release was actually built as — a subset of what the product
// declares. A release predating a variant has never been filed against it and
// does not list it, which is what keeps something introduced later from
// appearing to have shipped years ago.
export function Variants() {
  const { product = "", stream = "" } = useParams();
  const queries = useQueryClient();
  const who = useWho();
  const [name, setName] = useState("");
  const [facing, setFacing] = useState(true);
  const variants = useQuery({
    queryKey: ["variants", product, stream],
    queryFn: async () =>
      unwrap(
        await api.GET("/v1/products/{product}/streams/{stream}/variants", {
          params: { path: { product, stream } },
        }),
      ),
  });

  const declare = useMutation({
    mutationFn: async () =>
      unwrap(
        await api.POST("/v1/products/{product}/variants", {
          params: { path: { product } },
          body: { name: name.trim(), customer_facing: facing },
        }),
      ),
    onSuccess: () => {
      setName("");
      void queries.invalidateQueries({ queryKey: ["variants", product, stream] });
      void queries.invalidateQueries({ queryKey: ["variants", product] });
    },
  });

  if (variants.isPending) return <p className="text-sm text-[var(--muted)]">Loading…</p>;
  if (variants.isError) {
    return <Failed error={variants.error} what="The variants could not be read." />;
  }

  const items = variants.data?.items ?? [];
  return (
    <div>
      <Crumbs product={product} stream={stream} />
      <h1 className="mb-4 text-lg font-semibold tracking-tight">Variants</h1>
      {/* Declaring is the product's, listing is the release's. What a product
          is built as belongs to the product; which of them a given release was
          built as is the release's, and is answered by a scan arriving
          (MDL-01). So this form adds to the product and the list below does
          not change until something is filed. */}
      <Declare
        what="a variant of this product"
        hint="Declared against the product, not this release. A release that predates a variant has never been filed against it and will not list it, which is what keeps something introduced later from appearing to have shipped years ago."
        onSubmit={() => declare.mutate()}
        error={declare.error}
        busy={declare.isPending || name.trim() === ""}
        can={who.data?.admin ?? false}
      >
        <Field label="Name" value={name} onChange={setName} placeholder="broadcom" hint="How scans name it" />
        <div className="field" style={{ margin: 0, minWidth: 190 }}>
          <label htmlFor="declare-facing">Reaches customers</label>
          <select
            id="declare-facing"
            value={facing ? "yes" : "no"}
            onChange={(event) => setFacing(event.target.value === "yes")}
          >
            <option value="yes">yes — ranks above the rest</option>
            <option value="no">no — internal only</option>
          </select>
        </div>
      </Declare>
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
                className="block rounded-lg border border-[var(--line)] bg-[var(--surface)] px-4 py-3 hover:border-[var(--accent)]"
              >
                <span className="font-medium">{variant.name}</span>
                {variant.customer_facing === false && (
                  <span className="mt-0.5 block text-sm text-[var(--muted)]">not customer-facing</span>
                )}
              </Link>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
