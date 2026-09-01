import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { api } from "../api/client";
import { unwrap } from "../api/queries";
import { Empty } from "../ui/Empty";
import { Failed } from "../ui/Failed";
import type { Who } from "../app/session";

// You pick a product first, and everything below is bound to it (UIX-07).
// This is that first pick — not a dashboard, which is a different screen with
// different rules about spanning products.
export function Products({ who }: { who: Who }) {
  const streams = useQuery({
    queryKey: ["products"],
    queryFn: async () => unwrap(await api.GET("/v1/products", {})),
  });

  if (streams.isPending) return <p className="text-sm text-[var(--muted)]">Loading…</p>;
  if (streams.isError) {
    return <Failed error={streams.error} what="The products could not be read." />;
  }

  const products = streams.data?.items ?? [];
  if (products.length === 0) {
    return (
      <Empty
        title="You can reach no product yet."
        detail={
          who.admin
            ? "Declare one before a build can file a scan against it."
            : "Access is granted in advance, so an administrator grants a role before anything appears here."
        }
      />
    );
  }

  return (
    <div>
      <h1 className="mb-4 text-lg font-semibold tracking-tight">Products</h1>
      <ul className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
        {products.map((product) => (
          <li key={product.name}>
            <Link
              to={`/products/${encodeURIComponent(product.name)}/streams`}
              className="block rounded-lg border border-[var(--line)] bg-[var(--surface)] px-4 py-3 hover:border-[var(--accent)]"
            >
              <span className="font-medium">{product.display_name || product.name}</span>
              <span className="mt-0.5 block text-sm text-[var(--muted)]">{product.name}</span>
            </Link>
          </li>
        ))}
      </ul>
    </div>
  );
}
