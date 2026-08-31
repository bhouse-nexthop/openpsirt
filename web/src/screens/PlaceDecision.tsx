import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useParams } from "react-router-dom";
import { api } from "../api/client";
import { unwrap } from "../api/queries";
import { Failed } from "../ui/Failed";
import { Crumbs } from "../ui/Crumbs";
import { Outcome, JUSTIFICATIONS } from "../ui/Outcome";
import { State } from "../ui/State";
import { Markdown } from "../ui/Markdown";
import { Editor } from "../ui/Editor";

// What was decided at one place, and how to decide it. A place is the
// component and what directly pulled it in, which is the unit a decision is
// keyed on — not the finding, and not the release.
export function PlaceDecision() {
  const { product = "", stream = "", variant = "", vulnerability = "", place = "" } = useParams();
  const at = { product, stream, variant, vulnerability, place };
  const queries = useQueryClient();

  const decided = useQuery({
    queryKey: ["decided", at],
    queryFn: async () =>
      unwrap(
        await api.GET(
          "/v1/products/{product}/streams/{stream}/variants/{variant}/findings/{vulnerability}/places/{place}/decision",
          { params: { path: at } },
        ),
      ),
  });

  const decide = useMutation({
    mutationFn: async (body: {
      outcome: string;
      justification?: string;
      deferred_until?: string;
      reasoning: string;
    }) =>
      unwrap(
        await api.POST(
          "/v1/products/{product}/streams/{stream}/variants/{variant}/findings/{vulnerability}/places/{place}/decision",
          // The generated types describe the body; the outcome is one of the
          // four the server names, and TypeScript holds us to that.
          { params: { path: at }, body: body as never },
        ),
      ),
    onSuccess: () => {
      void queries.invalidateQueries({ queryKey: ["decided"] });
      void queries.invalidateQueries({ queryKey: ["queue"] });
    },
  });

  if (decided.isPending) return <p className="text-sm text-muted">Loading…</p>;
  if (decided.isError) {
    return <Failed error={decided.error} what="What was decided here could not be read." />;
  }

  const standing = decided.data?.standing;
  const previously = decided.data?.previously ?? [];
  const back =
    `/products/${encodeURIComponent(product)}` +
    `/streams/${encodeURIComponent(stream)}` +
    `/variants/${encodeURIComponent(variant)}/findings`;

  return (
    <div className="max-w-3xl">
      <Crumbs product={product} stream={stream} variant={variant} />
      <Link to={back} className="mb-3 inline-block text-sm text-muted hover:text-ink">
        ← All findings
      </Link>
      <h1 className="mb-4 text-xl font-semibold tracking-tight">{vulnerability}</h1>

      {standing ? (
        <Standing detail={standing} />
      ) : (
        <>
          <p className="mb-4 text-sm text-muted">
            Nothing is in force here. A claim waiting for a second person suppresses nothing,
            so it does not appear as one.
          </p>
          <Decide
            onDecide={(body) => decide.mutate(body)}
            pending={decide.isPending}
            error={decide.error}
            draftKey={`decide:${product}:${vulnerability}:${place}`}
          />
        </>
      )}

      {previously.length > 0 && <Previously items={previously} />}
    </div>
  );
}

type Detail = {
  decision?: { id?: number; outcome?: string; state?: string; needs_approval?: boolean; deferred_until?: string };
  reasoning?: string;
  proposed_by?: string;
  proposed_at?: string;
  age_days?: number;
};

function Standing({ detail }: { detail: Detail }) {
  return (
    <section className="mb-6 rounded-lg border border-edge bg-raised p-4">
      <div className="mb-2 flex flex-wrap items-center gap-2">
        <Outcome outcome={detail.decision?.outcome} />
        <State state={detail.decision?.state} />
        {detail.decision?.deferred_until && (
          <span className="text-sm text-muted">until {detail.decision.deferred_until}</span>
        )}
      </div>
      <p className="mb-3 text-sm text-muted">
        {detail.proposed_by && <>claimed by {detail.proposed_by}</>}
        {detail.proposed_at && <> on {detail.proposed_at.slice(0, 10)}</>}
      </p>
      {detail.reasoning && <Markdown source={detail.reasoning} />}
    </section>
  );
}

// Read what was decided here before deciding again. A claim that lapsed on a
// version bump is usually still the right answer, and re-affirming it is a
// different request from making a new one.
function Previously({ items }: { items: Detail[] }) {
  return (
    <section className="mt-8">
      <h2 className="mb-2 text-sm font-semibold">Decided here before</h2>
      <p className="mb-3 text-sm text-muted">
        Withdrawn claims, and claims that stopped applying when a version moved. Worth reading
        before deciding again.
      </p>
      <ul className="flex flex-col gap-2">
        {items.map((each, index) => (
          <li key={each.decision?.id ?? index} className="rounded-lg border border-edge bg-sunken p-3">
            <div className="mb-1 flex flex-wrap items-center gap-2">
              <Outcome outcome={each.decision?.outcome} />
              <State state={each.decision?.state} />
              <span className="text-sm text-muted">{each.proposed_by}</span>
            </div>
            {each.reasoning && (
              <div className="text-sm">
                <Markdown source={each.reasoning} />
              </div>
            )}
          </li>
        ))}
      </ul>
    </section>
  );
}

function Decide({
  onDecide,
  pending,
  error,
  draftKey,
}: {
  onDecide: (body: { outcome: string; justification?: string; deferred_until?: string; reasoning: string }) => void;
  pending: boolean;
  error: unknown;
  draftKey: string;
}) {
  const [outcome, setOutcome] = useState("not-applicable");
  const [justification, setJustification] = useState(JUSTIFICATIONS[0]?.value ?? "");
  const [until, setUntil] = useState("");
  const [reasoning, setReasoning] = useState("");

  const needsJustification = outcome === "not-applicable";
  const needsDate = outcome === "deferred";
  const ready =
    reasoning.trim() !== "" && (!needsJustification || justification) && (!needsDate || until);

  return (
    <section>
      <h2 className="mb-2 text-sm font-semibold">Decide</h2>
      <div className="flex flex-col gap-3">
        <label className="text-sm">
          <span className="mb-1 block text-muted">What is true here</span>
          <select
            value={outcome}
            onChange={(event) => setOutcome(event.target.value)}
            className="w-full rounded border border-edge bg-raised px-2 py-1.5"
          >
            <option value="not-applicable">It does not apply to us</option>
            <option value="affected">It applies and needs fixing</option>
            <option value="deferred">It applies, but not until a date</option>
            <option value="wont-fix">It applies and will not be fixed</option>
          </select>
        </label>

        {/* The claim that something does not affect us *is* which of the
            recognized reasons applies, so it is not a note beside the outcome
            — it is the outcome. */}
        {needsJustification && (
          <label className="text-sm">
            <span className="mb-1 block text-muted">Which reason</span>
            <select
              value={justification}
              onChange={(event) => setJustification(event.target.value)}
              className="w-full rounded border border-edge bg-raised px-2 py-1.5"
            >
              {JUSTIFICATIONS.map((each) => (
                <option key={each.value} value={each.value}>
                  {each.label}
                </option>
              ))}
            </select>
          </label>
        )}

        {needsDate && (
          <label className="text-sm">
            <span className="mb-1 block text-muted">Until when</span>
            <input
              type="date"
              value={until}
              onChange={(event) => setUntil(event.target.value)}
              className="rounded border border-edge bg-raised px-2 py-1.5"
            />
            <span className="mt-1 block text-muted">
              A deferral with no date is a decision never to look again.
            </span>
          </label>
        )}

        <div>
          <span className="mb-1 block text-sm text-muted">
            Why. Somebody else has to agree with this.
          </span>
          <Editor
            value={reasoning}
            onChange={setReasoning}
            draftKey={draftKey}
            label="Reasoning"
            placeholder="What makes this true for this component, at this place?"
          />
        </div>

        {error != null && <Failed error={error} what="That could not be recorded." />}

        <div>
          <button
            type="button"
            disabled={!ready || pending}
            onClick={() =>
              onDecide({
                outcome,
                ...(needsJustification ? { justification } : {}),
                ...(needsDate ? { deferred_until: until } : {}),
                reasoning,
              })
            }
            className="rounded bg-accent px-3 py-1.5 text-sm font-medium text-accent-ink disabled:opacity-50"
          >
            Record it
          </button>
          <span className="ml-3 text-sm text-muted">
            Most outcomes wait for a second person before they take effect.
          </span>
        </div>
      </div>
    </section>
  );
}
