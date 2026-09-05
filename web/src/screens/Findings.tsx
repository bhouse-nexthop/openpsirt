import { Fragment, useEffect, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link, useNavigate, useParams, useSearchParams } from "react-router-dom";
import { api } from "../api/client";
import type { Body } from "../api/client";
import { unwrap } from "../api/queries";
import { Empty } from "../ui/Empty";
import { Failed } from "../ui/Failed";
import { Exploited, Severity } from "../ui/Severity";
import { Icon } from "../ui/Icons";
import { Decide, said, type Recorded } from "../ui/Decide";

const PAGE = 50;

// How old the issue is, from the year in its identifier.
//
// Not a disclosure date and not called one — REJ-11 declined to store one, and
// the identifier's year is the year it was assigned. It is enough for the
// question the column answers: an unfixed issue from years ago and one from
// last month are different situations.
function yearsOld(identifier: string | undefined): number | null {
  const year = /^(?:CVE|GHSA-[^-]*)-(\d{4})-/.exec(identifier ?? "")?.[1];
  if (!year) return null;
  const age = new Date().getUTCFullYear() - Number(year);
  return age >= 0 ? age : null;
}

// What upstream has done, said rather than left to be inferred from a blank.
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

// The package kinds a real image carries, most numerous first.
const ECOSYSTEMS = [
  ["", "any kind"],
  ["generic", "generic"],
  ["golang", "Go"],
  ["deb", "Debian"],
  ["cargo", "Rust"],
  ["pypi", "Python"],
  ["oci", "container"],
  ["github", "GitHub"],
  ["maven", "Maven"],
] as const;

// How far a group has been decided. A group covers every place an issue sits
// at in one component, so each of these is a statement about all of them.
const STATES = [
  ["", "any"],
  ["undecided", "undecided"],
  ["waiting", "pending approval"],
  ["agreed", "decided"],
  ["lapsed", "lapsed"],
] as const;

// Which judgment stands, which the state cannot say: "decided" means one
// stands, not which one. Asking what has been dismissed is this.
const OUTCOMES = [
  ["", "any"],
  ["not-applicable", "dismissed — not applicable"],
  ["wont-fix", "dismissed — will not fix"],
  ["deferred", "deferred"],
  ["affected", "affected"],
] as const;

const ASSIGNED = [
  ["", "anyone or nobody"],
  ["me", "me"],
  ["somebody", "somebody"],
  ["nobody", "nobody"],
] as const;

type Row = Body<"FindingBody">;

// What makes a row that row, for finding it again in a list read afresh.
function identityOf(row: Row): string {
  return `${row.vulnerability} ${row.component} ${row.version} ${row.ecosystem ?? ""}`;
}

// One row per issue in a component, not per place. Every filter is in the
// URL, so a link carries what somebody is looking at; every filter is the
// server's, so the total beside the list counts the same thing the list
// shows (REJ-10).
//
// Triage mode (UIX-43) opens the decision form inside the row: the list keeps
// the reader's place, the form keeps its width, and the keys do the walking.
export function Findings() {
  const { product = "", stream: named = "", variant: builtAs = "" } = useParams();
  const [params, setParams] = useSearchParams();
  // The branch and the variant come from the path on a build's own list and
  // from the picker's selection otherwise. Either may be "all": the list is
  // not one of the screens that needs a whole build (UIX-53).
  const stream = named || params.get("stream") || "";
  const variant = builtAs || params.get("variant") || "";
  const oneBuild = Boolean(stream && variant);
  // What the server needs to know about the selection, beside the filters.
  const selection = { ...(stream ? { stream } : {}), ...(variant ? { variant } : {}) };
  const navigate = useNavigate();
  const offset = Number(params.get("offset") ?? 0);
  const floor = params.get("floor") ?? "low";
  const only = params.get("only") ?? "";
  const view = params.get("view") ?? "issues";
  const hiding = (params.get("hide") ?? "").split(",").filter(Boolean);
  const onlyComponent = params.get("component") ?? "";
  const below = params.get("below") === "yes";
  const searching = params.get("q") ?? "";
  const ecosystem = params.get("ecosystem") ?? "";
  const under = params.get("under") ?? "";
  // Everything under a node of the dependency tree, by the tree's own walk,
  // so the tree's count and this list agree.
  const beneath = params.get("beneath") ?? "";
  const underBuild = params.get("under_build") === "yes";
  const state = params.get("state") ?? "";
  const outcome = params.get("outcome") ?? "";
  const assigned = params.get("assigned") ?? "";
  const reassessed = params.get("reassessed") === "1";
  // Reached only by comparing a published identifier against an upstream
  // version range, never against an advisory for the package in its own
  // ecosystem. A distribution backports fixes without moving the upstream
  // version, so these are neither confirmed nor refuted.
  const unconfirmed = params.get("unconfirmed") === "1";
  const triage = params.get("mode") === "triage";
  const advanced =
    [ecosystem, under, state, outcome, assigned].filter(Boolean).length +
    (underBuild ? 1 : 0) +
    (reassessed ? 1 : 0) +
    (unconfirmed ? 1 : 0);
  const [more, setMore] = useState(advanced > 0);
  const [peeking, setPeeking] = useState<string | null>(null);
  const [typed, setTyped] = useState(searching);
  // Triage mode's cursor and the row whose form is open.
  const [cursor, setCursor] = useState(0);
  const [deciding, setDeciding] = useState<number | null>(null);
  // Where triage mode goes once the list has been read again after a
  // decision: the row it was going to, by identity, and the index it was
  // going from should that row be gone as well.
  const [following, setFollowing] = useState<{ key: string; index: number; since: number } | null>(
    null,
  );
  // What the last decision recorded, kept beside the list so a build that
  // refused it is seen without leaving triage mode.
  const [recorded, setRecorded] = useState<Recorded | null>(null);

  const query = {
    limit: PAGE,
    offset,
    ...(floor !== "low" ? { severity: floor as (typeof FLOORS)[number] } : {}),
    ...(only === "exploited" ? { exploited: true } : {}),
    ...(only === "hasFix" ? { fixable: true } : {}),
    ...(searching ? { q: searching } : {}),
    ...(ecosystem ? { ecosystem } : {}),
    ...(under ? { under } : {}),
    ...(beneath ? { beneath } : {}),
    ...(underBuild ? { under_build: true } : {}),
    ...(state ? { state: state as "undecided" | "waiting" | "agreed" | "lapsed" } : {}),
    ...(outcome
      ? { outcome: outcome as "affected" | "not-applicable" | "deferred" | "wont-fix" }
      : {}),
    ...(assigned ? { assigned: assigned as "me" | "somebody" | "nobody" } : {}),
    ...(reassessed ? { reassessed: true } : {}),
    ...(unconfirmed ? { unconfirmed: true } : {}),
    ...(hiding.length > 0 ? { exclude: hiding } : {}),
    ...(onlyComponent ? { component: onlyComponent } : {}),
    ...(below ? { below_floor: true } : {}),
  };

  const findings = useQuery({
    queryKey: ["findings", product, stream, variant, query],
    queryFn: async () =>
      unwrap(
        await api.GET("/v1/products/{product}/findings", {
          params: { path: { product }, query: { ...query, ...selection } },
        }),
      ),
    enabled: view === "issues",
  });

  // Which build a row's actions and links are about. Where the selection is
  // one build that is the selection; across several the row names one of them
  // and says how many hold it, so the action lands somewhere real and the
  // decision reaches the rest by matching.
  function buildOf(row: Row) {
    return { product, stream: row.stream || stream, variant: row.variant || variant };
  }

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

  // Memoized because the fallback is a fresh array each render, which made
  // the effect that lands the cursor depend on something that always changed.
  const rows = useMemo(() => findings.data?.items ?? [], [findings.data]);
  const total = findings.data?.total ?? 0;

  // Once the list has been read again after a decision, land on the row
  // that was next by what it is. Under a state filter the decided row has
  // left the list, so the row after it now sits at the index the cursor was
  // moving from; advancing by index alone would step over it.
  useEffect(() => {
    if (!following || findings.dataUpdatedAt <= following.since) return;
    const at = rows.findIndex((row) => identityOf(row) === following.key);
    const landed = at >= 0 ? at : Math.min(following.index, Math.max(0, rows.length - 1));
    setCursor(landed);
    setDeciding(rows.length > 0 ? landed : null);
    setFollowing(null);
  }, [following, rows, findings.dataUpdatedAt]);

  // The keys, in triage mode: j and k move, Enter opens, Escape closes. The
  // form's own keys live in the form. Never while typing, and never under
  // the review sheet.
  useEffect(() => {
    if (!triage || view !== "issues") return;
    function key(event: KeyboardEvent) {
      if (document.body.dataset.sheet) return;
      const tag = (document.activeElement?.tagName ?? "").toUpperCase();
      if (/INPUT|TEXTAREA|SELECT/.test(tag)) {
        if (event.key === "Escape") (document.activeElement as HTMLElement).blur();
        return;
      }
      if (event.key === "Escape") {
        setDeciding(null);
      } else if (event.key === "j" || event.key === "ArrowDown") {
        event.preventDefault();
        setCursor((c) => {
          const next = Math.min(rows.length - 1, c + 1);
          setDeciding((d) => (d === null ? null : next));
          return next;
        });
      } else if (event.key === "k" || event.key === "ArrowUp") {
        event.preventDefault();
        setCursor((c) => {
          const next = Math.max(0, c - 1);
          setDeciding((d) => (d === null ? null : next));
          return next;
        });
      } else if (event.key === "Enter") {
        event.preventDefault();
        setDeciding((d) => (d === cursor ? null : cursor));
      }
    }
    document.addEventListener("keydown", key);
    return () => document.removeEventListener("keydown", key);
  }, [triage, view, rows.length, cursor]);

  useEffect(() => {
    if (!triage) return;
    const row = document.querySelector(`#findingRows tr.row[data-i="${cursor}"]`);
    row?.scrollIntoView({ block: "nearest" });
  }, [cursor, triage, deciding]);

  const controls = (
    <>
      <div className="filters">
        <form
          className="searchbox"
          onSubmit={(event) => {
            event.preventDefault();
            set("q", typed.trim());
          }}
        >
          <Icon name="search" />
          <input
            type="text"
            value={typed}
            onChange={(event) => setTyped(event.target.value)}
            placeholder="Find a component — openssl, linux, python…"
            aria-label="Find a component"
          />
        </form>
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
            what somebody uses constantly. What is on is said on the control,
            so a narrowed list never looks like an unnarrowed one. */}
        <button
          type="button"
          className="chip"
          aria-pressed={advanced > 0 || more}
          aria-expanded={more}
          onClick={() => setMore(!more)}
        >
          Filters{advanced > 0 ? ` · ${advanced}` : ""}
        </button>
        {view === "issues" && (
          <button
            type="button"
            className="chip mode"
            aria-pressed={triage}
            onClick={() => {
              setDeciding(null);
              set("mode", triage ? "" : "triage");
            }}
          >
            <Icon name="triage" />
            Triage mode
          </button>
        )}
        <span className="floor">
          <span style={{ color: "var(--faint)" }}>Min severity</span>
          <span className="seg">
            {FLOORS.map((band) => (
              <button
                key={band}
                type="button"
                aria-pressed={floor === band}
                onClick={() => set("floor", band)}
              >
                {band[0]?.toUpperCase()}
                {band.slice(1)}
              </button>
            ))}
          </span>
        </span>
      </div>

      {more && (
        <div className="advanced">
          <label className="field">
            <span>Package kind</span>
            <select value={ecosystem} onChange={(e) => set("ecosystem", e.target.value)}>
              {ECOSYSTEMS.map(([value, label]) => (
                <option key={value} value={value}>
                  {label}
                </option>
              ))}
            </select>
          </label>

          <label className="field">
            <span>Container</span>
            <input
              type="text"
              value={under}
              disabled={underBuild}
              placeholder="a container, by name"
              onChange={(e) => set("under", e.target.value)}
            />
          </label>

          <label className="field row">
            <input
              type="checkbox"
              checked={underBuild}
              onChange={(e) => set("under_build", e.target.checked ? "yes" : "")}
            />
            <span>Top-level only</span>
          </label>

          <label className="field">
            <span>Decision state</span>
            <select value={state} onChange={(e) => set("state", e.target.value)}>
              {STATES.map(([value, label]) => (
                <option key={value} value={value}>
                  {label}
                </option>
              ))}
            </select>
          </label>

          {/* What kind of judgment stands, which the state cannot say: "agreed"
              means one stands, not which one. This is how somebody asks what
              has been dismissed. */}
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
            <span>Assigned</span>
            <select value={assigned} onChange={(e) => set("assigned", e.target.value)}>
              {ASSIGNED.map(([value, label]) => (
                <option key={value} value={value}>
                  {label}
                </option>
              ))}
            </select>
          </label>

          <label className="check">
            <input
              type="checkbox"
              checked={reassessed}
              onChange={(e) => set("reassessed", e.target.checked ? "1" : "")}
            />
            <span>Rated differently by us</span>
          </label>

          <label
            className="check"
            title="A distribution backports fixes without moving the upstream version, so these matched an upstream range and may already be fixed here. Nobody has confirmed either way."
          >
            <input
              type="checkbox"
              checked={unconfirmed}
              onChange={(e) => set("unconfirmed", e.target.checked ? "1" : "")}
            />
            <span>Not confirmed by a packager</span>
          </label>

          {advanced > 0 && (
            <button
              type="button"
              className="linkish"
              onClick={() => {
                const next = new URLSearchParams(params);
                for (const key of [
                  "ecosystem",
                  "under",
                  "under_build",
                  "state",
                  "outcome",
                  "assigned",
                  "reassessed",
                  "unconfirmed",
                ]) {
                  next.delete(key);
                }
                next.delete("offset");
                setParams(next);
              }}
            >
              Clear these
            </button>
          )}
          <span className="hint" style={{ alignSelf: "center" }}>
            Active filters are counted on the <b>Filters</b> chip while this panel is closed.
          </span>
        </div>
      )}

      {beneath && (
        <div className="filters" style={{ marginTop: -4 }}>
          <span style={{ color: "var(--faint)" }}>Beneath</span>
          <button
            type="button"
            className="chip"
            aria-pressed
            title="Show the whole build again"
            onClick={() => set("beneath", "")}
          >
            {beneath} ×
          </button>
          <span className="hint">
            This component and everything under it in the dependency tree.
          </span>
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
            Hidden everywhere on this page and counted out of the total — a filter changes what{" "}
            <b>you</b> are looking at, never what anybody else is reported.
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
            {product} · {stream || "every branch"} · {variant || "every variant"} — one row per
            component, so you can see where the weight is before deciding what to read.
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
            ...(beneath ? { beneath } : {}),
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

  function advance(from: number, done: Recorded) {
    setRecorded(done);
    // Submitting from the list moves to the next row and opens it, so a
    // stretch of similar findings is decided without leaving the list. The
    // next row is remembered by what it is rather than by where it sits,
    // because the list is read again once the decision lands.
    const next = rows[from + 1];
    if (next) {
      setFollowing({ key: identityOf(next), index: from, since: findings.dataUpdatedAt });
      setCursor(from + 1);
      setDeciding(from + 1);
    } else {
      setFollowing(null);
      setDeciding(null);
    }
  }

  return (
    <>
      <div className="screen-head">
        <h2>
          Findings{" "}
          {/* Beside the heading, not only at the foot of the list. Choosing a
              filter is the moment somebody wants to know what it did, and a
              count that lives under a page of rows is a scroll away from the
              control that changed it. */}
          <span className="n" title="Matching the filters in force">
            {findings.isPending ? "…" : total.toLocaleString()}
          </span>
        </h2>
        <p>
          {product} · {stream || "every branch"} · {variant || "every variant"} — one row per issue
          and component, however many locations it sits at.
        </p>
      </div>

      {controls}

      {triage ? (
        <div className="modebar">
          <span>
            <b>Triage mode.</b> Decide from the list without leaving it.
          </span>
          <span className="keys">
            <kbd>j</kbd>
            <kbd>k</kbd> move <kbd>↵</kbd> open <kbd>1</kbd>–<kbd>5</kbd> outcome <kbd>r</kbd> or{" "}
            <kbd>ctrl</kbd>+<kbd>↵</kbd> submit and advance <kbd>esc</kbd> close
          </span>
          <span className="hint" style={{ marginLeft: "auto" }}>
            Same form as the finding screen, at full width
          </span>
        </div>
      ) : (
        <p className="hint" style={{ margin: "0 0 8px" }}>
          Ordered by urgency: exploited, then customer-facing, then severity, then EPSS. Click a row
          to open the finding; the arrow previews it in place.
        </p>
      )}

      {/* What the last decision from the list recorded, in the same words the
          finding screen uses — a build the guided review could not apply it to
          is reported here rather than dropped on the way to the next row. */}
      {triage && recorded && (
        <div className="alert info" style={{ marginBottom: 10 }}>
          <strong>Submitted</strong>
          <span>
            Recorded against {recorded.recorded}{" "}
            {recorded.recorded === 1 ? "location" : "locations"}
            {recorded.applied.filter((a) => a.ok).length > 0 && (
              <>
                , and in{" "}
                {recorded.applied
                  .filter((a) => a.ok)
                  .map((a) => a.build)
                  .join(", ")}
              </>
            )}
            .{" "}
            {recorded.needsApproval
              ? `The ${said(recorded.outcome)} takes effect once a second person approves it.`
              : "In force now."}
            {recorded.applied
              .filter((a) => !a.ok)
              .map((a) => (
                <Fragment key={a.build}>
                  <br />
                  <b>{a.build}</b>: {a.said}
                </Fragment>
              ))}
          </span>
          <button
            type="button"
            className="linkish"
            style={{ marginLeft: "auto" }}
            onClick={() => setRecorded(null)}
          >
            Dismiss
          </button>
        </div>
      )}

      {sameName.size > 0 && (
        <p className="hint" style={{ margin: "0 0 8px" }}>
          {sameName.size === 1 ? "One component appears" : `${sameName.size} components appear`} at
          more than one version on this page. Those rows are not repeats — a build that vendors a
          library twice carries two of it.
        </p>
      )}

      {rows.length === 0 ? (
        <Empty
          title="Nothing matches what you are looking at."
          detail="Everything here is below the floor you set, or outside the filter."
        />
      ) : (
        <div className="findings">
          <div className="tablewrap">
            <table>
              <thead>
                <tr>
                  <th style={{ width: 30 }} />
                  <th>Severity</th>
                  <th>Issue</th>
                  <th>Component</th>
                  {/* Both ends of the way down, middle collapsed (UIX-12). */}
                  <th>{oneBuild ? "Path" : "Build"}</th>
                  <th
                    className="num"
                    title="EPSS: published probability of exploitation. Orders findings of equal severity"
                  >
                    EPSS
                  </th>
                  <th>Fixed in</th>
                  <th className="num">Locations</th>
                  <th>State</th>
                </tr>
              </thead>
              <tbody id="findingRows">
                {rows.map((row, i) => {
                  const key = `${row.vulnerability} ${row.component} ${row.version} ${row.ecosystem ?? ""}`;
                  const at = to(buildOf(row), row);
                  // How far it is decided comes from the server, defined the
                  // way the state filter defines it; a row does not guess from
                  // what the build argued away, which is a different claim by
                  // a different author.
                  const pill = row.sent_back
                    ? { cls: "lapsed", word: "Rejected" }
                    : row.state === "agreed"
                      ? { cls: "agreed", word: "Decided" }
                      : row.state === "waiting"
                        ? { cls: "waiting", word: "Pending" }
                        : row.state === "lapsed"
                          ? { cls: "lapsed", word: "Lapsed" }
                          : row.state === "undecided"
                            ? { cls: "open", word: "Undecided" }
                            : { cls: "waiting", word: "Partly" };
                  return (
                    <Fragment key={key}>
                      <tr
                        className={`row${triage && i === cursor ? " cursor" : ""}`}
                        data-i={i}
                        onClick={() => {
                          if (!triage) {
                            navigate(at);
                            return;
                          }
                          setCursor(i);
                          setDeciding(deciding === i ? null : i);
                        }}
                      >
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
                          {row.score ? (
                            <span className="hint" style={{ marginLeft: 6 }}>
                              {row.score.toFixed(1)}
                            </span>
                          ) : null}
                        </td>
                        <td>
                          <Link to={at} className="id" onClick={(e) => e.stopPropagation()}>
                            {row.vulnerability}
                          </Link>{" "}
                          <Exploited when={row.exploited} />
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
                          <br />
                          <span className="id" style={{ color: "var(--faint)" }}>
                            {row.version}
                          </span>
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
                              <>
                                <span
                                  className={said.kind === "id" ? "id" : "hint"}
                                  style={
                                    said.kind === "faint" ? { color: "var(--faint)" } : undefined
                                  }
                                >
                                  {said.text}
                                </span>
                                {row.matched === "identifier" && (
                                  <div
                                    className="hint"
                                    title="Matched by comparing a published identifier against an upstream version range. A distribution backports fixes without moving that version, so this may already be fixed here — nobody has confirmed either way."
                                  >
                                    not confirmed
                                  </div>
                                )}
                              </>
                            );
                          })()}
                        </td>
                        <td className="num">
                          {row.places}
                          {(row.answered ?? 0) > 0 && (
                            <span
                              className="hint"
                              title="Argued away by the build's own VEX, which is a different claim by a different author"
                            >
                              {" "}
                              · {row.answered} by the build
                            </span>
                          )}
                        </td>
                        <td>
                          <span className={`state ${pill.cls}`}>{pill.word}</span>
                        </td>
                      </tr>
                      {peeking === key && (
                        <tr className="places">
                          <td colSpan={9}>
                            <Peek
                              at={buildOf(row)}
                              vulnerability={row.vulnerability ?? ""}
                              component={row.component ?? ""}
                              version={row.version ?? ""}
                              to={at}
                            />
                          </td>
                        </tr>
                      )}
                      {triage && deciding === i && (
                        <tr className="decide">
                          <td colSpan={9}>
                            <Inline
                              at={buildOf(row)}
                              row={row}
                              position={{ row: i + 1, of: rows.length }}
                              onDone={(done) => advance(i, done)}
                              onClose={() => setDeciding(null)}
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

          <div className="cards">
            {rows.map((row) => {
              const at = to(buildOf(row), row);
              return (
                <article
                  key={`${row.vulnerability} ${row.component} ${row.version} ${row.ecosystem ?? ""}`}
                  className={`fcard ${row.exploited ? "exploited" : (row.severity ?? "")}`}
                  onClick={() => navigate(at)}
                >
                  <header>
                    <Severity word={row.severity} />
                    <Exploited when={row.exploited} />
                  </header>
                  <div>
                    <span className="id">{row.vulnerability}</span> in{" "}
                    <span className="id">{row.component}</span>
                  </div>
                  <div className="hint">
                    {row.places} {row.places === 1 ? "location" : "locations"} · fixed in{" "}
                    {row.fixed_in ?? "—"}
                  </div>
                </article>
              );
            })}
          </div>
        </div>
      )}

      <div className="filters" style={{ margin: "10px 0 0" }}>
        <span className="hint">
          Showing {rows.length.toLocaleString()} of {total.toLocaleString()}
          {(findings.data?.hidden ?? 0) > 0 && !below && (
            <>
              {" "}
              · {(findings.data?.hidden ?? 0).toLocaleString()} more are below what this product
              triages ({findings.data?.floor}). They are still recorded and still counted.
            </>
          )}
          {below && <> · showing what is below the line as well as above it.</>}
        </span>
        {(findings.data?.hidden ?? 0) > 0 && !below && (
          <button type="button" className="linkish" onClick={() => set("below", "yes")}>
            Include
          </button>
        )}
        {below && (
          <button type="button" className="linkish" onClick={() => set("below", "")}>
            Back to what is triaged
          </button>
        )}
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
      </div>
    </>
  );
}

// The decision form inside a row (UIX-43). It needs the finding's places,
// which the list row does not carry, so they are read when the row opens.
function Inline({
  at,
  row,
  position,
  onDone,
  onClose,
}: {
  at: { product: string; stream: string; variant: string };
  row: Row;
  position: { row: number; of: number };
  onDone: (recorded: Recorded) => void;
  onClose: () => void;
}) {
  const detail = useQuery({
    queryKey: ["finding", at, row.vulnerability, row.component, row.version],
    queryFn: async () =>
      unwrap(
        await api.GET(
          "/v1/products/{product}/streams/{stream}/variants/{variant}/findings/{vulnerability}/components/{component}",
          {
            params: {
              path: {
                ...at,
                vulnerability: row.vulnerability ?? "",
                component: row.component ?? "",
              },
              query: { version: row.version ?? "" },
            },
          },
        ),
      ),
  });
  if (detail.isPending) return <p className="hint">Loading…</p>;
  if (detail.isError) return <Failed error={detail.error} what="This could not be read." />;
  return (
    <div onClick={(event) => event.stopPropagation()}>
      {detail.data?.description && (
        <p className="hint" style={{ margin: "0 0 10px", maxWidth: "78ch" }}>
          {detail.data.description.slice(0, 420)}
        </p>
      )}
      <Decide
        at={{
          ...at,
          vulnerability: row.vulnerability ?? "",
          component: row.component ?? "",
          version: row.version ?? "",
        }}
        places={detail.data?.places ?? []}
        inline
        position={position}
        onDone={onDone}
        onClose={onClose}
      />
    </div>
  );
}

// Where a component sits, as the two ends that differ between sibling rows —
// or, where the selection spans builds, which build the row is being read in.
//
// A chain belongs to one build's graph, so a row covering three builds is
// reached three ways and has no single way down. Naming the build is the
// honest thing to put in the column instead: it is what the row's link and
// its actions are about, and the count says it is one of several.
function Sits({ row }: { row: Row }) {
  if (row.builds) {
    return (
      <span className="chain">
        <span className="hop id">{row.stream}</span>
        <span className="arrow">/</span>
        <span className="hop id">{row.variant}</span>
        {row.builds > 1 && <span className="hint">· one of {row.builds} builds</span>}
      </span>
    );
  }
  if (!row.owner && !row.parent) {
    return <span className="hint">nothing records what pulls this in</span>;
  }
  const same = row.owner === row.parent;
  return (
    <span className="chain">
      <span className="hop id">{row.owner}</span>
      {!same && (
        <>
          <span className="arrow">→</span>
          {row.middle ? (
            <>
              <span
                className="gap"
                title={`${row.middle} step${row.middle > 1 ? "s" : ""} collapsed — open the finding for the full chain`}
              >
                +{row.middle}
              </span>
              <span className="arrow">→</span>
            </>
          ) : null}
          <span className="hop id">{row.parent}</span>
        </>
      )}
      {(row.chains ?? 0) > 1 && <span className="hint">· one of {row.chains}</span>}
    </span>
  );
}

// Where the weight is, rather than what is wrong. Somebody opening a list of
// several thousand rows needs to know that one package is most of it before
// they start reading.
function ByComponent({
  at,
  query,
  offset,
  onHide,
  onOnly,
  onPage,
}: {
  at: { product: string; stream?: string; variant?: string };
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
        await api.GET("/v1/products/{product}/findings/components", {
          params: {
            path: { product: at.product },
            query: {
              ...(query as Record<string, never>),
              ...(at.stream ? { stream: at.stream } : {}),
              ...(at.variant ? { variant: at.variant } : {}),
            },
          },
        }),
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
        Ordered by issue count. <b>Issues</b> is rows in the by-issue view; <b>locations</b> is how
        many places those issues occupy in what you are looking at.
      </p>

      <div className="tablewrap">
        <table>
          <thead>
            <tr>
              <th>Component</th>
              <th className="num">Issues</th>
              <th className="num">Locations</th>
              <th style={{ width: 120 }} />
            </tr>
          </thead>
          <tbody>
            {rows.map((row) => {
              const name = row.component ?? "";
              const share = most > 0 ? Math.round(((row.issues ?? 0) / most) * 100) : 0;
              return (
                <tr key={`${name} ${row.version} ${row.ecosystem ?? ""}`} className="row">
                  <td>
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
                    <span className="id" style={{ color: "var(--faint)" }}>
                      {row.version}
                    </span>
                  </td>
                  <td className="num">
                    {(row.issues ?? 0).toLocaleString()}
                    <span
                      aria-hidden
                      className="share"
                      style={{ width: `${Math.max(share, 2)}%` }}
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

      <div className="filters" style={{ margin: "10px 0 0" }}>
        <span className="hint">
          Showing {rows.length.toLocaleString()} of {total.toLocaleString()} components that carry
          anything open
        </span>
        <Pager offset={offset} total={total} onGo={onPage} />
      </div>
    </>
  );
}

// Opening a row is a look, not a commitment: what the issue actually says and
// where it sits, without leaving a list of a thousand rows.
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
        <p style={{ margin: "8px 0 0", fontSize: "var(--step--1)", maxWidth: "78ch" }}>
          {it.description.slice(0, 420)}
        </p>
      ) : (
        <p className="hint" style={{ margin: 0 }}>
          The report says nothing beyond the identifier.
        </p>
      )}
      <ul className="placelist">
        {(it?.places ?? []).slice(0, 6).map((place) => (
          <li key={place.place}>
            <span className="id">
              {place.consumer
                ? `under ${place.consumer}`
                : place.chain?.[0]?.component
                  ? `under ${place.chain[0].component}`
                  : "nothing records what pulls this in"}
            </span>
            {place.decision != null && <span className="note">decided</span>}
          </li>
        ))}
      </ul>
      <Link to={link} className="linkish">
        Open finding →
      </Link>
    </div>
  );
}

// Where a row opens. The version is part of the address: a component name is
// not unique within a build.
function to(at: { product: string; stream: string; variant: string }, row: Row): string {
  const { product, stream, variant } = at;
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
  const upto = Math.min(offset + PAGE, total);
  return (
    <span style={{ marginLeft: "auto", display: "flex", gap: 6 }}>
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
  );
}
