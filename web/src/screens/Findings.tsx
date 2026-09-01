import { Fragment, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link, useNavigate, useParams, useSearchParams } from "react-router-dom";
import { api } from "../api/client";
import { unwrap } from "../api/queries";
import { Empty } from "../ui/Empty";
import { Failed } from "../ui/Failed";
import { Exploited, Severity } from "../ui/Severity";

const PAGE = 50;
const FLOORS = ["low", "medium", "high", "critical"] as const;

// One row per issue in a component, not per place. A real image produced
// 335,021 individual findings that collapse to 7,906 rows here, so the
// grouping is not a nicety — ungrouped it is six thousand screens of rows
// differing in a column nobody reads.
//
// Every filter is in the URL, so a link carries what somebody is looking at.
export function Findings() {
  const { product = "", stream = "", variant = "" } = useParams();
  const [params, setParams] = useSearchParams();
  const navigate = useNavigate();
  const offset = Number(params.get("offset") ?? 0);
  const floor = params.get("floor") ?? "low";
  const only = params.get("only") ?? "";
  const view = params.get("view") ?? "issues";
  const hiding = (params.get("hide") ?? "").split(",").filter(Boolean);
  // Set by "only this" in the by-component view, which is how somebody moves
  // from "where is the weight" to "what is actually wrong with it".
  const onlyComponent = params.get("component") ?? "";
  const [peeking, setPeeking] = useState<string | null>(null);

  // The narrowing is the server's. Filtering a page that has already been
  // fetched answers a different question from the one it appears to —
  // "exploited" over fifty rows means exploited among those fifty — and leaves
  // the total beside it counting something else, which is the number people
  // quote (REJ-10).
  const query = {
    limit: PAGE,
    offset,
    ...(floor !== "low" ? { severity: floor as (typeof FLOORS)[number] } : {}),
    ...(only === "exploited" ? { exploited: true } : {}),
    ...(only === "hasFix" ? { fixable: true } : {}),
    ...(hiding.length > 0 ? { exclude: hiding } : {}),
    ...(onlyComponent ? { component: onlyComponent } : {}),
  };

  const findings = useQuery({
    queryKey: ["findings", product, stream, variant, query],
    queryFn: async () =>
      unwrap(
        await api.GET("/v1/products/{product}/streams/{stream}/variants/{variant}/findings", {
          params: { path: { product, stream, variant }, query },
        }),
      ),
    enabled: view === "issues",
  });

  function set(key: string, value: string) {
    const next = new URLSearchParams(params);
    if (value) next.set(key, value);
    else next.delete(key);
    next.delete("offset");
    setParams(next);
  }

  function hide(component: string) {
    set("hide", [...new Set([...hiding, component])].join(","));
  }

  function unhide(component: string) {
    set("hide", hiding.filter((name) => name !== component).join(","));
  }

  const controls = (
    <>
      <div className="filters">
        <span className="seg">
          {[
            ["issues", "By issue"],
            ["components", "By component"],
          ].map(([value, label]) => (
            <button
              key={value}
              type="button"
              aria-pressed={view === value}
              onClick={() => set("view", value === "issues" ? "" : (value as string))}
            >
              {label}
            </button>
          ))}
        </span>
        {[
          ["", "Open"],
          ["exploited", "Exploited"],
          ["hasFix", "Fix available"],
        ].map(([value, label]) => (
          <button
            key={label}
            type="button"
            className="chip"
            aria-pressed={only === value}
            onClick={() => set("only", value as string)}
          >
            {label}
          </button>
        ))}
        <span className="floor">
          <span style={{ color: "var(--faint)" }}>At least</span>
          <span className="seg">
            {FLOORS.map((band) => (
              <button
                key={band}
                type="button"
                aria-pressed={floor === band}
                onClick={() => set("floor", band)}
              >
                {band[0]?.toUpperCase()}{band.slice(1)}
              </button>
            ))}
          </span>
        </span>
        <Link
          to={`/products/${product}/streams/${stream}/variants/${variant}/components`}
          className="linkish"
          style={{ marginLeft: "auto" }}
        >
          Dependencies →
        </Link>
      </div>

      {onlyComponent && (
        <div className="filters" style={{ marginTop: -4 }}>
          <span style={{ color: "var(--faint)" }}>Only</span>
          <button
            type="button"
            className="chip"
            aria-pressed
            title="Show every component again"
            onClick={() => set("component", "")}
          >
            {onlyComponent} ×
          </button>
          <span className="hint">
            Matched by name, so a build that vendors this twice answers with both.
          </span>
        </div>
      )}

      {hiding.length > 0 && (
        <div className="filters" style={{ marginTop: -4 }}>
          <span style={{ color: "var(--faint)" }}>Hiding</span>
          {hiding.map((name) => (
            <button
              key={name}
              type="button"
              className="chip"
              aria-pressed
              title="Show it again"
              onClick={() => unhide(name)}
            >
              {name} ×
            </button>
          ))}
          <span className="hint">
            Hidden everywhere on this page, and counted out of the total — a filter changes what
            <b> you</b> are looking at and never what anybody else is reported.
          </span>
        </div>
      )}
    </>
  );

  if (view === "components") {
    return (
      <>
        <div className="screen-head">
          <h2>Findings</h2>
          <p>
            {product} · {stream} · {variant} — one row per component, so you can see where the
            weight is before deciding what to read.
          </p>
        </div>
        {controls}
        <ByComponent
          at={{ product, stream, variant }}
          query={{
            limit: PAGE,
            offset,
            ...(floor !== "low" ? { severity: floor as (typeof FLOORS)[number] } : {}),
            ...(only === "exploited" ? { exploited: true } : {}),
            ...(only === "hasFix" ? { fixable: true } : {}),
            ...(hiding.length > 0 ? { exclude: hiding } : {}),
          }}
          offset={offset}
          onHide={hide}
          onOnly={(name) => {
            const next = new URLSearchParams(params);
            next.delete("view");
            next.delete("offset");
            next.set("component", name);
            setParams(next);
          }}
          onPage={(next) => {
            const now = new URLSearchParams(params);
            if (next === 0) now.delete("offset");
            else now.set("offset", String(next));
            setParams(now);
          }}
        />
      </>
    );
  }

  if (findings.isPending) return <p className="hint">Loading…</p>;
  if (findings.isError) {
    return <Failed error={findings.error} what="The findings could not be read." />;
  }

  const rows = findings.data?.items ?? [];
  const total = findings.data?.total ?? 0;

  // Names carried at more than one version on this page. They read as repeats
  // and are not: a build that vendors a library twice ships two of it.
  const seen = new Map<string, Set<string>>();
  for (const row of rows) {
    const versions = seen.get(row.component ?? "") ?? new Set<string>();
    versions.add(row.version ?? "");
    seen.set(row.component ?? "", versions);
  }
  const sameName = new Set(
    [...seen.entries()].filter(([, versions]) => versions.size > 1).map(([name]) => name),
  );

  return (
    <>
      <div className="screen-head">
        <h2>Findings</h2>
        <p>
          {product} · {stream} · {variant} — one row per issue and component, however many
          places it sits at.
        </p>
      </div>

      {controls}

      <p className="hint" style={{ margin: "0 0 8px" }}>
        Most urgent first: <b>known-exploited</b>, then whether the build reaches customers,
        then <b>severity</b>, then how likely exploitation is. The score is shown beside the
        word because it is what the order compares — two rows can tie on 10.0 while one reads
        "high" and the other "critical", which is the two CVSS generations disagreeing about
        vocabulary rather than the list being unsorted.
      </p>
      <p className="hint" style={{ margin: "0 0 8px" }}>
        Click a row to open the finding. The arrow beside it previews what the issue is, without
        leaving the list.
      </p>

      {sameName.size > 0 && (
        <p className="hint" style={{ margin: "0 0 8px" }}>
          {sameName.size === 1 ? "One component appears" : `${sameName.size} components appear`}{" "}
          at more than one version on this page. Those rows are not repeats — a build that
          vendors a library twice carries two of it, and each is its own thing to fix.
        </p>
      )}

      {rows.length === 0 ? (
        <Empty
          title="Nothing matches what you are looking at."
          detail="Everything here is below the floor you set, or outside the filter."
        />
      ) : (
        <div className="tablewrap">
          <table>
            <thead>
              <tr>
                <th style={{ width: 30 }} />
                <th>Severity</th>
                <th>Issue</th>
                <th>Component</th>
                <th className="num" title="Published estimate that this will be exploited. It orders things that are equally severe">Likely</th>
                <th>Fix</th>
                <th className="num">Places</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((row) => {
                const key = `${row.vulnerability}\u0000${row.component}\u0000${row.version}`;
                const at = to(product, stream, variant, row);
                return (
                  <Fragment key={key}>
                    <tr className="row" onClick={() => navigate(at)}>
                      <td>
                        <button
                          type="button"
                          className="peek"
                          aria-expanded={peeking === key}
                          title="Preview without leaving the list"
                          onClick={(event) => {
                            event.stopPropagation();
                            setPeeking(peeking === key ? null : key);
                          }}
                        >
                          {peeking === key ? "▾" : "▸"}
                        </button>
                      </td>
                      <td>
                        <Severity word={row.severity} />
                        {/* The number the order actually compares. Two rows
                            can tie on it while their words disagree — a 2003
                            issue scored 10.0 reads "high" under CVSS v2 and
                            "critical" under v3 — and without it that reads as
                            a list sorted wrongly. */}
                        {row.score ? (
                          <span className="hint" style={{ marginLeft: 6 }}>{row.score.toFixed(1)}</span>
                        ) : null}
                      </td>
                      <td>
                        <Link to={at} className="id" onClick={(e) => e.stopPropagation()}>
                          {row.vulnerability}
                        </Link>{" "}
                        {/* Why the first rows are where they are. Without it
                            the order reads as arbitrary: an exploited high
                            above an unexploited critical is correct and looks
                            like nothing at all. */}
                        <Exploited when={row.exploited} />
                      </td>
                      <td>
                        <span className="id">{row.component}</span>
                        <br />
                        <span className="id" style={{ color: "var(--faint)" }}>{row.version}</span>
                      </td>
                      <td className="num hint">
                        {row.likelihood ? row.likelihood.toFixed(3) : "—"}
                      </td>
                      <td className="id">{row.fixed_in || "—"}</td>
                      <td className="num">
                        {row.places}
                        {(row.answered ?? 0) > 0 && (
                          <span className="hint"> · {row.answered} answered</span>
                        )}
                      </td>
                    </tr>
                    {peeking === key && (
                      <tr className="places">
                        <td colSpan={7}>
                          <Peek
                            at={{ product, stream, variant }}
                            vulnerability={row.vulnerability ?? ""}
                            component={row.component ?? ""}
                            version={row.version ?? ""}
                            to={at}
                          />
                        </td>
                      </tr>
                    )}
                  </Fragment>
                );
              })}
            </tbody>
          </table>
        </div>
      )}

      <p className="hint" style={{ margin: "12px 0 0" }}>
        Showing {rows.length.toLocaleString()} of {total.toLocaleString()} · ordered by urgency:
        exploited first, then whether it reaches customers, then likelihood, then severity
      </p>

      <Pager
        offset={offset}
        total={total}
        onGo={(next) => {
          const now = new URLSearchParams(params);
          if (next === 0) now.delete("offset");
          else now.set("offset", String(next));
          setParams(now);
        }}
      />
    </>
  );
}

// Where the weight is, rather than what is wrong. Somebody opening a list of
// several thousand rows needs to know that one package is most of it before
// they start reading — on a real image the kernel was 4,943 rows of 6,822, and
// ordered by urgency that looks like nothing but a long list.
function ByComponent({
  at,
  query,
  offset,
  onHide,
  onOnly,
  onPage,
}: {
  at: { product: string; stream: string; variant: string };
  query: Record<string, unknown>;
  offset: number;
  onHide: (component: string) => void;
  onOnly: (component: string) => void;
  onPage: (offset: number) => void;
}) {
  const grouped = useQuery({
    queryKey: ["findings-by-component", at, query],
    queryFn: async () =>
      unwrap(
        await api.GET(
          "/v1/products/{product}/streams/{stream}/variants/{variant}/findings/components",
          { params: { path: at, query: query as never } },
        ),
      ),
  });

  if (grouped.isPending) return <p className="hint">Loading…</p>;
  if (grouped.isError) {
    return <Failed error={grouped.error} what="What is open could not be read by component." />;
  }

  const rows = grouped.data?.items ?? [];
  const total = grouped.data?.total ?? 0;
  if (rows.length === 0) {
    return (
      <Empty
        title="Nothing matches what you are looking at."
        detail="Everything here is below the floor you set, or outside the filter."
      />
    );
  }

  const most = rows[0]?.issues ?? 0;

  return (
    <>
      <p className="hint" style={{ margin: "0 0 8px" }}>
        Ordered by how many issues each carries, not by urgency — that is the findings list, and
        repeating it here at worse resolution would answer nothing new. <b>Issues</b> is how many
        rows this component contributes to that list; <b>places</b> is how many times they sit
        somewhere in the build.
      </p>

      <div className="tablewrap">
        <table>
          <thead>
            <tr>
              <th>Component</th>
              <th className="num">Issues</th>
              <th className="num">Places</th>
              <th style={{ width: 150 }} />
            </tr>
          </thead>
          <tbody>
            {rows.map((row) => {
              const name = row.component ?? "";
              const share = most > 0 ? Math.round(((row.issues ?? 0) / most) * 100) : 0;
              return (
                <tr key={`${name}\u0000${row.version}`}>
                  <td>
                    <span className="id">{name}</span>
                    {row.exploited && (
                      <>
                        {" "}
                        <Exploited when />
                      </>
                    )}
                    <br />
                    <span className="id" style={{ color: "var(--faint)" }}>{row.version}</span>
                  </td>
                  <td className="num">
                    {(row.issues ?? 0).toLocaleString()}
                    {/* A bar rather than a percentage: the point is that one
                        row is an order of magnitude above the rest, which a
                        column of numbers hides and a length does not. */}
                    <span
                      aria-hidden
                      style={{
                        display: "block",
                        height: 3,
                        marginTop: 3,
                        borderRadius: 2,
                        background: "var(--accent)",
                        opacity: 0.55,
                        width: `${Math.max(share, 2)}%`,
                        marginLeft: "auto",
                      }}
                    />
                  </td>
                  <td className="num">{(row.places ?? 0).toLocaleString()}</td>
                  <td>
                    <button type="button" className="linkish" onClick={() => onOnly(name)}>
                      Only this →
                    </button>{" "}
                    <button
                      type="button"
                      className="linkish"
                      style={{ color: "var(--muted)" }}
                      title="Hide it from both views until you put it back"
                      onClick={() => onHide(name)}
                    >
                      Hide
                    </button>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>

      <p className="hint" style={{ margin: "12px 0 0" }}>
        Showing {rows.length.toLocaleString()} of {total.toLocaleString()} components that carry
        anything open
      </p>

      <Pager offset={offset} total={total} onGo={onPage} />
    </>
  );
}

// Opening a row is a look, not a commitment: what the issue actually says and
// where it sits, without leaving a list of a thousand rows and losing your
// place. Going to the finding itself is a link inside it.
function Peek({
  at,
  vulnerability,
  component,
  version,
  to: link,
}: {
  at: { product: string; stream: string; variant: string };
  vulnerability: string;
  component: string;
  version: string;
  to: string;
}) {
  const detail = useQuery({
    queryKey: ["finding", at, vulnerability, component, version],
    queryFn: async () =>
      unwrap(
        await api.GET(
          "/v1/products/{product}/streams/{stream}/variants/{variant}/findings/{vulnerability}/components/{component}",
          { params: { path: { ...at, vulnerability, component }, query: { version } } },
        ),
      ),
  });

  if (detail.isPending) return <p className="hint">Loading…</p>;
  if (detail.isError) return <Failed error={detail.error} what="This could not be read." />;
  const it = detail.data;

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
      {it?.description ? (
        <p style={{ margin: 0, fontSize: "var(--step--1)" }}>{it.description.slice(0, 420)}</p>
      ) : (
        <p className="hint" style={{ margin: 0 }}>The report says nothing beyond the identifier.</p>
      )}
      <div className="legend">
        {(it?.places ?? []).slice(0, 6).map((place) => (
          <span key={place.place}>
            {place.consumer ? `under ${place.consumer}` : "under the product itself"}
          </span>
        ))}
      </div>
      <Link to={link} className="linkish">Open the finding →</Link>
    </div>
  );
}

// Where a row opens. Clicking the row opens the thing it names (UIX-36); the
// arrow is the separate preview control, so neither steals the other's job.
function to(
  product: string,
  stream: string,
  variant: string,
  row: { vulnerability?: string; component?: string; version?: string },
): string {
  // The version is part of the address. A component name is not unique within
  // a build — a real image ships three vendored versions of one library — so a
  // link carrying only the name opens whichever was interned first, which for
  // most of those rows is a finding that does not exist.
  const query = row.version ? `?version=${encodeURIComponent(row.version)}` : "";
  return (
    `/products/${encodeURIComponent(product)}` +
    `/streams/${encodeURIComponent(stream)}` +
    `/variants/${encodeURIComponent(variant)}` +
    `/findings/${encodeURIComponent(row.vulnerability ?? "")}` +
    `/components/${encodeURIComponent(row.component ?? "")}` +
    query
  );
}

function Pager({
  offset,
  total,
  onGo,
}: {
  offset: number;
  total: number;
  onGo: (offset: number) => void;
}) {
  if (total <= PAGE) return null;
  const from = offset + 1;
  const upto = Math.min(offset + PAGE, total);
  return (
    <div style={{ marginTop: 14, display: "flex", alignItems: "center", gap: 12 }}>
      <span className="hint">
        {from.toLocaleString()}–{upto.toLocaleString()} of {total.toLocaleString()}
      </span>
      <span style={{ marginLeft: "auto", display: "flex", gap: 8 }}>
        <button
          type="button"
          className="chip"
          disabled={offset === 0}
          onClick={() => onGo(Math.max(0, offset - PAGE))}
        >
          Previous
        </button>
        <button
          type="button"
          className="chip"
          disabled={upto >= total}
          onClick={() => onGo(offset + PAGE)}
        >
          Next
        </button>
      </span>
    </div>
  );
}
