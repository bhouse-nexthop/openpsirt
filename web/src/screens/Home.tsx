import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { api } from "../api/client";
import { scopeQuery, useScope } from "../app/scope";
import { unwrap } from "../api/queries";
import { Failed } from "../ui/Failed";
import { Pace, Mix, Ring } from "../ui/Charts";
import type { Who } from "../app/session";

// One home page, assembled from what this person holds, and the only screen
// that summarizes across products — work falling between people is exactly
// what hides when every screen is scoped to one product.
//
// The risk this page has to avoid is trying to be everything. What keeps it
// from that is that each panel is a number, three rows, and a way through to
// the screen that actually does the work.
export function Home({ who }: { who: Who }) {
  // Whatever the picker has selected, with every level offering "all"
  // (UIX-38). Home used to summarize across every product and only that,
  // which once a product is chosen answers a question nobody asked.
  const at = useScope();
  const scope = scopeQuery(at);
  const trend = useQuery({
    queryKey: ["home", "trend", scope],
    queryFn: async () =>
      unwrap(await api.GET("/v1/trend", { params: { query: { weeks: 12, ...scope } } })),
  });
  const points = trend.data?.items ?? [];

  return (
    <>
      <div className="screen-head">
        <h2>{greeting()}, {who.name.replace(/^[a-z]+:/, "")}</h2>
        <p>
          {holdings(who)}
          {/* Said rather than implied. A narrowed page that looks like an
              unnarrowed one is how two people quote different figures for the
              same question (REJ-10). */}
          {at.product && (
            <>
              {" · counting "}
              <span className="id">{at.product}</span>
              {at.stream ? (
                <>
                  {" "}
                  <span className="id">{at.stream}</span>
                </>
              ) : (
                " across every branch"
              )}
              {at.variant && (
                <>
                  {" "}
                  <span className="id">{at.variant}</span>
                </>
              )}
            </>
          )}
        </p>
      </div>

      <div className="panels">
        <Waiting />
        <Assigned />
        <Lapsed />

        <div className="panel wide">
          <header>
            <h3>Are we keeping pace?</h3>
            <span className="eyebrow" style={{ marginLeft: "auto" }}>12 weeks · all products</span>
          </header>
          {trend.isError ? (
            <Failed error={trend.error} what="The trend could not be read." />
          ) : (
            <>
              <Pace points={points} />
              <div className="legend">
                <span><i style={{ background: "var(--accent)" }} /> Open</span>
                <span><i style={{ background: "var(--sev-high)" }} /> New</span>
                <span><i style={{ background: "var(--ok)" }} /> Resolved</span>
              </div>
              <p className="reading">{paceReading(points)}</p>
            </>
          )}
        </div>

        <div className="panel wide">
          <header>
            <h3>Severity over time</h3>
            <span className="eyebrow" style={{ marginLeft: "auto" }}>Open, by severity</span>
          </header>
          <Mix points={points} />
          <p className="reading">
            A single line would hide this. A total that barely moves while its critical share
            rises is getting worse rather than staying still.
          </p>
        </div>

        <div className="panel">
          <header>
            <h3>Open findings by severity</h3>
            <span className="eyebrow" style={{ marginLeft: "auto" }}>Open now · all products</span>
          </header>
          <Ring point={points[points.length - 1]} />
          <p className="reading">
            What is open right now, split the way the ranking splits it. Exploited is counted
            with what it is rated, because the ring is about how bad rather than how urgent.
          </p>
        </div>

        <div className="panel">
          <header>
            <h3>Compared to the last release <span className="todo">not built</span></h3>
          </header>
          <p className="reading">
            The pre-release question: is what we are about to ship better or worse than what we
            last shipped? Comparing two builds is built; comparing a branch against the last
            release cut from it is not.
          </p>
        </div>

        <Operational who={who} />
      </div>
    </>
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
  if (who.admin) return "You administer this deployment, so you reach everything in it.";
  const triage = who.reach.filter((each) => each.may_triage).map((each) => each.product);
  const agree = who.reach.filter((each) => each.may_agree).map((each) => each.product);
  const parts: string[] = [];
  if (triage.length) parts.push(`triage on ${triage.join(", ")}`);
  if (agree.length) parts.push(`approval on ${agree.join(", ")}`);
  if (parts.length === 0) {
    return `You can read ${who.reach.length} product${who.reach.length === 1 ? "" : "s"}.`;
  }
  return `You hold ${parts.join(" and ")}.`;
}

function paceReading(points: { open?: number; opened?: number; resolved?: number }[]): string {
  if (points.length < 2) return "Not enough history yet to say which way this is going.";
  const outran = points.filter((p) => (p.opened ?? 0) > (p.resolved ?? 0)).length;
  const first = points[0]?.open ?? 0;
  const last = points[points.length - 1]?.open ?? 0;
  const moved = last - first;
  const direction = moved > 0 ? "growing" : moved < 0 ? "shrinking" : "flat";
  return `Separately these are three numbers. Together they say the backlog is ${direction}: ` +
    `new outran resolved in ${outran} of the last ${points.length} weeks, and open is ` +
    `${moved >= 0 ? "up" : "down"} ${Math.abs(moved)} across the range.`;
}

function Waiting() {
  const queue = useQuery({
    queryKey: ["queue"],
    queryFn: async () => unwrap(await api.GET("/v1/review-queue", { params: { query: { limit: 3 } } })),
  });
  const items = queue.data?.items ?? [];

  return (
    <div className="panel">
      <header>
        <h3>Waiting for you</h3>
        <span className="tally">{queue.data?.total ?? 0}</span>
      </header>
      {queue.isError && <Failed error={queue.error} what="This could not be read." />}
      {items.length === 0 && !queue.isError && (
        <p className="reading">Nothing is waiting on you.</p>
      )}
      <ul>
        {items.map((row) => (
          <li key={row.decision?.id}>
            <span className="id">{row.place?.vulnerability}</span>
            <span className="what">{row.place?.product} · {row.decision?.outcome}</span>
            {typeof row.age_days === "number" && <span className="when">{row.age_days}d</span>}
          </li>
        ))}
      </ul>
      <footer>
        <Link to="/review-queue" className="linkish">Open the review queue →</Link>
      </footer>
    </div>
  );
}

// The mockup's "assigned to you". There is no endpoint that lists what one
// person holds — only how much each person holds — so the count is real and
// the rows are not.
function Assigned() {
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
        <h3>Being worked on <span className="todo">not yours alone</span></h3>
        <span className={overdue > 0 ? "tally urgent" : "tally"}>{total}</span>
      </header>
      {held.isError && <Failed error={held.error} what="This could not be read." />}
      <ul>
        {mine.slice(0, 3).map((each) => (
          <li key={each.person}>
            <span className="id">{each.person}</span>
            <span className="what">{each.open} open</span>
            {(each.overdue ?? 0) > 0 && <span className="when">{each.overdue} late</span>}
          </li>
        ))}
      </ul>
      <p className="reading">
        Nothing lists what one person holds — only how much each person holds — so this is
        everybody rather than you.
      </p>
      <footer>
        <Link to="/unassigned" className="linkish">See what nobody owns →</Link>
      </footer>
    </div>
  );
}

// A decision the code moved out from under, and a deferral whose date has
// passed, are both somebody having to look again — and both disappear silently
// without somewhere that says so.
function Lapsed() {
  const lapsed = useQuery({
    queryKey: ["home", "lapsed"],
    queryFn: async () =>
      unwrap(await api.GET("/v1/decisions", { params: { query: { state: "lapsed", limit: 3 } } })),
  });
  const expired = useQuery({
    queryKey: ["home", "expired"],
    queryFn: async () =>
      unwrap(await api.GET("/v1/decisions", { params: { query: { expired: true, limit: 3 } } })),
  });

  const lapsedTotal = lapsed.data?.total ?? 0;
  const expiredTotal = expired.data?.total ?? 0;

  return (
    <div className="panel">
      <header>
        <h3>Decisions that stopped applying</h3>
        <span className={lapsedTotal + expiredTotal > 0 ? "tally urgent" : "tally"}>
          {lapsedTotal + expiredTotal}
        </span>
      </header>
      {lapsedTotal > 0 && (
        <div className="alert">
          <strong>{lapsedTotal} {lapsedTotal === 1 ? "decision" : "decisions"} lapsed</strong>
          <br />
          <span>The versions they were claims about have moved.</span>
        </div>
      )}
      {expiredTotal > 0 && (
        <div className="alert">
          <strong>{expiredTotal} {expiredTotal === 1 ? "deferral" : "deferrals"} ran out</strong>
          <br />
          <span>The date they were put off until has passed.</span>
        </div>
      )}
      {lapsedTotal + expiredTotal === 0 && (
        <p className="reading">Nothing has stopped applying.</p>
      )}
      <footer>
        <Link to="/review-queue" className="linkish">Review them →</Link>
      </footer>
    </div>
  );
}

// The tool's own health. An operator who has not opted into anything is
// exactly the one who needs telling that a product stopped being scanned.
function Operational({ who }: { who: Who }) {
  const first = who.reach[0]?.product;
  const scans = useQuery({
    queryKey: ["home", "scans", first],
    enabled: !!first,
    queryFn: async () =>
      unwrap(
        await api.GET("/v1/products/{product}/streams/{stream}/variants/{variant}/scans", {
          params: { path: { product: first ?? "", stream: "master", variant: "broadcom" } },
        }),
      ),
  });
  const last = (scans.data?.items ?? [])[0];

  return (
    <div className="panel">
      <header>
        <h3>Operational</h3>
      </header>
      {last?.failure && (
        <div className="alert">
          <strong>The last scan did not finish</strong>
          <br />
          <span>{last.failure}</span>
        </div>
      )}
      <ul>
        <li>
          <span className="what">Last scan of {first ?? "anything"}</span>
          <span className="when">{last?.received_at?.slice(0, 10) ?? "never"}</span>
        </li>
        <li>
          <span className="what">What it reported</span>
          <span className="when">{last?.state ?? "—"}</span>
        </li>
      </ul>
      <p className="reading">
        A product quietly dropping out of scanning is the failure that makes everything else
        wrong, so when it was last seen is on the front page.
      </p>
    </div>
  );
}
