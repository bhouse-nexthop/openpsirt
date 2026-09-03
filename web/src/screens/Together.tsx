import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useParams, useSearchParams } from "react-router-dom";
import { api } from "../api/client";
import { unwrap } from "../api/queries";
import { Empty } from "../ui/Empty";
import { Failed } from "../ui/Failed";
import { Crumbs } from "../ui/Crumbs";
import { Severity } from "../ui/Severity";
import { JUSTIFICATIONS } from "../ui/Outcome";
import { Editor, forget } from "../ui/Editor";

// One judgment about many issues at one component. The transpose of the usual
// grouping: one issue across many places is what a decision already covers,
// and a kernel carrying thousands of issues — most of them in drivers a given
// image never builds — has no answer at all without this.
//
// The claim is ordinary and needs a second person like any other dismissal.
// What makes it defensible is that it writes a separate decision per issue and
// per place, each keyed and expiring on its own, rather than one blanket claim.
export function Together() {
  const { product = "", stream = "", variant = "", component = "" } = useParams();
  const [params, setParams] = useSearchParams();
  const contains = params.get("contains") ?? "";
  const [typed, setTyped] = useState(contains);
  const [picked, setPicked] = useState<Set<string>>(new Set());
  const queries = useQueryClient();

  const at = { product, stream, variant, component };

  const issues = useQuery({
    queryKey: ["at-component", at, contains],
    queryFn: async () =>
      unwrap(
        await api.GET(
          "/v1/products/{product}/streams/{stream}/variants/{variant}/components/{component}/issues",
          { params: { path: at, query: { limit: 500, ...(contains ? { contains } : {}) } } },
        ),
      ),
  });

  const decide = useMutation({
    mutationFn: async (body: {
      vulnerabilities: string[];
      outcome: string;
      justification?: string;
      selected_by: string;
      reasoning: string;
    }) =>
      unwrap(
        await api.POST(
          "/v1/products/{product}/streams/{stream}/variants/{variant}/components/{component}/decisions",
          { params: { path: at }, body: body as never },
        ),
      ),
    onSuccess: () => {
      // Cleared only once the server has taken it. A refused submission keeps
      // every word, which is the whole point of keeping a draft at all.
      forget(`together:${product}:${component}`);
      setPicked(new Set());
      void queries.invalidateQueries({ queryKey: ["at-component"] });
      void queries.invalidateQueries({ queryKey: ["queue"] });
    },
  });

  const items = issues.data?.items ?? [];

  return (
    <div className="max-w-4xl">
      <Crumbs product={product} stream={stream} variant={variant} />
      <div className="screen-head">
        <h2>Bulk decision</h2>
        <p>
          <span className="id">{component}</span> · {product} · {stream} · {variant} — one outcome, one
          reasoning, a separate record per issue and per location. Nothing here selects for you.
        </p>
      </div>

      <form
        className="mb-4 flex flex-wrap gap-2"
        onSubmit={(event) => {
          event.preventDefault();
          setParams(typed ? { contains: typed } : {});
        }}
      >
        <input
          value={typed}
          onChange={(event) => setTyped(event.target.value)}
          placeholder="Narrow by what the report says — driver, ioctl…"
          aria-label="Narrow the list"
          className="min-w-56 flex-1 rounded border border-[var(--line)] bg-[var(--surface)] px-2 py-1.5 text-sm"
        />
        <button type="submit" className="rounded border border-[var(--line)] px-3 py-1.5 text-sm">
          Narrow
        </button>
        <span className="hint" style={{ alignSelf: "center" }}>
          Text matching on the description — a way to find candidates, not a judgment about them
        </span>
      </form>

      {issues.isPending && <p className="text-sm text-[var(--muted)]">Loading…</p>}
      {issues.isError && <Failed error={issues.error} what="The issues could not be read." />}

      {issues.data && items.length === 0 && (
        <Empty title="Nothing matches." detail="Nothing is open against this component under that narrowing." />
      )}

      {items.length > 0 && (
        <>
          <div className="mb-2 flex flex-wrap items-center gap-3 text-sm">
            <button
              type="button"
              onClick={() => setPicked(new Set(items.map((i) => i.vulnerability ?? "")))}
              className="text-[var(--accent)] hover:underline"
            >
              Select all {items.length}
            </button>
            <button type="button" onClick={() => setPicked(new Set())} className="text-[var(--muted)] hover:text-[var(--ink)]">
              Clear
            </button>
            <span className="ml-auto text-[var(--muted)]">
              {picked.size} of {issues.data?.total ?? items.length} selected
            </span>
          </div>

          <ul className="mb-5 max-h-96 divide-y divide-edge overflow-y-auto rounded-lg border border-[var(--line)]">
            {items.map((issue) => {
              const name = issue.vulnerability ?? "";
              return (
                <li key={name} className="flex flex-wrap items-center gap-2 bg-[var(--surface)] px-3 py-2 text-sm">
                  <label className="flex items-center gap-2">
                    <input
                      type="checkbox"
                      checked={picked.has(name)}
                      onChange={(event) => {
                        const next = new Set(picked);
                        if (event.target.checked) next.add(name);
                        else next.delete(name);
                        setPicked(next);
                      }}
                    />
                    <span className="font-medium">{name}</span>
                  </label>
                  <Severity word={issue.severity} />
                  <span className="ml-auto flex gap-3 text-[var(--muted)]">
                    <span>
                      {issue.places} {issue.places === 1 ? "location" : "locations"}
                    </span>
                    {issue.fixed_in && <span>fixed in {issue.fixed_in}</span>}
                  </span>
                </li>
              );
            })}
          </ul>

          <Claim
            narrowed={contains}
            count={picked.size}
            onClaim={(body) => decide.mutate({ ...body, vulnerabilities: [...picked] })}
            pending={decide.isPending}
            error={decide.error}
            recorded={decide.data?.recorded}
            draftKey={`together:${product}:${component}`}
            mentions={{ product }}
          />
        </>
      )}
    </div>
  );
}

function Claim({
  narrowed,
  count,
  onClaim,
  pending,
  error,
  recorded,
  draftKey,
  mentions,
}: {
  narrowed: string;
  count: number;
  onClaim: (body: {
    outcome: string;
    justification?: string;
    selected_by: string;
    reasoning: string;
  }) => void;
  pending: boolean;
  error: unknown;
  recorded?: number;
  draftKey: string;
  mentions: { product: string };
}) {
  const [outcome, setOutcome] = useState("not-applicable");
  const [justification, setJustification] = useState(JUSTIFICATIONS[0]?.value ?? "");
  // Prefilled from the narrowing that is on, in the form the approver's
  // outlier check reads back — `contains "driver"` — and still editable, since
  // how a set was chosen may be more than one term.
  const [selectedBy, setSelectedBy] = useState(narrowed ? `contains "${narrowed}"` : "");
  const [reasoning, setReasoning] = useState("");

  const needsJustification = outcome === "not-applicable";
  const ready = count > 0 && reasoning.trim() !== "" && selectedBy.trim() !== "";

  return (
    <section className="rounded-lg border border-[var(--line)] bg-[var(--surface)] p-4">
      <h2 className="mb-3 text-sm font-semibold">Decision for {count} {count === 1 ? "issue" : "issues"}</h2>

      <div className="flex flex-col gap-3">
        <label className="text-sm">
          <span className="mb-1 block text-[var(--muted)]">Outcome</span>
          <select
            value={outcome}
            onChange={(event) => setOutcome(event.target.value)}
            className="w-full rounded border border-[var(--line)] bg-[var(--surface)] px-2 py-1.5"
          >
            <option value="not-applicable">Not applicable</option>
            <option value="wont-fix">Won't fix</option>
            <option value="affected">Affected</option>
          </select>
        </label>

        {needsJustification && (
          <label className="text-sm">
            <span className="mb-1 block text-[var(--muted)]">Justification</span>
            <select
              value={justification}
              onChange={(event) => setJustification(event.target.value)}
              className="w-full rounded border border-[var(--line)] bg-[var(--surface)] px-2 py-1.5"
            >
              {JUSTIFICATIONS.map((each) => (
                <option key={each.value} value={each.value}>
                  {each.label}
                </option>
              ))}
            </select>
          </label>
        )}

        {/* Recorded with every claim, separately from the reasoning. How a
            candidate was found is not why the claim is true — "these matched a
            word" is not a defence anybody would accept — but "how were these
            chosen" is the question asked of a bulk judgment months later. */}
        <label className="text-sm">
          <span className="mb-1 block text-[var(--muted)]">Narrowed by</span>
          <input
            value={selectedBy}
            onChange={(event) => setSelectedBy(event.target.value)}
            placeholder="e.g. searched the reports for the drivers this image does not build"
            className="w-full rounded border border-[var(--line)] bg-[var(--surface)] px-2 py-1.5"
          />
        </label>

        <div>
          <span className="mb-1 block text-sm text-[var(--muted)]">Reasoning</span>
          <Editor
            value={reasoning}
            onChange={setReasoning}
            draftKey={draftKey}
            label="Reasoning"
            mentions={mentions}
            placeholder="What makes this true of all of them? A search term is not a reason."
          />
        </div>

        {error != null && <Failed error={error} what="That could not be recorded." />}
        {typeof recorded === "number" && recorded > 0 && (
          <p className="rounded border border-[var(--ok)] bg-[var(--ok-bg)] px-3 py-2 text-sm">
            {recorded} records written — one per issue, per location. One claim, pending a second
            person; each record expires on its own.
          </p>
        )}

        <div>
          <button
            type="button"
            disabled={!ready || pending}
            onClick={() => {
              onClaim({
                outcome,
                ...(needsJustification ? { justification } : {}),
                selected_by: selectedBy,
                reasoning,
              })
            }}
            className="rounded bg-[var(--accent)] px-3 py-1.5 text-sm font-medium text-white disabled:opacity-50"
          >
            Submit for {count}
          </button>
          <span className="ml-3 text-sm text-[var(--muted)]">
            Always needs a second person, whatever the outcome.
          </span>
        </div>
      </div>
    </section>
  );
}
