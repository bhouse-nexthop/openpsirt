import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { api } from "../api/client";
import { unwrap } from "../api/queries";
import { Empty } from "../ui/Empty";
import { Failed } from "../ui/Failed";
import { Severity } from "../ui/Severity";

// Work nobody owns, across every product somebody can see. Nobody-is-assigned
// is a state to be asked about rather than an absence: work that falls between
// people is invisible unless it can be listed, and it is exactly what hides
// when every screen shows one product.
export function Unassigned() {
  const queries = useQueryClient();
  const nobodys = useQuery({
    queryKey: ["unassigned"],
    queryFn: async () =>
      unwrap(await api.GET("/v1/unassigned", { params: { query: { limit: 50 } } })),
  });

  const assign = useMutation({
    mutationFn: async (to: {
      product: string;
      stream: string;
      variant: string;
      vulnerability: string;
      component: string;
      person: string;
    }) =>
      unwrap(
        await api.PUT(
          "/v1/products/{product}/streams/{stream}/variants/{variant}/findings/{vulnerability}/components/{component}/assignment",
          {
            params: {
              path: {
                product: to.product,
                stream: to.stream,
                variant: to.variant,
                vulnerability: to.vulnerability,
                component: to.component,
              },
            },
            body: { person: to.person },
          },
        ),
      ),
    onSuccess: () => {
      void queries.invalidateQueries({ queryKey: ["unassigned"] });
      void queries.invalidateQueries({ queryKey: ["home"] });
    },
  });

  if (nobodys.isPending) return <p className="text-sm text-muted">Loading…</p>;
  if (nobodys.isError) {
    return <Failed error={nobodys.error} what="What nobody owns could not be read." />;
  }

  const items = nobodys.data?.items ?? [];

  return (
    <div className="max-w-4xl">
      <header className="mb-4">
        <h1 className="text-lg font-semibold tracking-tight">Nobody is dealing with these</h1>
        <p className="text-sm text-muted">
          Across every product you can see. {nobodys.data?.total ?? 0} in total.
        </p>
      </header>

      {assign.error != null && <Failed error={assign.error} what="That could not be assigned." />}

      {items.length === 0 ? (
        <Empty title="Everything open has somebody on it." />
      ) : (
        <ul className="flex flex-col gap-2">
          {items.map((row) => (
            <Row
              key={`${row.product}-${row.vulnerability}-${row.component}`}
              row={row}
              onAssign={(person) =>
                assign.mutate({
                  product: row.product ?? "",
                  stream: row.stream ?? "",
                  variant: row.variant ?? "",
                  vulnerability: row.vulnerability ?? "",
                  component: row.component ?? "",
                  person,
                })
              }
              pending={assign.isPending}
            />
          ))}
        </ul>
      )}
    </div>
  );
}

type Owned = {
  product?: string;
  stream?: string;
  variant?: string;
  vulnerability?: string;
  component?: string;
  version?: string;
  severity?: string;
  exploited?: boolean;
  places?: number;
};

function Row({
  row,
  onAssign,
  pending,
}: {
  row: Owned;
  onAssign: (person: string) => void;
  pending: boolean;
}) {
  const [person, setPerson] = useState("");
  const [asking, setAsking] = useState(false);

  return (
    <li className="rounded-lg border border-edge bg-raised p-3 text-sm">
      <div className="flex flex-wrap items-center gap-2">
        <Link
          to={
            `/products/${encodeURIComponent(row.product ?? "")}` +
            `/streams/${encodeURIComponent(row.stream ?? "")}` +
            `/variants/${encodeURIComponent(row.variant ?? "")}` +
            `/findings/${encodeURIComponent(row.vulnerability ?? "")}` +
            `/components/${encodeURIComponent(row.component ?? "")}`
          }
          className="font-medium hover:text-accent"
        >
          {row.vulnerability}
        </Link>
        <Severity word={row.severity} exploited={row.exploited} />
        <span className="text-muted">
          {row.component} {row.version}
        </span>
        <span className="ml-auto text-muted">
          {row.product} / {row.stream} / {row.variant}
        </span>
      </div>

      <div className="mt-2 flex flex-wrap items-center gap-2">
        <span className="text-muted">
          {row.places} {row.places === 1 ? "place" : "places"}
        </span>
        {asking ? (
          <span className="ml-auto flex items-center gap-2">
            {/* Assignment covers the whole group at once — one issue in one
                component, however many places it sits at. Assigning one place
                and not another is not something anybody means to do. */}
            <input
              value={person}
              onChange={(event) => setPerson(event.target.value)}
              placeholder="their sign-in identity"
              aria-label="Who takes this on"
              className="rounded border border-edge bg-raised px-2 py-1"
            />
            <button
              type="button"
              disabled={!person.trim() || pending}
              onClick={() => {
                onAssign(person.trim());
                setAsking(false);
                setPerson("");
              }}
              className="rounded bg-accent px-2 py-1 font-medium text-accent-ink disabled:opacity-50"
            >
              Assign
            </button>
            <button type="button" onClick={() => setAsking(false)} className="text-muted">
              Cancel
            </button>
          </span>
        ) : (
          <button
            type="button"
            onClick={() => setAsking(true)}
            className="ml-auto text-accent hover:underline"
          >
            Give it to somebody
          </button>
        )}
      </div>
    </li>
  );
}
