import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useSearchParams } from "react-router-dom";
import { api } from "../api/client";
import type { Body } from "../api/client";
import { unwrap } from "../api/queries";
import { Empty } from "../ui/Empty";
import { Failed } from "../ui/Failed";
import { Exploited, Severity } from "../ui/Severity";
import { scopeQuery, useScope } from "../app/scope";

// Two questions about work in flight, across every product somebody can see.
//
// Work assigned to somebody who has gone is invisible twice over: not in the
// shared queue because it is assigned, and not in anybody's list because they
// are not here. Nothing tells us somebody left, so releasing their work is an
// action rather than something the tool discovers (ACC-43 to ACC-45).
export function Work() {
  const [params, setParams] = useSearchParams();
  const tab = params.get("tab") ?? "due";
  const days = Number(params.get("days") ?? 30);

  const scope = scopeQuery(useScope());
  const due = useQuery({
    queryKey: ["running-out", days, scope],
    queryFn: async () =>
      unwrap(
        await api.GET("/v1/running-out", {
          params: { query: { days, limit: 200, ...scope } },
        }),
      ),
  });
  const holdings = useQuery({
    queryKey: ["holdings"],
    queryFn: async () => unwrap(await api.GET("/v1/assignments", {})),
  });

  function go(next: string) {
    const now = new URLSearchParams(params);
    if (next === "due") now.delete("tab");
    else now.set("tab", next);
    setParams(now);
  }

  const lateRows = due.data?.items ?? [];
  const peopleRows = holdings.data?.items ?? [];

  return (
    <>
      <div className="screen-head">
        <h2>Who is working on what</h2>
        <p>
          {scope.product
            ? `${scope.product}${scope.stream ? ` · ${scope.stream}` : ""}${scope.variant ? ` · ${scope.variant}` : ""}`
            : "Across every product you can see"}
        </p>
      </div>

      <div className="tabs2">
        <button
          type="button"
          className="tab2 urgent"
          aria-selected={tab === "due"}
          onClick={() => go("due")}
        >
          Due soon, still undecided <span className="n">{lateRows.length}</span>
        </button>
        <button
          type="button"
          className="tab2"
          aria-selected={tab === "people"}
          onClick={() => go("people")}
        >
          By person <span className="n">{peopleRows.length}</span>
        </button>
        <Link to="/unassigned" className="tab2" aria-selected={false}>
          Nobody assigned
        </Link>
      </div>

      {tab === "due" ? (
        <Due days={days} rows={lateRows} query={due} onDays={(next) => {
          const now = new URLSearchParams(params);
          now.set("days", String(next));
          setParams(now);
        }} />
      ) : (
        <ByPerson rows={peopleRows} query={holdings} />
      )}
    </>
  );
}

type Query = { isPending: boolean; isError: boolean; error: unknown };

// A deadline that has been answered — deferred to a date, or dismissed — is not
// on this list. What is left is time passing with nothing said.
function Due({
  days,
  rows,
  query,
  onDays,
}: {
  days: number;
  rows: Body<"LateBody">[];
  query: Query;
  onDays: (days: number) => void;
}) {
  if (query.isPending) return <p className="hint">Loading…</p>;
  if (query.isError) {
    return <Failed error={query.error} what="What is running out of time could not be read." />;
  }

  return (
    <>
      <p className="reading" style={{ marginBottom: 12 }}>
        Findings whose deadline is close and which nobody has decided about yet. A deadline that
        has been answered — deferred to a date, or dismissed — is not on this list: a dismissal
        takes a finding off the clock and a deferral replaces the deadline with its own date.
        What is left is time passing with nothing said.
      </p>

      <div className="filters" style={{ marginBottom: 10 }}>
        <span style={{ color: "var(--faint)" }}>Within</span>
        <span className="seg">
          {[7, 30, 90].map((window) => (
            <button
              key={window}
              type="button"
              aria-pressed={days === window}
              onClick={() => onDays(window)}
            >
              {window} days
            </button>
          ))}
        </span>
      </div>

      {rows.length === 0 ? (
        <Empty
          title="Nothing is running out inside that window."
          detail="Either everything close to its deadline has been answered, or nothing is close."
        />
      ) : (
        <div className="tablewrap">
          <table>
            <thead>
              <tr>
                <th>Due</th>
                <th>Severity</th>
                <th>Issue</th>
                <th>Component</th>
                <th>Build</th>
                <th>Assigned to</th>
                <th className="num">Places</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((row) => {
                const late = (row.days_left ?? 0) < 0;
                return (
                  <tr
                    key={`${row.product} ${row.stream} ${row.variant} ${row.vulnerability} ${row.component}`}
                    className="row"
                  >
                    <td>
                      <span style={late ? { color: "var(--sev-exploited)", fontWeight: 600 } : undefined}>
                        {row.due}
                      </span>
                      <br />
                      <span className="hint">
                        {late
                          ? `${Math.abs(row.days_left ?? 0)} days over`
                          : `${row.days_left ?? 0} days left`}
                      </span>
                    </td>
                    <td>
                      <Severity word={row.severity} />
                    </td>
                    <td>
                      <Link
                        className="id"
                        to={
                          `/products/${encodeURIComponent(row.product ?? "")}` +
                          `/streams/${encodeURIComponent(row.stream ?? "")}` +
                          `/variants/${encodeURIComponent(row.variant ?? "")}` +
                          `/findings/${encodeURIComponent(row.vulnerability ?? "")}` +
                          `/components/${encodeURIComponent(row.component ?? "")}` +
                          // Without the version a name that ships twice in one
                          // build resolves to nothing, so the link dead-ends.
                          (row.version ? `?version=${encodeURIComponent(row.version)}` : "")
                        }
                      >
                        {row.vulnerability}
                      </Link>{" "}
                      <Exploited when={row.exploited} />
                    </td>
                    <td>
                      <span className="id">{row.component}</span>
                    </td>
                    <td className="hint">
                      {row.product} {row.stream} {row.variant}
                    </td>
                    <td>
                      {row.assigned_to ? (
                        row.assigned_to
                      ) : (
                        <span style={{ color: "var(--faint)" }}>nobody</span>
                      )}
                    </td>
                    <td className="num">{row.places}</td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </>
  );
}

// What each person is holding, and the one action nothing else offers: giving
// their work back when they have gone.
function ByPerson({
  rows,
  query,
}: {
  rows: { person?: string; open?: number; overdue?: number }[];
  query: Query;
}) {
  const queries = useQueryClient();
  const release = useMutation({
    mutationFn: async (identity: string) =>
      unwrap(
        await api.POST("/v1/people/{identity}/assignments/release", {
          params: { path: { identity } },
          body: {},
        }),
      ),
    onSuccess: () => {
      void queries.invalidateQueries({ queryKey: ["holdings"] });
      void queries.invalidateQueries({ queryKey: ["unassigned"] });
    },
  });

  if (query.isPending) return <p className="hint">Loading…</p>;
  if (query.isError) {
    return <Failed error={query.error} what="What people are holding could not be read." />;
  }
  if (rows.length === 0) {
    return <Empty title="Nobody is holding anything." detail="Every open finding is in the shared queue." />;
  }

  return (
    <>
      {release.error != null && (
        <Failed error={release.error} what="Their work could not be released." />
      )}
      <p className="reading" style={{ marginBottom: 12 }}>
        Work assigned to somebody who has gone is invisible twice over: not in the shared queue
        because it is assigned, and not in anybody's list because they are not here. Nothing
        tells us somebody left, so giving their work back is an action rather than something the
        tool discovers.
      </p>
      <div className="tablewrap">
        <table>
          <thead>
            <tr>
              <th>Person</th>
              <th className="num">Open</th>
              <th className="num">Past its deadline</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {rows.map((row) => (
              <tr key={row.person} className="row">
                <td>
                  <span className="id">{row.person}</span>
                </td>
                <td className="num">{(row.open ?? 0).toLocaleString()}</td>
                <td className="num">
                  {(row.overdue ?? 0) > 0 ? (
                    <span style={{ color: "var(--sev-exploited)", fontWeight: 600 }}>
                      {row.overdue}
                    </span>
                  ) : (
                    <span style={{ color: "var(--faint)" }}>0</span>
                  )}
                </td>
                <td>
                  <button
                    type="button"
                    className="linkish"
                    title="Put everything they hold back into the shared queue"
                    onClick={() => release.mutate(row.person ?? "")}
                  >
                    Give their work back
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </>
  );
}
