import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { api } from "../api/client";
import { unwrap } from "../api/queries";
import { AddButton, Declare, Field } from "../ui/Declare";
import { Empty } from "../ui/Empty";
import { Failed } from "../ui/Failed";
import type { Who } from "../app/session";

// You pick a product first, and everything below is bound to it (UIX-07).
// What each one holds is on the row, so the list answers the question it
// exists to answer without every row being opened.
export function Products({ who }: { who: Who }) {
  const queries = useQueryClient();
  const [adding, setAdding] = useState(false);
  const [name, setName] = useState("");
  const [displayName, setDisplayName] = useState("");
  const products = useQuery({
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
      setAdding(false);
      void queries.invalidateQueries({ queryKey: ["products"] });
    },
  });

  if (products.isPending) return <p className="hint">Loading…</p>;
  if (products.isError) {
    return <Failed error={products.error} what="The products could not be read." />;
  }

  const items = products.data?.items ?? [];

  return (
    <>
      <div className="screen-head">
        <h2>Products</h2>
        <p>
          Everything a scan can be filed against has to be declared first — a pipeline with a typo
          would otherwise invent a product that looks entirely genuine.
        </p>
        {who.admin && <AddButton label="Add product" onClick={() => setAdding(true)} />}
      </div>

      {items.length === 0 ? (
        <Empty
          title="You can reach no product yet."
          detail={
            who.admin
              ? "Declare one before a build can file a scan against it."
              : "Access is granted in advance, so an administrator grants a role before anything appears here."
          }
        />
      ) : (
        <div className="tablewrap">
          <table>
            <thead>
              <tr>
                <th>Product</th>
                <th className="num">Branches</th>
                <th className="num">Tags</th>
                <th className="num">Variants</th>
                <th className="num">Open</th>
                <th>Last inventory</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {items.map((product) => {
                const stale =
                  !!product.last_scan_at &&
                  Date.now() - Date.parse(product.last_scan_at) > 7 * 86_400_000;
                return (
                  <tr key={product.name} className="row">
                    <td>
                      <Link to={`/products/${encodeURIComponent(product.name)}/streams`} className="id">
                        {product.display_name || product.name}
                      </Link>
                      {product.display_name && product.display_name !== product.name && (
                        <>
                          <br />
                          <span className="id" style={{ color: "var(--faint)" }}>
                            {product.name}
                          </span>
                        </>
                      )}
                    </td>
                    <td className="num">{product.branches ?? 0}</td>
                    <td className="num">{product.tags ?? 0}</td>
                    <td className="num">{product.variants ?? 0}</td>
                    <td className="num">{(product.open ?? 0).toLocaleString()}</td>
                    <td className={stale ? "" : "hint"} style={stale ? { color: "var(--sev-high)", fontWeight: 600 } : undefined}>
                      {product.last_scan_at ? product.last_scan_at.slice(0, 10) : "never"}
                    </td>
                    <td>
                      <Link to={`/products/${encodeURIComponent(product.name)}/streams`} className="linkish">
                        Manage
                      </Link>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}

      <Declare
        title="Add product"
        open={adding}
        onClose={() => setAdding(false)}
        onSubmit={() => declare.mutate()}
        error={declare.error}
        busy={declare.isPending || name.trim() === ""}
        ok="Add product"
        hint="Declared rather than created on first use. A pipeline with a typo in a product name would otherwise invent a product that looks entirely genuine, with its own findings and its own place in every report, while the real one appears to have stopped being scanned."
      >
        <Field
          label="Name"
          value={name}
          onChange={setName}
          placeholder="sonic"
          hint="How scans name it. Matched without regard to capitals; the spelling typed here is what is shown back."
        />
        <Field label="Display name" value={displayName} onChange={setDisplayName} placeholder="SONiC" hint="Optional. Defaults to the name" />
      </Declare>
    </>
  );
}
