import { Fragment, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link, useNavigate, useParams, useSearchParams } from "react-router-dom";
import { api } from "../api/client";
import { unwrap } from "../api/queries";
import { Empty } from "../ui/Empty";
import { Failed } from "../ui/Failed";
import { Exploited, Severity } from "../ui/Severity";

const PAGE = 50;

// How old the issue is, from the year in its identifier.
//
// Not a disclosure date and not called one — REJ-11 declined to store one, and
// the identifier's year is the year it was assigned. It is enough for the
// question the column answers: an unfixed issue from years ago and one from
// last month are different situations, and at that distance a few months
// either way changes nothing.
function yearsOld(identifier: string | undefined): number | null {
  const year = /^(?:CVE|GHSA-[^-]*)-(\d{4})-/.exec(identifier ?? "")?.[1];
  if (!year) return null;
  const age = new Date().getUTCFullYear() - Number(year);
  return age >= 0 ? age : null;
}

// What upstream has done, said rather than left to be inferred from a blank.
// "No fix" and "upstream declined" are different answers and only one of them
// means somebody is still waiting.
function upstreamSays(state: string | undefined, fixedIn: string | undefined) {
  if (fixedIn) return { text: fixedIn, kind: "id" as const };
  switch (state) {
    case "wont-fix":
      return { text: "declined", kind: "note" as const };
    case "none":
      return { text: "none yet", kind: "note" as const };
    default:
      return { text: "—", kind: "faint" as const };
  }
}
const FLOORS = ["low", "medium", "high", "critical"] as const;

// The package kinds a real image carries, most numerous first. Taken from a
// switch operating-system image rather than from a registry's list of every
// kind that exists: offering twenty when a build has eight is a menu somebody
// reads past.
const ECOSYSTEMS = [
  ["", "any kind"], ["generic", "generic"], ["golang", "Go"], ["deb", "Debian"],
  ["cargo", "Rust"], ["pypi", "Python"], ["oci", "container"],
  ["github", "GitHub"], ["maven", "Maven"],
] as const;

// How far a group has been decided. A group covers every place an issue sits
// at in one component, so each of these is a statement about all of them.
const STATES = [
  ["", "any state"], ["undecided", "nobody has decided"],
  ["waiting", "waiting for a second person"], ["agreed", "answered everywhere"],
  ["lapsed", "stopped applying"],
] as const;

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
  // Whether to show what this product does not consider worth triaging. They
  // are always recorded and always counted; this asks to see them here.
  const below = params.get("below") === "yes";
  const searching = params.get("q") ?? "";
  const ecosystem = params.get("ecosystem") ?? "";
  const under = params.get("under") ?? "";
  const underBuild = params.get("under_build") === "yes";
  const state = params.get("state") ?? "";
  // How many narrowings are on beyond the chips, so the panel says so while it
  // is shut. A filter nobody can see is how two people read one screen and
  // quote different numbers (REJ-10).
  const advanced = [ecosystem, under, state].filter(Boolean).length + (underBuild ? 1 : 0);
  const [more, setMore] = useState(advanced > 0);
  const [peeking, setPeeking] = useState<string | null>(null);
  // What is typed, before it is asked for. Submitted rather than sent per
  // keystroke: each one is a query over every open finding in the build, and
  // the answer to a half-typed word is not worth asking for.
  const [typed, setTyped] = useState(searching);

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
    ...(searching ? { q: searching } : {}),
    ...(ecosystem ? { ecosystem } : {}),
    ...(under ? { under } : {}),
    ...(underBuild ? { under_build: true } : {}),
    ...(state ? { state: state as "undecided" | "waiting" | "agreed" | "lapsed" } : {}),
    ...(hiding.length > 0 ? { exclude: hiding } : {}),
    ...(onlyComponent ? { component: onlyComponent } : {}),
    ...(below ? { below_floor: true } : {}),
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
        {/* Searching is the way into a list this size, so it comes first.
            Submitted rather than sent per keystroke, and in the URL like every
            other filter so a link carries what somebody was looking at. */}
        <form
          style={{ display: "contents" }}
          onSubmit={(event) => {
            event.preventDefault();
            set("q", typed.trim());
          }}
        >
          <input
            type="search"
            value={typed}
            onChange={(event) => setTyped(event.target.value)}
            placeholder="Find a component — openssl, linux, python…"
            aria-label="Find a component"
            style={{ width: 260 }}
          />
          <button type="submit" className="btn ghost">Search</button>
          {searching && (
            <button
              type="button"
              className="linkish"
              onClick={() => {
                setTyped("");
                set("q", "");
              }}
            >
              Clear “{searching}”
            </button>
          )}
        </form>
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
        {/* Behind a control rather than always on screen: the chips above are
            what somebody uses constantly, and putting six more beside them
            makes the common case slower to reach. What is on is said on the
            button, so a narrowed list never looks like an unnarrowed one. */}
        <button
          type="button"
          className="chip"
          aria-pressed={more}
          aria-expanded={more}
          onClick={() => setMore(!more)}
        >
          More{advanced > 0 ? ` · ${advanced}` : ""}
        </button>
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

      {more && (
        <div className="advanced">
          <label className="field">
            <span>Package kind</span>
            <select value={ecosystem} onChange={(e) => set("ecosystem", e.target.value)}>
              {ECOSYSTEMS.map(([value, label]) => (
                <option key={value} value={value}>{label}</option>
              ))}
            </select>
          </label>

          <label className="field">
            <span>Inside</span>
            <input
              type="text"
              value={under}
              disabled={underBuild}
              placeholder="a container, by name"
              onChange={(e) => set("under", e.target.value)}
            />
          </label>

          {/* The other half of the same question. What the build holds
              directly has no container above it to name, so it cannot be
              asked for by typing one. */}
          <label className="field row">
            <input
              type="checkbox"
              checked={underBuild}
              onChange={(e) => set("under_build", e.target.checked ? "yes" : "")}
            />
            <span>Held by the build itself</span>
          </label>

          <label className="field">
            <span>How far decided</span>
            <select value={state} onChange={(e) => set("state", e.target.value)}>
              {STATES.map(([value, label]) => (
                <option key={value} value={value}>{label}</option>
              ))}
            </select>
          </label>

          {advanced > 0 && (
            <button
              type="button"
              className="linkish"
              onClick={() => {
                const next = new URLSearchParams(params);
                for (const key of ["ecosystem", "under", "under_build", "state"]) {
                  next.delete(key);
                }
                next.delete("offset");
                setParams(next);
              }}
            >
              Clear these
            </button>
          )}
        </div>
      )}

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

      {/* A list that is hiding something says so, with the count. Somebody
          else set this line once, for everybody, so a smaller number with
          nothing explaining it is how two people quote different figures for
          one question (TRI-44, REJ-10). */}
      {(findings.data?.hidden ?? 0) > 0 && (
        <div className="filters" style={{ marginTop: -4 }}>
          <span className="hint">
            {(findings.data?.hidden ?? 0).toLocaleString()} more are below what this product
            triages ({findings.data?.floor}). They are still recorded and still counted.
          </span>
          <button type="button" className="linkish" onClick={() => set("below", "yes")}>
            Show them too
          </button>
        </div>
      )}
      {below && (
        <div className="filters" style={{ marginTop: -4 }}>
          <span className="hint">
            Showing what is below the line as well as above it.
          </span>
          <button type="button" className="linkish" onClick={() => set("below", "")}>
            Back to what is triaged
          </button>
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
            ...(searching ? { q: searching } : {}),
            ...(ecosystem ? { ecosystem } : {}),
            ...(under ? { under } : {}),
            ...(underBuild ? { under_build: true } : {}),
            ...(state ? { state: state as "undecided" | "waiting" | "agreed" | "lapsed" } : {}),
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
                {/* Both ends of the way down, middle collapsed (UIX-12).
                    The top says which part of the product this is, the bottom
                    says what pulls it in and is therefore what a decision is
                    about; the steps between rarely tell two rows apart. */}
                <th>Where it sits</th>
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
                        {/* How long it has been unanswered. Shown only where
                            it is worth reacting to: everything is at least a
                            few months old and saying so on every row is noise. */}
                        {(() => {
                          const age = yearsOld(row.vulnerability);
                          return age !== null && age >= 2 ? (
                            <span className="hint" style={{ marginLeft: 6 }}>
                              {age} years old
                            </span>
                          ) : null;
                        })()}
                      </td>
                      <td>
                        <button
                          type="button"
                          className="linkish id"
                          title={`Everything open against ${row.component}`}
                          onClick={(event) => {
                            event.stopPropagation();
                            set("component", row.component ?? "");
                          }}
                        >
                          {row.component}
                        </button>
                        <br />
                        <span className="id" style={{ color: "var(--faint)" }}>{row.version}</span>
                        {/* Hiding lives here, not only on the by-component
                            view: this is the list somebody triages, and one
                            package drowning it is what they are getting past.
                            On a switch image the kernel is 4,943 rows of
                            6,822, and it is not what somebody triaging
                            userland is looking for. */}
                        <button
                          type="button"
                          className="linkish hideit"
                          title={`Hide ${row.component} from this list`}
                          onClick={(event) => {
                            event.stopPropagation();
                            hide(row.component ?? "");
                          }}
                        >
                          hide
                        </button>
                      </td>
                      <td>
                        <Sits row={row} />
                      </td>
                      <td className="num hint">
                        {row.likelihood ? row.likelihood.toFixed(3) : "—"}
                      </td>
                      <td>
                        {(() => {
                          const said = upstreamSays(row.fix_state, row.fixed_in);
                          return (
                            <span
                              className={said.kind === "id" ? "id" : "hint"}
                              style={said.kind === "faint" ? { color: "var(--faint)" } : undefined}
                            >
                              {said.text}
                            </span>
                          );
                        })()}
                      </td>
                      <td className="num">
                        {row.places}
                        {(row.answered ?? 0) > 0 && (
                          <span className="hint"> · {row.answered} answered</span>
                        )}
                      </td>
                    </tr>
                    {peeking === key && (
                      <tr className="places">
                        <td colSpan={8}>
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

// Where a component sits, as the two ends that differ between sibling rows.
//
// Where the row covers places reached more than one way, it says so rather
// than presenting one route as though it were the only one: the pair shown is
// one of several, and a reader deciding about "the" parent would be deciding
// about a parent they were not shown.
function Sits({
  row,
}: {
  row: { owner?: string; parent?: string; middle?: number; chains?: number };
}) {
  if (!row.owner && !row.parent) {
    return <span className="hint">nothing records what pulls this in</span>;
  }
  const same = row.owner === row.parent;
  return (
    <span className="hint">
      <span className="id">{row.owner}</span>
      {!same && (
        <>
          {row.middle ? ` › +${row.middle} › ` : " › "}
          <span className="id">{row.parent}</span>
        </>
      )}
      {(row.chains ?? 0) > 1 && <> · one of {row.chains}</>}
    </span>
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
                    {/* The name is the way in. It used to be plain text with a
                        button beside it saying "only this", which is the same
                        act named twice — and the thing somebody reaches for
                        first is the name. */}
                    <button
                      type="button"
                      className="linkish id"
                      title={`What is open against ${name}`}
                      onClick={() => onOnly(name)}
                    >
                      {name}
                    </button>
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
            {place.consumer
              ? `under ${place.consumer}`
              : // No consumer means the build itself contains it, which the
                // chain states; only an empty chain means nothing was recorded.
                place.chain?.[0]?.component
                ? `under ${place.chain[0].component}`
                : "nothing records what pulls this in"}
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
