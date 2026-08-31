import { useQuery } from "@tanstack/react-query";
import { useParams, useSearchParams } from "react-router-dom";
import { api } from "../api/client";
import { unwrap } from "../api/queries";
import { Empty } from "../ui/Empty";
import { Failed } from "../ui/Failed";
import { Severity } from "../ui/Severity";

const PAGE = 50;

// One row per issue-and-component, not one per place (UIX-01). A real image
// produced 335,021 individual findings that collapse to 7,906 rows here, so
// the grouping is not a nicety — an ungrouped list is six thousand screens of
// rows differing in a column nobody reads.
//
// Paging state lives in the URL (UIX-11), so a link carries what somebody is
// looking at.
export function Findings() {
  const { product = "", stream = "", variant = "" } = useParams();
  const [params, setParams] = useSearchParams();
  const offset = Number(params.get("offset") ?? 0);

  const findings = useQuery({
    queryKey: ["findings", product, stream, variant, offset],
    queryFn: async () =>
      unwrap(
        await api.GET(
          "/v1/products/{product}/streams/{stream}/variants/{variant}/findings",
          {
            params: {
              path: { product, stream, variant },
              query: { limit: PAGE, offset },
            },
          },
        ),
      ),
  });

  if (findings.isPending) return <p className="text-sm text-muted">Loading…</p>;
  if (findings.isError) {
    return <Failed error={findings.error} what="The findings could not be read." />;
  }

  const rows = findings.data?.items ?? [];
  const total = findings.data?.total ?? 0;

  if (total === 0) {
    return <Empty title="Nothing is open here." detail="Every finding in this build has been answered, or none was found." />;
  }

  return (
    <div>
      <header className="mb-4 flex flex-wrap items-baseline justify-between gap-2">
        <h1 className="text-lg font-semibold tracking-tight">
          {product} <span className="text-muted">/ {stream} / {variant}</span>
        </h1>
        <p className="text-sm text-muted">
          {total.toLocaleString()} open {total === 1 ? "issue" : "issues"}
        </p>
      </header>

      {/* A table on a wide screen, cards on a narrow one — never a table that
          scrolls sideways, which is unusable on a phone (UIX-16). */}
      <ul className="flex flex-col gap-2 md:hidden">
        {rows.map((row) => (
          <li key={`${row.vulnerability}-${row.component}`} className="rounded-lg border border-edge bg-raised p-3">
            <div className="flex items-start justify-between gap-2">
              <span className="font-medium">{row.vulnerability}</span>
              <Severity word={row.severity} exploited={row.exploited} />
            </div>
            <p className="mt-1 text-sm text-muted">
              {row.component} {row.version}
            </p>
            <Places places={row.places} answered={row.answered} />
          </li>
        ))}
      </ul>

      <div className="hidden overflow-hidden rounded-lg border border-edge md:block">
        <table className="w-full text-sm">
          <thead className="bg-sunken text-left text-xs uppercase tracking-wide text-muted">
            <tr>
              <th className="px-3 py-2 font-medium">Issue</th>
              <th className="px-3 py-2 font-medium">Component</th>
              <th className="px-3 py-2 font-medium">Severity</th>
              <th className="px-3 py-2 font-medium">Places</th>
              <th className="px-3 py-2 font-medium">Fixed in</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((row) => (
              <tr key={`${row.vulnerability}-${row.component}`} className="border-t border-edge bg-raised">
                <td className="px-3 py-2 font-medium">{row.vulnerability}</td>
                <td className="px-3 py-2">
                  {row.component}
                  <span className="text-muted"> {row.version}</span>
                  {row.upstream && <span className="text-muted"> (from {row.upstream})</span>}
                </td>
                <td className="px-3 py-2">
                  <Severity word={row.severity} exploited={row.exploited} />
                </td>
                <td className="px-3 py-2">
                  <Places places={row.places} answered={row.answered} />
                </td>
                <td className="px-3 py-2 text-muted">{row.fixed_in || "—"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

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
  );
}

// How many places this row covers, and how many of them the build already
// argued away. The count is part of what is read rather than a detail
// (UIX-34): one judgment covering sixty places is a different act from one
// covering one.
function Places({ places, answered }: { places?: number; answered?: number }) {
  const covered = answered ?? 0;
  return (
    <span className="text-sm text-muted">
      {places} {places === 1 ? "place" : "places"}
      {covered > 0 && ` · ${covered} already answered by the build`}
    </span>
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
  const to = Math.min(offset + PAGE, total);
  return (
    <div className="mt-4 flex items-center justify-between text-sm">
      <p className="text-muted">
        {from.toLocaleString()}–{to.toLocaleString()} of {total.toLocaleString()}
      </p>
      <div className="flex gap-2">
        <button
          type="button"
          disabled={offset === 0}
          onClick={() => onGo(Math.max(0, offset - PAGE))}
          className="rounded border border-edge px-2 py-1 disabled:opacity-40"
        >
          Previous
        </button>
        <button
          type="button"
          disabled={to >= total}
          onClick={() => onGo(offset + PAGE)}
          className="rounded border border-edge px-2 py-1 disabled:opacity-40"
        >
          Next
        </button>
      </div>
    </div>
  );
}
