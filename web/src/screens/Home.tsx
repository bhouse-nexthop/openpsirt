import { useQuery } from "@tanstack/react-query";
import { Link, useNavigate } from "react-router-dom";
import { api } from "../api/client";
import { findingsPath, scopeQuery, useScope } from "../app/scope";
import type { Scoped } from "../app/scope";
import { unwrap } from "../api/queries";
import { Failed } from "../ui/Failed";
import { Pace, Mix, Ring, Releases } from "../ui/Charts";
import { claimOf } from "../api/claims";
import type { Who } from "../app/session";

// The most the server returns of what is overdue. A cap with no total, so a
// full list is a floor on the figure rather than the figure.
const OVERDUE_LIMIT = 200;

// The findings list narrowed to one of the tiles. Joined rather than assumed
// to be the first parameter: the list for anything wider than a build carries
// the branch and the variant in its address already.
function withOnly(path: string, only: string): string {
  return `${path}${path.includes("?") ? "&" : "?"}only=${only}`;
}

// One home page, assembled from what this person holds. Four figures that
// follow the scope (UIX-51), then the work — what is pending, what is in
// progress, what lapsed — then the trends, then the system's own state
// (UIX-42). Somebody opening this most days wants the size of the day before
// its contents.
export function Home({ who }: { who: Who }) {
  const at = useScope();
  const scope = scopeQuery(at);
  const counting = at.product
    ? [at.product, at.stream, at.variant].filter(Boolean).join(" · ")
    : "all products";

  const trend = useQuery({
    queryKey: ["home", "trend", scope],
    queryFn: async () =>
      unwrap(await api.GET("/v1/trend", { params: { query: { weeks: 12, ...scope } } })),
  });
  const points = trend.data?.items ?? [];
  // The other axis (RPT-09). Asked for whenever a product is picked, and drawn
  // instead of the calendar where the product has releases to plot: a tag
  // never moves again, and releases months apart read on a calendar as slow
  // drift rather than the step change they were.
  const releases = useQuery({
    queryKey: ["home", "release-trend", scope],
    enabled: !!at.product,
    queryFn: async () =>
      unwrap(await api.GET("/v1/trend/releases", { params: { query: { limit: 12, ...scope } } })),
  });
  // Two releases is the fewest that is a shape. One bar reads as broken
  // rather than as sparse, so the calendar stays until there are two.
  const byRelease = (releases.data?.items ?? []).length >= 2;

  return (
    <>
      <div className="screen-head">
        <h2>
          {greeting()}, {who.name.replace(/^[a-z]+:/, "")}
        </h2>
        <p>
          {holdings(who)} · counting <span className="id">{counting}</span>
        </p>
      </div>

      <Figures counting={counting} points={points} />

      <div className="panels">
        <Pending />
        <InProgress />
        <Lapsed />

        <div className="panel wide">
          <header>
            <h3>
              {byRelease ? "Open issues in each release" : "Trend: open, new and resolved issues"}
            </h3>
            <span className="eyebrow" style={{ marginLeft: "auto" }}>
              {byRelease ? "release over release" : "12 weeks"} · {counting}
            </span>
          </header>
          {trend.isError ? (
            <Failed error={trend.error} what="The trend could not be read." />
          ) : byRelease ? (
            <>
              <Releases points={releases.data?.items ?? []} />
              <p className="hint">
                Each release as it stands against today&rsquo;s vulnerability data, not as of the
                day it was cut &mdash; which is what re-scanning a shipped release is for. Rates
                are not shown here: how much appeared between two releases is an artifact of how
                far apart they were cut.
              </p>
            </>
          ) : (
            <>
              <Pace points={points} />
              <div className="legend">
                <span>
                  <i style={{ background: "var(--accent)" }} /> Open issues
                </span>
                <span>
                  <i style={{ background: "var(--sev-high)" }} /> New
                </span>
                <span>
                  <i style={{ background: "var(--ok)" }} /> Resolved
                </span>
              </div>
              <p className="reading">{paceReading(points)}</p>
            </>
          )}
        </div>

        <div className="panel wide">
          <header>
            <h3>Severity over time</h3>
            <span className="eyebrow" style={{ marginLeft: "auto" }}>
              Open, by severity
            </span>
          </header>
          <Mix points={points} />
          <p className="reading">{mixReading(points)}</p>
        </div>

        <div className="panel">
          <header>
            <h3>Open issues by severity</h3>
            <span className="eyebrow" style={{ marginLeft: "auto" }}>
              Open now
            </span>
          </header>
          <Ring point={points[points.length - 1]} />
          <p className="reading">Exploited is counted with what it is rated.</p>
        </div>

        <Readiness at={at} />

        <Status />
      </div>
    </>
  );
}

// A branch beside the last release cut from it: is what we are about to ship
// better or worse than what we last shipped (RPT-12).
//
// Drawn only where the question has an answer. It needs a whole build picked,
// because a count across products is not a release; and it needs a branch,
// because a tag is one frozen point and was not cut into anything. Where a
// branch has released nothing that has been scanned, the panel says so rather
// than drawing zeroes — a release that shipped clean and a release nobody
// scanned are not the same answer.
function Readiness({ at }: { at: Scoped }) {
  const whole = !!at.product && !!at.stream && !!at.variant;
  const ready = useQuery({
    queryKey: ["readiness", at],
    enabled: whole,
    queryFn: async () =>
      unwrap(
        await api.GET("/v1/products/{product}/streams/{stream}/variants/{variant}/readiness", {
          params: {
            path: { product: at.product ?? "", stream: at.stream ?? "", variant: at.variant ?? "" },
          },
        }),
      ),
  });

  if (!whole) {
    return (
      <div className="panel">
        <header>
          <h3>Release readiness</h3>
        </header>
        <p className="reading">
          Pick a branch and a variant to compare it against the last release cut from it.
        </p>
      </div>
    );
  }
  if (ready.isPending) return null;
  if (ready.isError) {
    return (
      <div className="panel">
        <header>
          <h3>Release readiness</h3>
        </header>
        <Failed error={ready.error} what="The comparison could not be read." />
      </div>
    );
  }
  // A tag is not compared against itself, and the panel is absent rather than
  // present and explaining itself on every visit.
  if (ready.data?.now?.kind === "tag") return null;

  const now = ready.data?.now;
  const shipped = ready.data?.shipped;
  const floor = ready.data?.floor;

  return (
    <div className="panel">
      <header>
        <h3>Release readiness</h3>
        <span className="eyebrow" style={{ marginLeft: "auto" }}>
          {at.stream} · {at.variant}
        </span>
      </header>
      {shipped ? (
        <>
          <table className="plain">
            <thead>
              <tr>
                <th />
                <th className="num">Now</th>
                <th className="num">{shipped.stream}</th>
                <th className="num">Change</th>
              </tr>
            </thead>
            <tbody>
              {(["critical", "high", "medium", "low"] as const).map((band) => (
                <tr key={band}>
                  <td style={{ textTransform: "capitalize" }}>{band}</td>
                  <td className="num">{now?.[band] ?? 0}</td>
                  <td className="num hint">{shipped[band] ?? 0}</td>
                  <td className="num">
                    <Change from={shipped[band] ?? 0} to={now?.[band] ?? 0} />
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          <p className="reading">
            {reading(now?.critical ?? 0, shipped.critical ?? 0, shipped.stream ?? "")}
            {floor ? ` Counted at ${floor} and above.` : ""}
          </p>
        </>
      ) : (
        <p className="reading">{ready.data?.why || "Nothing to compare against yet."}</p>
      )}
      {at.product && (
        <footer>
          <Link to={`/products/${encodeURIComponent(at.product)}/comparison`} className="linkish">
            Release comparison →
          </Link>
        </footer>
      )}
    </div>
  );
}

// The move, said as a direction rather than as a signed number. Fewer is
// better here, so the colour follows the meaning and not the arithmetic.
function Change({ from, to }: { from: number; to: number }) {
  const by = to - from;
  if (by === 0) return <span className="hint">same</span>;
  return (
    <span style={{ color: by > 0 ? "var(--sev-high)" : "var(--ok)", fontWeight: 600 }}>
      {by > 0 ? "+" : "−"}
      {Math.abs(by)}
    </span>
  );
}

// One sentence somebody can repeat in a meeting.
function reading(now: number, shipped: number, release: string): string {
  if (now === shipped) return `${now} critical now, the same as ${release} shipped with.`;
  if (now > shipped) {
    return `${now} critical now against ${shipped} in ${release} — ${now - shipped} more than last shipped.`;
  }
  return `${now} critical now against ${shipped} in ${release} — ${shipped - now} fewer than last shipped.`;
}

// The four figures. Each names what it counts, because the picker narrows
// them — a tile reading "all products" while a product is picked describes the
// one thing it is not showing (REJ-10).
function Figures({
  counting,
  points,
}: {
  counting: string;
  points: { open?: number; by_severity?: Record<string, number> }[];
}) {
  const at = useScope();
  const scope = scopeQuery(at);
  const navigate = useNavigate();

  // One definition at every scope: the trend's latest point, which counts
  // distinct issues. The findings list counts one row per issue and
  // component, and a tile that switched between the two as the picker moved
  // would quote two figures for one word (REJ-10).
  const exploited = useQuery({
    queryKey: ["home", "exploited", scope],
    // Answered for whatever is selected, like every other figure here
    // (UIX-51). It was a whole build's alone while the list behind it was, and
    // a tile that vanishes when somebody widens the scope reads as a tile that
    // broke rather than one that declines (UIX-53).
    enabled: !!at.product,
    queryFn: async () =>
      unwrap(
        await api.GET("/v1/products/{product}/findings", {
          params: {
            path: { product: at.product ?? "" },
            query: {
              limit: 1,
              exploited: true,
              ...(at.stream ? { stream: at.stream } : {}),
              ...(at.variant ? { variant: at.variant } : {}),
            },
          },
        }),
      ),
  });
  const queue = useQuery({
    queryKey: ["queue", "count"],
    queryFn: async () =>
      unwrap(await api.GET("/v1/review-queue", { params: { query: { limit: 1 } } })),
  });
  const late = useQuery({
    queryKey: ["home", "overdue", scope],
    queryFn: async () =>
      unwrap(
        await api.GET("/v1/running-out", { params: { query: { days: 0, limit: OVERDUE_LIMIT, ...scope } } }),
      ),
  });

  const latest = points[points.length - 1];
  const openCount = latest?.open;
  const overdue = (late.data?.items ?? []).filter((row) => (row.days_left ?? 0) < 0);
  const overdueExploited = overdue.filter((row) => row.exploited).length;

  return (
    <div className="kpis">
      <button
        type="button"
        className="kpi"
        onClick={() => navigate(findingsPath(at))}
      >
        <span className="l">Open issues · {counting}</span>
        <span className="n">{openCount === undefined ? "—" : openCount.toLocaleString()}</span>
        <span className="d">one per vulnerability, by any identifier · findings count each issue per component</span>
      </button>
      {!!at.product && (
        <button
          type="button"
          className={`kpi${(exploited.data?.total ?? 0) > 0 ? " urgent" : ""}`}
          onClick={() => navigate(withOnly(findingsPath(at), "exploited"))}
        >
          <span className="l">
            <i style={{ background: "var(--sev-exploited)" }} /> Known exploited
          </span>
          <span className="n">{(exploited.data?.total ?? 0).toLocaleString()}</span>
          <span className="d">sorted above everything else</span>
        </button>
      )}
      <button type="button" className="kpi" onClick={() => navigate("/review-queue")}>
        <span className="l">
          <i style={{ background: "var(--wait)" }} /> Pending your approval
        </span>
        <span className="n">{(queue.data?.total ?? 0).toLocaleString()}</span>
        <span className="d">across every product you may approve on</span>
      </button>
      <button type="button" className="kpi" onClick={() => navigate("/work?days=0")}>
        <span className="l">
          <i style={{ background: "var(--sev-critical)" }} /> Overdue
        </span>
        {/* The list behind this is capped, so a full one is said to be a
            floor rather than passed off as the count. */}
        <span className="n">
          {overdue.length >= OVERDUE_LIMIT ? `${OVERDUE_LIMIT.toLocaleString()}+` : overdue.length.toLocaleString()}
        </span>
        <span className="d">
          {overdueExploited > 0 ? `${overdueExploited} exploited · ` : ""}undecided, past the deadline
          {overdue.length >= OVERDUE_LIMIT ? " · at least" : ""}
        </span>
      </button>
    </div>
  );
}

function greeting(): string {
  const hour = new Date().getHours();
  if (hour < 12) return "Good morning";
  if (hour < 18) return "Good afternoon";
  return "Good evening";
}

// Said in what the roles let somebody do rather than what they are called.
function holdings(who: Who): string {
  if (who.admin) return "You administer this deployment";
  const triage = who.reach.filter((each) => each.may_triage).map((each) => each.product);
  const agree = who.reach.filter((each) => each.may_agree).map((each) => each.product);
  const parts: string[] = [];
  if (triage.length) parts.push(`triage on ${triage.join(", ")}`);
  if (agree.length) parts.push(`approval on ${agree.join(", ")}`);
  if (parts.length === 0) {
    return `You can read ${who.reach.length} product${who.reach.length === 1 ? "" : "s"}`;
  }
  return `You hold ${parts.join(" and ")}`;
}

function paceReading(points: { open?: number; opened?: number; resolved?: number }[]): string {
  if (points.length < 2) return "Not enough history yet to say which way this is going.";
  const outran = points.filter((p) => (p.opened ?? 0) > (p.resolved ?? 0)).length;
  const first = points[0]?.open ?? 0;
  const last = points[points.length - 1]?.open ?? 0;
  const moved = last - first;
  const direction = moved > 0 ? "growing" : moved < 0 ? "shrinking" : "flat";
  return `Backlog ${direction}: new exceeded resolved in ${outran} of ${points.length} weeks; open ${
    moved >= 0 ? "up" : "down"
  } ${Math.abs(moved).toLocaleString()} across the range.`;
}

function mixReading(points: { by_severity?: Record<string, number> }[]): string {
  if (points.length < 2) return "Not enough history yet.";
  const first = points[0]?.by_severity?.critical ?? 0;
  const last = points[points.length - 1]?.by_severity?.critical ?? 0;
  if (first === last) return `Critical unchanged at ${last.toLocaleString()} across the range.`;
  return `Critical went ${first.toLocaleString()} → ${last.toLocaleString()} across the range.`;
}

function Pending() {
  const queue = useQuery({
    queryKey: ["queue", "home"],
    queryFn: async () =>
      unwrap(await api.GET("/v1/review-queue", { params: { query: { limit: 3 } } })),
  });
  const items = queue.data?.items ?? [];

  return (
    <div className="panel">
      <header>
        <h3>Pending your approval</h3>
        {/* The queue is not narrowed by product, so this panel does not
            follow the scope the head names, and says so rather than
            letting the head speak for it. */}
        <span className="eyebrow" style={{ marginLeft: "auto" }}>
          every product you may approve on
        </span>
        <span className="tally">{queue.data?.total ?? 0}</span>
      </header>
      {queue.isError && <Failed error={queue.error} what="This could not be read." />}
      {items.length === 0 && !queue.isError && <p className="reading">Nothing is pending.</p>}
      <ul>
        {items.map((row) => {
          const claim = claimOf(row);
          return (
            <li key={claim.key}>
              <span className="id">{claim.title}</span>
              <span className="what">
                {claim.product} · {claim.outcome}
                {claim.records > 1 ? ` · ${claim.records.toLocaleString()} records` : ""}
              </span>
              {typeof row.age_days === "number" && <span className="when">{row.age_days}d</span>}
            </li>
          );
        })}
      </ul>
      <footer>
        <Link to="/review-queue" className="linkish">
          Open the review queue →
        </Link>
      </footer>
    </div>
  );
}

// What each person holds. Nothing lists what one person holds — only how much
// each person holds — so this is everybody rather than you.
function InProgress() {
  const held = useQuery({
    queryKey: ["home", "holdings"],
    queryFn: async () => unwrap(await api.GET("/v1/assignments", {})),
  });
  const mine = held.data?.items ?? [];
  const total = mine.reduce((sum, each) => sum + (each.open ?? 0), 0);
  const overdue = mine.reduce((sum, each) => sum + (each.overdue ?? 0), 0);

  return (
    <div className="panel">
      <header>
        <h3>In progress</h3>
        {/* Holdings are counted per person across everything, not per
            product, so this does not follow the scope the head names. */}
        <span className="eyebrow" style={{ marginLeft: "auto" }}>
          every product
        </span>
        <span className={overdue > 0 ? "tally urgent" : "tally"}>{total.toLocaleString()}</span>
      </header>
      {held.isError && <Failed error={held.error} what="This could not be read." />}
      {mine.length === 0 && !held.isError && <p className="reading">Nothing is assigned.</p>}
      <ul>
        {mine.slice(0, 3).map((each) => (
          <li key={each.person}>
            <span className="id">{each.person}</span>
            <span className="what">{each.open} open</span>
            {(each.overdue ?? 0) > 0 && <span className="when">{each.overdue} overdue</span>}
          </li>
        ))}
      </ul>
      <footer>
        <Link to="/work" className="linkish">
          View assignments →
        </Link>
      </footer>
    </div>
  );
}

// A decision the code moved out from under, and a deferral whose date has
// passed, are both somebody having to look again — and both disappear silently
// without somewhere that says so.
function Lapsed() {
  // Decisions are listed by product and no finer, so this follows the
  // product of the scope the head names and says when it is not the whole
  // of it.
  const at = useScope();
  const product = at.product ? { product: at.product } : {};
  const lapsed = useQuery({
    queryKey: ["home", "lapsed", product],
    queryFn: async () =>
      unwrap(await api.GET("/v1/decisions", { params: { query: { state: "lapsed", limit: 3, ...product } } })),
  });
  const expired = useQuery({
    queryKey: ["home", "expired", product],
    queryFn: async () =>
      unwrap(await api.GET("/v1/decisions", { params: { query: { expired: true, limit: 3, ...product } } })),
  });

  const lapsedTotal = lapsed.data?.total ?? 0;
  const expiredTotal = expired.data?.total ?? 0;

  return (
    <div className="panel">
      <header>
        <h3>Lapsed decisions</h3>
        <span className="eyebrow" style={{ marginLeft: "auto" }}>
          {at.product ? `${at.product}, every branch` : "every product"}
        </span>
        <span className={lapsedTotal + expiredTotal > 0 ? "tally urgent" : "tally"}>
          {(lapsedTotal + expiredTotal).toLocaleString()}
        </span>
      </header>
      {lapsedTotal > 0 && (
        <div className="alert">
          <strong>
            {lapsedTotal.toLocaleString()} {lapsedTotal === 1 ? "decision" : "decisions"} lapsed
          </strong>
          <span>The versions they were claims about have moved.</span>
        </div>
      )}
      {expiredTotal > 0 && (
        <div className="alert">
          <strong>
            {expiredTotal.toLocaleString()} {expiredTotal === 1 ? "deferral" : "deferrals"} ran out
          </strong>
          <span>The date they were put off until has passed.</span>
        </div>
      )}
      {lapsedTotal + expiredTotal === 0 && <p className="reading">Nothing has lapsed.</p>}
      <footer>
        <Link to="/review-queue#lapsed" className="linkish">
          View →
        </Link>
      </footer>
    </div>
  );
}

// The tool's own health. A build that stops being scanned reports no new
// findings and fails nothing, so it looks healthier than one still being
// scanned; the quiet ones are named, because a name is acted on.
function Status() {
  const at = useScope();
  const scope = scopeQuery(at);
  const scanning = useQuery({
    queryKey: ["home", "scanning", scope],
    queryFn: async () => unwrap(await api.GET("/v1/scanning", { params: { query: scope } })),
  });
  const builds = scanning.data?.items ?? [];
  const quiet = builds.filter((b) => b.quiet);
  const last = builds.find((b) => b.last_received_at);
  const whole = !!(at.product && at.stream && at.variant);

  return (
    <div className="panel">
      <header>
        <h3>System status</h3>
      </header>
      {scanning.isError && (
        <Failed error={scanning.error} what="What has been scanned could not be read." />
      )}
      {quiet.slice(0, 3).map((build) => (
        <div className="alert" key={`${build.product} ${build.stream} ${build.variant}`}>
          <strong>
            {build.product} · {build.stream} · {build.variant}: no inventory
            {build.last_received_at ? ` for ${build.quiet_days} days` : " ever"}
          </strong>
          <span>
            {build.last_received_at
              ? "Nothing has failed — nothing has arrived."
              : `Declared ${build.quiet_days} days ago.`}
          </span>
        </div>
      ))}
      {quiet.length > 3 && <p className="hint">and {quiet.length - 3} more.</p>}
      <ul>
        <li>
          <span className="what">Builds being scanned</span>
          <span className="when">
            {builds.length - quiet.length} of {builds.length}
          </span>
        </li>
        <li>
          <span className="what">Last inventory received</span>
          <span className="when">{last?.last_received_at?.slice(0, 10) ?? "never"}</span>
        </li>
        <li>
          <span className="what">Quiet after</span>
          <span className="when">{scanning.data?.quiet_after_days ?? 7} days</span>
        </li>
      </ul>
      {whole && (
        <footer>
          <Link
            to={`/products/${encodeURIComponent(at.product ?? "")}/streams/${encodeURIComponent(at.stream ?? "")}/variants/${encodeURIComponent(at.variant ?? "")}/scans`}
            className="linkish"
          >
            View inventories →
          </Link>
        </footer>
      )}
    </div>
  );
}
