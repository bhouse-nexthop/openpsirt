import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { useParams, useSearchParams } from "react-router-dom";
import { api } from "../api/client";
import { unwrap } from "../api/queries";
import { Empty } from "../ui/Empty";
import { Failed } from "../ui/Failed";
import { Crumbs } from "../ui/Crumbs";

// The graph is walked one step at a time. A full render is not offered and
// would not be useful: a real image holds thousands of components and tens of
// thousands of edges, which neither draws nor reads.
//
// Which component is in focus lives in the URL, so a link carries it.
export function Tree() {
  const { product = "", stream = "", variant = "" } = useParams();
  const [params, setParams] = useSearchParams();
  const focus = params.get("at") ?? "";

  return (
    <div className="max-w-3xl">
      <Crumbs product={product} stream={stream} variant={variant} />
      <h1 className="mb-1 text-lg font-semibold tracking-tight">What this build contains</h1>
      <p className="mb-4 text-sm text-muted">
        One step at a time. A component reached several ways appears once with several parents —
        it is a graph rather than a tree.
      </p>
      {focus ? (
        <Around
          at={{ product, stream, variant }}
          component={focus}
          onFocus={(next) => setParams(next ? { at: next } : {})}
        />
      ) : (
        <Top at={{ product, stream, variant }} onFocus={(next) => setParams({ at: next })} />
      )}
    </div>
  );
}

type At = { product: string; stream: string; variant: string };

// Where walking starts: what the build itself depends on, most findings first.
function Top({ at, onFocus }: { at: At; onFocus: (component: string) => void }) {
  const top = useQuery({
    queryKey: ["tree-top", at],
    queryFn: async () =>
      unwrap(
        await api.GET("/v1/products/{product}/streams/{stream}/variants/{variant}/components", {
          params: { path: at },
        }),
      ),
  });

  if (top.isPending) return <p className="text-sm text-muted">Loading…</p>;
  if (top.isError) return <Failed error={top.error} what="The top level could not be read." />;

  const items = top.data?.items ?? [];
  if (items.length === 0) {
    return <Empty title="This build pulls nothing in." detail="Nothing has been scanned here, or the inventory described no dependencies." />;
  }
  return <Nodes items={items} onFocus={onFocus} />;
}

// What pulls a component in, and what it pulls in. Upward is the direction
// people actually use: somebody arrives from a finding and asks why the
// component is here — and up is short, where down is where the size lives.
function Around({
  at,
  component,
  onFocus,
}: {
  at: At;
  component: string;
  onFocus: (component: string) => void;
}) {
  const around = useQuery({
    queryKey: ["tree-around", at, component],
    queryFn: async () =>
      unwrap(
        await api.GET(
          "/v1/products/{product}/streams/{stream}/variants/{variant}/components/{component}/around",
          { params: { path: { ...at, component } } },
        ),
      ),
  });

  if (around.isPending) return <p className="text-sm text-muted">Loading…</p>;
  if (around.isError) {
    return <Failed error={around.error} what="What sits around this could not be read." />;
  }

  const above = around.data?.above ?? [];
  const below = around.data?.below ?? [];

  return (
    <div>
      <div className="mb-4 flex items-center gap-2">
        <button
          type="button"
          onClick={() => onFocus("")}
          className="text-sm text-muted hover:text-ink"
        >
          ← Back to the top level
        </button>
      </div>

      <div className="mb-3 flex flex-wrap items-center gap-3">
        <h2 className="text-sm font-semibold">{component}</h2>
        <Link
          to={
            `/products/${encodeURIComponent(at.product)}` +
            `/streams/${encodeURIComponent(at.stream)}` +
            `/variants/${encodeURIComponent(at.variant)}` +
            `/components/${encodeURIComponent(component)}/decide`
          }
          className="text-sm text-accent hover:underline"
        >
          Decide several at once
        </Link>
      </div>

      <section className="mb-5">
        <h3 className="mb-2 text-sm text-muted">What pulls it in</h3>
        {above.length === 0 ? (
          <p className="text-sm text-muted">Nothing — the build depends on it directly.</p>
        ) : (
          <Nodes items={above} onFocus={onFocus} />
        )}
      </section>

      <section>
        <h3 className="mb-2 text-sm text-muted">What it pulls in</h3>
        {below.length === 0 ? (
          <p className="text-sm text-muted">Nothing.</p>
        ) : (
          <Nodes items={below} onFocus={onFocus} />
        )}
      </section>
    </div>
  );
}

type Node = { component?: string; name?: string; version?: string; findings?: number; children?: number };

// Every entry says how many findings are open against it and how many
// components it pulls in, so descending follows something rather than being
// exploration.
function Nodes({ items, onFocus }: { items: Node[]; onFocus: (component: string) => void }) {
  return (
    <ul className="divide-y divide-edge overflow-hidden rounded-lg border border-edge">
      {items.map((node) => {
        const name = node.component ?? node.name ?? "";
        return (
          <li key={name} className="flex flex-wrap items-center gap-2 bg-raised px-3 py-2 text-sm">
            <button
              type="button"
              onClick={() => onFocus(name)}
              className="font-medium hover:text-accent"
            >
              {name}
            </button>
            {node.version && <span className="text-muted">{node.version}</span>}
            <span className="ml-auto flex gap-3 text-muted">
              {typeof node.findings === "number" && node.findings > 0 && (
                <span>{node.findings} open</span>
              )}
              {typeof node.children === "number" && node.children > 0 && (
                <span>pulls in {node.children}</span>
              )}
            </span>
          </li>
        );
      })}
    </ul>
  );
}
