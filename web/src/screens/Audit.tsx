import { useSearchParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { api, type Body } from "../api/client";
import { unwrap } from "../api/queries";
import { Empty } from "../ui/Empty";
import { Failed } from "../ui/Failed";
import { Markdown } from "../ui/Markdown";

type Judged = Body<"JudgedBody">;

const OUTCOMES = [
  ["", "every judgment"],
  ["not-applicable", "dismissed — not applicable"],
  ["wont-fix", "dismissed — will not fix"],
  ["deferred", "deferred"],
  ["affected", "affected"],
] as const;

const STATES = [
  ["", "any state"],
  ["approved", "agreed"],
  ["proposed", "waiting"],
  ["lapsed", "lapsed"],
  ["withdrawn", "withdrawn"],
] as const;

// The record of what was judged, for somebody auditing it.
//
// Its own screen rather than a filter on the findings list, because the unit
// differs: a finding is a thing that might be wrong, and this is a judgment
// somebody made about one — with who made it, who agreed, and when each
// happened. The findings list answers "what is open"; this answers "what did
// you decide, and on whose say-so".
//
// **Built to be printed.** An auditor takes a copy away, so the page prints as
// the record rather than as a screenshot of an application: the shell, the
// controls and the links go, a header states what was asked for and when it was
// taken, and a judgment does not break across a page.
export function Audit() {
  const [params, setParams] = useSearchParams();
  const product = params.get("product") ?? "";
  const outcome = params.get("outcome") ?? "";
  const state = params.get("state") ?? "";
  const from = params.get("from") ?? "";
  const to = params.get("to") ?? "";

  function set(key: string, value: string) {
    const next = new URLSearchParams(params);
    if (value) next.set(key, value);
    else next.delete(key);
    setParams(next);
  }

  const products = useQuery({
    queryKey: ["products"],
    queryFn: async () => unwrap(await api.GET("/v1/products", {})),
  });
  const record = useQuery({
    queryKey: ["audit", product, outcome, state, from, to],
    queryFn: async () =>
      unwrap(
        await api.GET("/v1/audit", {
          params: {
            query: {
              limit: 500,
              ...(product ? { product } : {}),
              ...(outcome
                ? { outcome: outcome as "affected" | "not-applicable" | "deferred" | "wont-fix" }
                : {}),
              ...(state
                ? { state: state as "proposed" | "approved" | "withdrawn" | "lapsed" }
                : {}),
              ...(from ? { from } : {}),
              ...(to ? { to } : {}),
            },
          },
        }),
      ),
  });

  const rows = record.data?.items ?? [];
  const total = record.data?.total ?? 0;
  // Said on the printed copy, because a page of judgments with no statement of
  // what was asked for is a page nobody can check.
  const asked = [
    product || "every product you can see",
    OUTCOMES.find(([v]) => v === outcome)?.[1] ?? "",
    STATES.find(([v]) => v === state)?.[1] ?? "",
    from || to ? `proposed ${from || "at any time"} to ${to || "now"}` : "",
  ]
    .filter(Boolean)
    .join(" · ");

  return (
    <>
      <div className="screen-head">
        <h2>
          The record <span className="n">{total.toLocaleString()}</span>
        </h2>
        <p>
          Every judgment made, with who made it, who agreed, and when. What the findings list
          answers is what is open; this answers what was decided and on whose say-so.
        </p>
        <span style={{ marginLeft: "auto" }} className="noprint">
          <button type="button" className="btn" onClick={() => window.print()}>
            Print
          </button>
        </span>
      </div>

      <div className="filters noprint">
        <label className="field">
          <span>Product</span>
          <select value={product} onChange={(e) => set("product", e.target.value)}>
            <option value="">every product</option>
            {(products.data?.items ?? []).map((each) => (
              <option key={each.name} value={each.name}>
                {each.display_name || each.name}
              </option>
            ))}
          </select>
        </label>
        <label className="field">
          <span>Judgment</span>
          <select value={outcome} onChange={(e) => set("outcome", e.target.value)}>
            {OUTCOMES.map(([value, label]) => (
              <option key={value} value={value}>
                {label}
              </option>
            ))}
          </select>
        </label>
        <label className="field">
          <span>State</span>
          <select value={state} onChange={(e) => set("state", e.target.value)}>
            {STATES.map(([value, label]) => (
              <option key={value} value={value}>
                {label}
              </option>
            ))}
          </select>
        </label>
        <label className="field">
          <span>Proposed from</span>
          <input type="date" value={from} onChange={(e) => set("from", e.target.value)} />
        </label>
        <label className="field">
          <span>to</span>
          <input type="date" value={to} onChange={(e) => set("to", e.target.value)} />
        </label>
      </div>

      {/* Only on paper. A printed record has to say what it is a record of and
          when it was taken, or nobody reading it later can check it. */}
      <div className="printhead">
        <h1>OpenPSIRT — record of judgments</h1>
        <p>
          {asked} · {total.toLocaleString()} {total === 1 ? "judgment" : "judgments"} · taken{" "}
          {new Date().toISOString().slice(0, 16).replace("T", " ")}Z
        </p>
      </div>

      {record.isPending ? (
        <p className="hint">Loading…</p>
      ) : record.isError ? (
        <Failed error={record.error} what="The record could not be read." />
      ) : rows.length === 0 ? (
        <Empty
          title="No judgment matches that."
          detail="Widen the dates, or clear the filters — a period with nothing decided in it is also an answer."
        />
      ) : (
        <>
          {rows.map((row) => (
            <Judgment key={row.id} row={row} />
          ))}
          {total > rows.length && (
            <p className="hint noprint">
              Showing the newest {rows.length.toLocaleString()} of {total.toLocaleString()}. Narrow
              the dates to print the rest.
            </p>
          )}
        </>
      )}
    </>
  );
}

function Judgment({ row }: { row: Judged }) {
  const standing = row.approvals?.filter((a) => !a.withdrawn_at) ?? [];
  const withdrawn = row.approvals?.filter((a) => a.withdrawn_at) ?? [];

  return (
    <div className="judgment">
      <header>
        <span className="id">{row.issue}</span>
        <span className={`claimed ${row.outcome}`}>
          <b>{row.outcome}</b>
          {row.justification && <span className="why mono">{row.justification}</span>}
        </span>
        <span className={`state ${row.standing ? "agreed" : "open"}`}>
          {row.standing ? "stands" : row.state}
        </span>
      </header>

      <dl className="facts">
        <dt>About</dt>
        <dd>
          <span className="id">{row.component}</span>{" "}
          <span className="id" style={{ color: "var(--faint)" }}>
            {row.version}
          </span>
          {row.consumer && (
            <>
              {" "}
              in <span className="id">{row.consumer}</span>
            </>
          )}{" "}
          · {row.product}
        </dd>

        <dt>Proposed</dt>
        <dd>
          <b>{row.proposed_by}</b> · {row.proposed_at}
        </dd>

        <dt>Agreed</dt>
        <dd>
          {standing.length === 0 ? (
            <span className="hint">nobody yet</span>
          ) : (
            standing.map((a, i) => (
              <span key={i}>
                {i > 0 && ", "}
                <b>{a.by}</b> · {a.at}
              </span>
            ))
          )}
          {/* Two different people is the control, so the record says whether
              this one has it rather than leaving a reader to compare names. */}
          {row.two_people ? (
            <span className="state agreed" style={{ marginLeft: 8 }}>
              two people
            </span>
          ) : (
            standing.length > 0 && (
              <span className="state lapsed" style={{ marginLeft: 8 }}>
                same person
              </span>
            )
          )}
        </dd>

        {withdrawn.length > 0 && (
          <>
            <dt>Taken back</dt>
            <dd>
              {withdrawn.map((a, i) => (
                <span key={i}>
                  {i > 0 && ", "}
                  <b>{a.by}</b> agreed {a.at}, withdrawn {a.withdrawn_at}
                </span>
              ))}
            </dd>
          </>
        )}

        {row.deferred_until && (
          <>
            <dt>Until</dt>
            <dd>{row.deferred_until}</dd>
          </>
        )}

        {row.mitigation && (
          <>
            <dt>Control named</dt>
            <dd>
              {row.mitigation}
              {/* Said here rather than left implicit. This is the one claim
                  nothing here can notice going away. */}
              <span className="hint"> — nothing here notices this being removed</span>
            </dd>
          </>
        )}

        {row.ended_at && (
          <>
            <dt>Stopped applying</dt>
            <dd>
              {row.ended_at} · {row.state}
            </dd>
          </>
        )}
      </dl>

      <div className="reasoning">
        <Markdown source={row.reasoning ?? ""} />
      </div>
    </div>
  );
}
