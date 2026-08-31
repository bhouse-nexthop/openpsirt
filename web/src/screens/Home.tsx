import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import {
  Area,
  AreaChart,
  CartesianGrid,
  Line,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { api } from "../api/client";
import { unwrap } from "../api/queries";
import { Failed } from "../ui/Failed";
import { Severity } from "../ui/Severity";
import type { Who } from "../app/session";

// One home page, assembled from what the person holds. Not a different page
// per role — the panels are the same, and what is in them is what this person
// can actually act on.
//
// These panels deliberately span products, where the findings list does not:
// work falling between people is exactly what hides when every screen is
// scoped to one product and nobody looks at the others.
export function Home({ who }: { who: Who }) {
  return (
    <div className="flex flex-col gap-8">
      <div>
        <h1 className="text-lg font-semibold tracking-tight">
          {greeting()}, {who.name}
        </h1>
        <p className="text-sm text-muted">
          {who.reach.length} {who.reach.length === 1 ? "product" : "products"} you can reach
        </p>
      </div>

      <RunningOut />
      <Trend />
    </div>
  );
}

function greeting(): string {
  const hour = new Date().getHours();
  if (hour < 12) return "Morning";
  if (hour < 18) return "Afternoon";
  return "Evening";
}

// Time passing with nothing said. A dismissal takes a finding off the clock
// and a deferral replaces the deadline with its own date, so what is left here
// is the part worth interrupting somebody about.
function RunningOut() {
  const late = useQuery({
    queryKey: ["home", "running-out"],
    queryFn: async () =>
      unwrap(await api.GET("/v1/running-out", { params: { query: { days: 14, limit: 10 } } })),
  });

  return (
    <section>
      <h2 className="mb-2 text-sm font-semibold">Running out of time</h2>
      {late.isPending && <p className="text-sm text-muted">Loading…</p>}
      {late.isError && <Failed error={late.error} what="What is running out could not be read." />}
      {late.data && (late.data.items ?? []).length === 0 && (
        <p className="text-sm text-muted">
          Nothing is due in the next fortnight that nobody has answered.
        </p>
      )}
      {late.data && (late.data.items ?? []).length > 0 && (
        <ul className="divide-y divide-edge overflow-hidden rounded-lg border border-edge">
          {(late.data.items ?? []).map((row) => (
            <li
              key={`${row.product}-${row.vulnerability}-${row.component}`}
              className="flex flex-wrap items-center gap-2 bg-raised px-3 py-2 text-sm"
            >
              <Link
                to={
                  `/products/${encodeURIComponent(row.product ?? "")}` +
                  `/streams/${encodeURIComponent(row.stream ?? "")}` +
                  `/variants/${encodeURIComponent(row.variant ?? "")}/findings`
                }
                className="font-medium hover:text-accent"
              >
                {row.vulnerability}
              </Link>
              <Severity word={row.severity} exploited={row.exploited} />
              <span className="text-muted">{row.component}</span>
              <span className="ml-auto flex items-center gap-3">
                {row.assigned_to ? (
                  <span className="text-muted">{row.assigned_to}</span>
                ) : (
                  <span className="text-muted">nobody</span>
                )}
                <Due days={row.days_left} />
              </span>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

// Overdue reads differently from due soon, so it says which rather than
// leaving somebody to work out that a negative number means late.
function Due({ days }: { days?: number }) {
  if (typeof days !== "number") return null;
  if (days < 0) {
    return (
      <span className="rounded bg-critical/12 px-1.5 py-0.5 text-xs font-medium text-critical ring-1 ring-inset ring-critical/30">
        {Math.abs(days)}d overdue
      </span>
    );
  }
  return <span className="text-xs text-muted">{days}d left</span>;
}

// Three series rather than one. Separately they are three numbers; together
// they say whether the team is keeping pace, and new consistently outrunning
// resolved is a growing backlog that should be visible without anybody doing
// the arithmetic.
function Trend() {
  const trend = useQuery({
    queryKey: ["home", "trend"],
    queryFn: async () => unwrap(await api.GET("/v1/trend", { params: { query: { weeks: 12 } } })),
  });

  if (trend.isPending) return <p className="text-sm text-muted">Loading…</p>;
  if (trend.isError) {
    return <Failed error={trend.error} what="The trend could not be read." />;
  }

  const points = (trend.data?.items ?? []).map((point) => ({
    at: (point.at ?? "").slice(5, 10),
    open: point.open ?? 0,
    opened: point.opened ?? 0,
    resolved: point.resolved ?? 0,
  }));

  if (points.length === 0) return null;

  return (
    <section>
      <h2 className="mb-2 text-sm font-semibold">Open over time</h2>
      <div className="rounded-lg border border-edge bg-raised p-3">
        <ResponsiveContainer width="100%" height={220}>
          <AreaChart data={points} margin={{ top: 4, right: 4, bottom: 0, left: -20 }}>
            <CartesianGrid strokeOpacity={0.15} vertical={false} />
            <XAxis dataKey="at" tick={{ fontSize: 11 }} tickLine={false} axisLine={false} />
            <YAxis tick={{ fontSize: 11 }} tickLine={false} axisLine={false} width={44} />
            <Tooltip
              contentStyle={{
                background: "var(--color-raised)",
                border: "1px solid var(--color-edge)",
                borderRadius: 8,
                fontSize: 12,
              }}
            />
            <Area
              type="monotone"
              dataKey="open"
              name="open"
              stroke="var(--color-accent)"
              fill="var(--color-accent)"
              fillOpacity={0.12}
            />
            <Line type="monotone" dataKey="opened" name="new" stroke="var(--color-high)" dot={false} />
            <Line
              type="monotone"
              dataKey="resolved"
              name="resolved"
              stroke="var(--color-low)"
              dot={false}
            />
          </AreaChart>
        </ResponsiveContainer>
      </div>
      <p className="mt-2 text-sm text-muted">
        New consistently outrunning resolved is a growing backlog. A bump that carried an issue
        with it is not counted as resolved.
      </p>
    </section>
  );
}
