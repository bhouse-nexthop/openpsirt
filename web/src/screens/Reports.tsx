import { useQuery } from "@tanstack/react-query";
import { Link, useSearchParams } from "react-router-dom";
import { api } from "../api/client";
import { unwrap } from "../api/queries";
import { scopeQuery, useScope } from "../app/scope";
import { Failed } from "../ui/Failed";
import { Empty } from "../ui/Empty";
import { Severity } from "../ui/Severity";

// How long the figures cover. Thirty days is the window RPT-03 names and the
// one people quote; the others are here because a month is too short to see a
// quarter's shape and too long to see this week's.
const WINDOWS = [7, 30, 90] as const;

// What the tool can say about how the work is going, rather than about what
// the work is. Three reports that had endpoints or nothing and no screen to
// live on: how fast things are fixed, what keeps being put off, and what has
// been argued away.
//
// It follows the scope picker like every other cross-product screen.
export function Reports() {
  const at = useScope();
  const scope = scopeQuery(at);
  const [params, setParams] = useSearchParams();
  const days = Number(params.get("days") ?? 30);

  const pace = useQuery({
    queryKey: ["remediation", scope, days],
    queryFn: async () =>
      unwrap(await api.GET("/v1/remediation", { params: { query: { days, ...scope } } })),
  });
  const repeated = useQuery({
    queryKey: ["repeated", at.product ?? ""],
    queryFn: async () =>
      unwrap(
        await api.GET("/v1/deferrals/repeated", {
          params: { query: { limit: 50, ...(at.product ? { product: at.product } : {}) } },
        }),
      ),
  });

  return (
    <>
      <div className="screen-head">
        <h2>Reports</h2>
        <p>
          {at.product ?? "Every product"} · {at.stream ?? "every branch"} ·{" "}
          {at.variant ?? "every variant"} — how the work is going, rather than what it is.
        </p>
      </div>

      <div className="controls">
        <div className="seg" role="group" aria-label="Window">
          {WINDOWS.map((n) => (
            <button
              key={n}
              type="button"
              aria-pressed={days === n}
              onClick={() => {
                const next = new URLSearchParams(params);
                next.set("days", String(n));
                setParams(next);
              }}
            >
              {n} days
            </button>
          ))}
        </div>
      </div>

      <section className="panel" style={{ marginTop: 14 }}>
        <h3>Keeping pace</h3>
        {pace.isError ? (
          <Failed error={pace.error} what="How fast things are fixed could not be read." />
        ) : (
          <>
            <div className="kpis" style={{ marginTop: 8 }}>
              <div className="kpi">
                <span className="l">Fixed</span>
                <span className="n">{(pace.data?.fixed ?? 0).toLocaleString()}</span>
                <span className="d">
                  issues that actually went away · a version that carried the issue with it is not
                  a fix
                </span>
              </div>
              <div className="kpi">
                <span className="l">Appeared</span>
                <span className="n">{(pace.data?.opened ?? 0).toLocaleString()}</span>
                <span className="d">
                  in the same window and the same unit, so the two can be read against each other
                </span>
              </div>
            </div>

            <h4 style={{ marginTop: 14 }}>Average time to fix</h4>
            {Object.keys(pace.data?.time_to_fix ?? {}).length === 0 ? (
              <p className="hint">Nothing closed in this window.</p>
            ) : (
              <ul className="files">
                {Object.entries(pace.data?.time_to_fix ?? {})
                  .sort()
                  .map(([band, hours]) => (
                    <li key={band}>
                      <Severity word={band} />{" "}
                      <b>{Math.round((hours as number) / 24)}</b> days
                    </li>
                  ))}
              </ul>
            )}

            <h4 style={{ marginTop: 14 }}>What is aging</h4>
            <div className="tablewrap">
              <table>
                <thead>
                  <tr>
                    <th>Open for</th>
                    <th className="num">Issues</th>
                  </tr>
                </thead>
                <tbody>
                  {(pace.data?.aging ?? []).map((bucket) => (
                    <tr key={bucket.label}>
                      <td>{bucket.label}</td>
                      <td className="num">{(bucket.open ?? 0).toLocaleString()}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </>
        )}
      </section>

      <section className="panel" style={{ marginTop: 14 }}>
        <h3>What keeps being put off</h3>
        <p className="hint">
          One item deferred three times is a judgment. Forty of them is a policy nobody wrote
          down, and that is what this is for.
        </p>
        {repeated.isError ? (
          <Failed error={repeated.error} what="Repeat deferrals could not be read." />
        ) : (repeated.data?.items ?? []).length === 0 ? (
          <Empty
            title="Nothing has been put off more than once."
            detail="Deferrals are recorded either way; this lists only the ones that repeat."
          />
        ) : (
          <div className="tablewrap">
            <table>
              <thead>
                <tr>
                  <th>Issue</th>
                  <th>Product</th>
                  <th className="num">Times</th>
                  <th className="num">Total days</th>
                  <th>State</th>
                </tr>
              </thead>
              <tbody>
                {(repeated.data?.items ?? []).map((row) => (
                  <tr key={`${row.product} ${row.vulnerability} ${row.place}`}>
                    <td>
                      <Severity word={row.severity} />{" "}
                      <span className="id">{row.vulnerability}</span>
                    </td>
                    <td>{row.product}</td>
                    <td className="num">{row.times}</td>
                    <td className="num">{row.total_days}</td>
                    <td>
                      {row.standing ? (
                        <span className="state waiting">Still deferred</span>
                      ) : (
                        <span className="hint">ran out</span>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>

      <section className="panel" style={{ marginTop: 14 }}>
        <h3>What was argued away</h3>
        <p className="hint">
          Every dismissal, with its reasoning and who agreed to it, is in{" "}
          <Link to="/audit">the record</Link>.
        </p>
      </section>
    </>
  );
}
