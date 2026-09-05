import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useSearchParams } from "react-router-dom";
import { api, type Body } from "../api/client";
import { scopeQuery, useScope } from "../app/scope";
import { useWho } from "../app/session";
import { unwrap } from "../api/queries";
import { Empty } from "../ui/Empty";
import { Failed } from "../ui/Failed";
import { Severity, Exploited } from "../ui/Severity";
import { Paged } from "../ui/Paged";

// A page of what nobody holds. The head says the total; the page says what
// of it is in front of somebody.
const PAGE = 50;

// Work nobody owns, across every product somebody can see. Unassigned is a
// state to be asked about rather than an absence: work that falls between
// people is invisible unless it can be listed, and it is exactly what hides
// when every screen shows one product.
export function Unassigned() {
  const scope = scopeQuery(useScope());
  const [params, setParams] = useSearchParams();
  const offset = Number(params.get("offset") ?? 0);
  const queries = useQueryClient();
  const [picked, setPicked] = useState<Set<string>>(new Set());
  const [person, setPerson] = useState("");
  const rows = useQuery({
    queryKey: ["unassigned", scope, offset],
    queryFn: async () =>
      unwrap(
        await api.GET("/v1/unassigned", { params: { query: { limit: PAGE, offset, ...scope } } }),
      ),
  });
  const me = useWho();
  // Who may be offered here, rather than everybody. Listing people asks for
  // administration, so a triager saw "Assign to…" and nothing else.
  //
  // The narrower question is per product, and this list spans every product
  // somebody can see — so the picker fills once a product is chosen, and
  // taking work yourself works either way, which is the case that was most
  // obviously missing.
  const people = useQuery({
    enabled: scope.product != null && scope.product !== "",
    queryKey: ["mentionable", scope.product],
    queryFn: async () =>
      unwrap(
        await api.GET("/v1/products/{product}/mentionable", {
          params: { path: { product: scope.product as string }, query: { limit: 100 } },
        }),
      ),
  });

  const assign = useMutation({
    mutationFn: async (to: { row: Owned; person: string }) =>
      unwrap(
        await api.PUT(
          "/v1/products/{product}/streams/{stream}/variants/{variant}/findings/{vulnerability}/components/{component}/assignment",
          {
            params: {
              path: {
                product: to.row.product ?? "",
                stream: to.row.stream ?? "",
                variant: to.row.variant ?? "",
                vulnerability: to.row.vulnerability ?? "",
                component: to.row.component ?? "",
              },
            },
            body: { person: to.person },
          },
        ),
      ),
    onSuccess: () => {
      void queries.invalidateQueries({ queryKey: ["unassigned"] });
      void queries.invalidateQueries({ queryKey: ["holdings"] });
      void queries.invalidateQueries({ queryKey: ["home"] });
    },
  });

  if (rows.isPending) return <p className="hint">Loading…</p>;
  if (rows.isError) {
    return <Failed error={rows.error} what="What is unassigned could not be read." />;
  }

  const items = rows.data?.items ?? [];
  // What identifies a row for picking, which is what it is *about* rather than
  // where it happens to be listed. The build is one of several where the code
  // is built more than one way, so including it would give the same piece of
  // work two keys if the representative ever changed.
  const keyOf = (row: Owned) =>
    `${row.product} ${row.vulnerability} ${row.component} ${row.version}`;

  async function assignTo(to: string) {
    // The same action repeated, not a different one — each finding records
    // who it went to and when.
    for (const row of items.filter((r) => picked.has(keyOf(r)))) {
      await assign.mutateAsync({ row, person: to });
    }
    setPicked(new Set());
  }

  return (
    <>
      <div className="screen-head">
        <h2>Unassigned</h2>
        <p>
          Undecided findings with nobody assigned,{" "}
          {scope.product
            ? [scope.product, scope.stream, scope.variant].filter(Boolean).join(" · ")
            : "across every product you can see"}{" "}
          · {(rows.data?.total ?? 0).toLocaleString()} in total
        </p>
      </div>

      {assign.error != null && <Failed error={assign.error} what="That could not be assigned." />}

      {items.length === 0 ? (
        <Empty title="Everything open has somebody on it." />
      ) : (
        <>
          <div className="batchbar">
            <span>
              <b>{picked.size === 0 ? "Nothing selected" : `${picked.size} selected`}</b>
            </span>
            <span className="spacer" />
            {/* Taking unowned work is a triager's own and needs nobody's
                picker, so it is a button of its own rather than a step
                through a select that may be empty. */}
            <button
              type="button"
              className="btn"
              disabled={picked.size === 0 || me.data?.identity == null || assign.isPending}
              onClick={() => void assignTo(me.data!.identity)}
            >
              {picked.size === 0 ? "Take" : `Take ${picked.size}`}
            </button>
            <select
              aria-label="Assign to"
              style={{ width: "auto" }}
              value={person}
              disabled={!scope.product}
              title={scope.product ? undefined : "Choose a product to hand work to somebody else"}
              onChange={(event) => setPerson(event.target.value)}
            >
              <option value="">Assign to…</option>
              {(people.data?.items ?? []).map((each) => (
                <option key={each.identity} value={each.identity}>
                  {each.name || each.identity}
                </option>
              ))}
            </select>
            <button
              type="button"
              className="btn"
              disabled={picked.size === 0 || !person || assign.isPending}
              onClick={() => void assignTo(person.trim())}
            >
              {picked.size === 0 ? "Assign" : `Assign ${picked.size}`}
            </button>
          </div>

          <div className="tablewrap" style={{ marginTop: 10 }}>
            <table>
              <thead>
                <tr>
                  <th style={{ width: 34 }}>
                    <input
                      type="checkbox"
                      aria-label="Select every row shown"
                      checked={picked.size > 0 && picked.size === items.length}
                      onChange={(event) =>
                        setPicked(event.target.checked ? new Set(items.map(keyOf)) : new Set())
                      }
                    />
                  </th>
                  <th>Severity</th>
                  <th>Issue</th>
                  <th>Component</th>
                  <th>Build</th>
                  <th className="num">Locations</th>
                </tr>
              </thead>
              <tbody>
                {items.map((row) => (
                  <tr key={keyOf(row)} className="row">
                    <td>
                      <input
                        type="checkbox"
                        aria-label={`Select ${row.vulnerability}`}
                        checked={picked.has(keyOf(row))}
                        onChange={(event) => {
                          const next = new Set(picked);
                          if (event.target.checked) next.add(keyOf(row));
                          else next.delete(keyOf(row));
                          setPicked(next);
                        }}
                      />
                    </td>
                    <td>
                      <Severity word={row.severity} />
                    </td>
                    <td>
                      <Link
                        to={
                          `/products/${encodeURIComponent(row.product ?? "")}` +
                          `/streams/${encodeURIComponent(row.stream ?? "")}` +
                          `/variants/${encodeURIComponent(row.variant ?? "")}` +
                          `/findings/${encodeURIComponent(row.vulnerability ?? "")}` +
                          `/components/${encodeURIComponent(row.component ?? "")}` +
                          (row.version ? `?version=${encodeURIComponent(row.version)}` : "")
                        }
                        className="id"
                      >
                        {row.vulnerability}
                      </Link>{" "}
                      <Exploited when={row.exploited} />
                    </td>
                    <td>
                      <span className="id">{row.component}</span>{" "}
                      <span className="id" style={{ color: "var(--faint)" }}>
                        {row.version}
                      </span>
                    </td>
                    <td className="hint">
                      {row.product}
                      {(row.builds ?? 1) > 1 ? (
                        // Not the one build named on the row. The same code
                        // built several ways is one piece of work — assigning
                        // it takes on every build of it — and naming one would
                        // read as being about that one.
                        <> · {row.builds} builds</>
                      ) : (
                        <>
                          {" "}
                          · {row.stream} · {row.variant}
                        </>
                      )}
                    </td>
                    <td className="num">{row.places}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <Paged
            shown={items.length}
            total={rows.data?.total}
            offset={offset}
            limit={PAGE}
            onGo={(next) => {
              const now = new URLSearchParams(params);
              if (next === 0) now.delete("offset");
              else now.set("offset", String(next));
              setParams(now);
            }}
          />
        </>
      )}
    </>
  );
}

type Owned = Body<"UnassignedBody">;
