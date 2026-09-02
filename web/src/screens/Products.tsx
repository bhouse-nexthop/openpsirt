import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { api } from "../api/client";
import { unwrap } from "../api/queries";
import { Empty } from "../ui/Empty";
import { Failed } from "../ui/Failed";
import { Declare, Field } from "../ui/Declare";
import type { Who } from "../app/session";

// You pick a product first, and everything below is bound to it (UIX-07).
// This is that first pick — not a dashboard, which is a different screen with
// different rules about spanning products.
export function Products({ who }: { who: Who }) {
  const queries = useQueryClient();
  const [name, setName] = useState("");
  const [displayName, setDisplayName] = useState("");
  const streams = useQuery({
    queryKey: ["products"],
    queryFn: async () => unwrap(await api.GET("/v1/products", {})),
  });
  const declare = useMutation({
    mutationFn: async () =>
      unwrap(
        await api.POST("/v1/products", {
          body: { name: name.trim(), display_name: displayName.trim() || undefined },
        }),
      ),
    onSuccess: () => {
      setName("");
      setDisplayName("");
      void queries.invalidateQueries({ queryKey: ["products"] });
    },
  });

  const form = (
    <Declare
      what="a product"
      hint="How scans name it, not how people read it. A scan filed against a name nobody declared is refused rather than quietly creating one, so this is where a product starts existing."
      onSubmit={() => declare.mutate()}
      error={declare.error}
      busy={declare.isPending || name.trim() === ""}
      can={who.admin}
    >
      <Field label="Name" value={name} onChange={setName} placeholder="sonic" hint="How scans name it" />
      <Field
        label="Display name"
        value={displayName}
        onChange={setDisplayName}
        placeholder="SONiC"
        hint="Optional. Defaults to the name"
        wide
      />
    </Declare>
  );

  if (streams.isPending) return <p className="text-sm text-[var(--muted)]">Loading…</p>;
  if (streams.isError) {
    return <Failed error={streams.error} what="The products could not be read." />;
  }

  const products = streams.data?.items ?? [];
  if (products.length === 0) {
    return (
      <>
        {form}
        <Empty
        title="You can reach no product yet."
        detail={
          who.admin
            ? "Declare one before a build can file a scan against it."
            : "Access is granted in advance, so an administrator grants a role before anything appears here."
        }
        />
      </>
    );
  }

  return (
    <div>
      <div className="screen-head">
        <h2>Products</h2>
        <p>Everything a scan can be filed against has to be declared first.</p>
      </div>
      {form}
      {/* What each one holds, rather than a list of names that has to be
          opened one at a time to find out whether anything is behind it. The
          open count is issues at components, the same way the findings list
          counts, so the two numbers agree. */}
      <div className="tablewrap">
        <table>
          <thead>
            <tr>
              <th>Product</th>
              <th className="num">Branches</th>
              <th className="num">Tags</th>
              <th className="num">Variants</th>
              <th className="num">Open</th>
              <th>Last scan</th>
            </tr>
          </thead>
          <tbody>
            {products.map((product) => (
              <tr key={product.name} className="row">
                <td>
                  <Link to={`/products/${encodeURIComponent(product.name)}/streams`} className="id">
                    {product.display_name || product.name}
                  </Link>
                  {product.display_name && product.display_name !== product.name && (
                    <>
                      <br />
                      <span className="id" style={{ color: "var(--faint)" }}>{product.name}</span>
                    </>
                  )}
                </td>
                <td className="num">{product.branches ?? 0}</td>
                <td className="num">{product.tags ?? 0}</td>
                <td className="num">{product.variants ?? 0}</td>
                <td className="num">{(product.open ?? 0).toLocaleString()}</td>
                <td className="hint">
                  {product.last_scan_at ? product.last_scan_at.slice(0, 10) : "never"}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
