import { useEffect, useMemo, useState } from "react";
import { useMutation, useQueries, useQueryClient } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { api } from "../api/client";
import { unwrap } from "../api/queries";
import { Editor, forget } from "./Editor";
import { Failed } from "./Failed";
import { JUSTIFICATIONS, type Justification } from "./Outcome";
import { Covering, type Sitting } from "./Covering";
import { Review, type Other, type Plan } from "./Review";

// One judgment about this finding: the decision form the finding screen
// carries and the findings list opens in place (UIX-43). Outcome, the
// justification where it does not apply, a date where it is deferred, the
// reasoning, which locations it covers (UIX-44), and — on submit — a guided
// review of where it applies beyond this build (UIX-45).

export type At = {
  product: string;
  stream: string;
  variant: string;
  vulnerability: string;
  component: string;
  version: string;
};

export type Recorded = {
  claimId: number;
  recorded: number;
  covered: number;
  left: number;
  needsApproval: boolean;
  applied: { build: string; ok: boolean; said?: string }[];
  matching: number;
};

const OUTCOMES = [
  { value: "affected", label: "Affected" },
  { value: "not-applicable", label: "Not applicable" },
  { value: "deferred", label: "Deferred" },
  { value: "wont-fix", label: "Won't fix" },
  { value: "already-fixed", label: "Already fixed" },
] as const;

// How many covered places the reach is merged from. One request per place,
// and a kernel sits at sixty; the builds that match one place of a finding
// almost always match the rest, so a bounded sample answers the question.
const REACH_SAMPLE = 8;

export function Decide({
  at,
  places,
  inline,
  position,
  onDone,
  onClose,
  extending,
  prefill,
}: {
  at: At;
  places: Sitting[];
  // Opened inside the findings list, where the keys are live and there is a
  // next row to go to.
  inline?: boolean;
  position?: { row: number; of: number };
  onDone: (recorded: Recorded) => void;
  onClose?: () => void;
  // An approved claim this extends (TRI-47), where the backend offers one.
  extending?: { claimId: number; decisionId: number } | null;
  prefill?: { outcome?: string; justification?: string; reasoning?: string } | null;
}) {
  const queries = useQueryClient();
  const draftKey = `decide:${at.product}:${at.stream}:${at.variant}:${at.vulnerability}:${at.component}`;
  const [outcome, setOutcome] = useState(prefill?.outcome ?? "not-applicable");
  const [fixedVersion, setFixedVersion] = useState("");
  const [justification, setJustification] = useState<Justification>(
    (prefill?.justification as Justification | undefined) ?? "vulnerable_code_not_in_execute_path",
  );
  const [mitigation, setMitigation] = useState("");
  const [until, setUntil] = useState("");
  const [reasoning, setReasoning] = useState(prefill?.reasoning ?? "");
  const [excluded, setExcluded] = useState<Set<string>>(() => new Set());
  const [reviewing, setReviewing] = useState(false);
  const [refused, setRefused] = useState<string | null>(null);

  useEffect(() => {
    if (prefill?.reasoning) setReasoning(prefill.reasoning);
    if (prefill?.outcome) setOutcome(prefill.outcome);
    if (prefill?.justification) setJustification(prefill.justification as Justification);
  }, [prefill]);

  const open = useMemo(() => places.filter((p) => p.decision == null), [places]);
  const answered = places.length - open.length;
  const covering = open.filter((p) => !excluded.has(p.place ?? ""));
  const needsJustification = outcome === "not-applicable";
  const needsDate = outcome === "deferred";
  // A claim that the fix is already here is a fact somebody can check against
  // whoever packages the component, and it is required for that reason
  // (TRI-51). The server refuses it empty; asking here means the person finds
  // out while they are still looking at the tracker.
  const needsFixedVersion = outcome === "already-fixed";
  const needsMitigation = needsJustification && justification === "inline_mitigations_already_exist";

  // Where a judgment here lands beyond this build, merged across a sample of
  // the covered places. Builds already matching are named; builds at other
  // versions are what the review walks.
  const sample = covering.slice(0, REACH_SAMPLE);
  const reach = useQueries({
    queries: sample.map((place) => ({
      queryKey: ["reach", at.product, at.stream, at.variant, at.vulnerability, place.place],
      queryFn: async () =>
        unwrap(
          await api.GET(
            "/v1/products/{product}/streams/{stream}/variants/{variant}/findings/{vulnerability}/places/{place}/reach",
            {
              params: {
                path: {
                  product: at.product,
                  stream: at.stream,
                  variant: at.variant,
                  vulnerability: at.vulnerability,
                  place: place.place ?? "",
                },
              },
            },
          ),
        ),
      staleTime: 60_000,
    })),
  });
  const { matching, offered } = useMemo(() => {
    const auto = new Map<string, true>();
    const diff = new Map<string, Other>();
    const differing = new Set<string>();
    for (const each of reach) {
      for (const m of each.data?.automatic ?? []) auto.set(`${m.stream} · ${m.variant}`, true);
      for (const m of each.data?.differing ?? []) {
        // Where it is, said as an aside. What is being asked about is the
        // version — a build at matching versions never reaches this list — so
        // "this build" is the honest label for another version sitting beside
        // the one in hand, rather than the build's own name repeated back.
        const build = m.here ? "this build" : `${m.stream} · ${m.variant}`;
        // One question per version, not one per build: a build carrying the
        // component at four versions is four claims about different code, and
        // each is posted with its own version.
        const key = `${build} @ ${m.version ?? ""}`;
        const had = diff.get(key);
        differing.add(build);
        diff.set(key, {
          key,
          build,
          stream: m.stream ?? "",
          variant: m.variant ?? "",
          version: m.version ?? "",
          places: (had?.places ?? 0) + (m.places ?? 0),
          note:
            "Different code: the same issue at another version. Check that what the reasoning rests on exists there.",
          tone: "warn",
        });
      }
    }
    for (const build of differing) auto.delete(build);
    return { matching: [...auto.keys()].sort(), offered: [...diff.values()].sort((a, b) => a.key.localeCompare(b.key)) };
  }, [reach]);

  const ready =
    covering.length > 0 &&
    reasoning.trim() !== "" &&
    (!needsDate || until !== "") &&
    (!needsFixedVersion || fixedVersion.trim() !== "") &&
    (!needsMitigation || mitigation.trim() !== "");

  function body(narrow: boolean) {
    return {
      outcome: outcome as "affected" | "not-applicable" | "deferred" | "wont-fix" | "already-fixed",
      ...(needsJustification ? { justification } : {}),
      ...(needsMitigation ? { mitigation } : {}),
      ...(needsDate ? { deferred_until: until } : {}),
      ...(needsFixedVersion ? { fixed_version: fixedVersion.trim() } : {}),
      reasoning,
      ...(narrow && excluded.size > 0 ? { places: covering.map((p) => p.place ?? "") } : {}),
      ...(extending ? { extends: extending.claimId } : {}),
    };
  }

  const submit = useMutation({
    mutationFn: async (applied: Other[]): Promise<Recorded> => {
      const here = unwrap(
        await api.POST(
          "/v1/products/{product}/streams/{stream}/variants/{variant}/findings/{vulnerability}/components/{component}/decision",
          {
            params: {
              path: {
                product: at.product,
                stream: at.stream,
                variant: at.variant,
                vulnerability: at.vulnerability,
                component: at.component,
              },
              query: at.version ? { version: at.version } : {},
            },
            body: body(true),
          },
        ),
      );
      // Then each build the review applied it to, one at a time, so a
      // refusal on one is reported for that one and does not decide the rest.
      const results: Recorded["applied"] = [];
      for (const other of applied) {
        try {
          unwrap(
            await api.POST(
              "/v1/products/{product}/streams/{stream}/variants/{variant}/findings/{vulnerability}/components/{component}/decision",
              {
                params: {
                  path: {
                    product: at.product,
                    stream: other.stream,
                    variant: other.variant,
                    vulnerability: at.vulnerability,
                    component: at.component,
                  },
                  // The other build may ship this component at several
                  // versions, and the reach named the one the issue sits at
                  // there; without it, a build with four is a refusal.
                  query: other.version ? { version: other.version } : {},
                },
                // Only what nothing stands at there: the places at matching
                // versions are already reached by lookup, and a second
                // claim about them would be refused.
                body: { ...body(false), remaining: true },
              },
            ),
          );
          results.push({ build: other.version ? `${other.build} at ${other.version}` : other.build, ok: true });
        } catch (error) {
          results.push({
            build: other.version ? `${other.build} at ${other.version}` : other.build,
            ok: false,
            said: error instanceof Error ? error.message : String(error),
          });
        }
      }
      return {
        claimId: here.claim_id,
        recorded: here.recorded,
        covered: here.covered,
        left: here.left,
        needsApproval: here.needs_approval,
        applied: results,
        matching: matching.length,
      };
    },
    onSuccess: (recorded) => {
      forget(draftKey);
      setReviewing(false);
      void queries.invalidateQueries({ queryKey: ["finding"] });
      void queries.invalidateQueries({ queryKey: ["findings"] });
      void queries.invalidateQueries({ queryKey: ["queue"] });
      void queries.invalidateQueries({ queryKey: ["home"] });
      onDone(recorded);
    },
  });

  function start() {
    if (!reasoning.trim()) {
      setRefused("Reasoning is required.");
      return;
    }
    if (needsFixedVersion && !fixedVersion.trim()) {
      setRefused("Say which package version the fix arrived in, so somebody can check it.");
      return;
    }
    if (needsDate && !until) {
      setRefused("A deferral needs a date.");
      return;
    }
    if (needsMitigation && !mitigation.trim()) {
      setRefused("Say what stops it.");
      return;
    }
    if (covering.length === 0) {
      setRefused("Every location is excluded, so there is nothing to decide.");
      return;
    }
    setRefused(null);
    setReviewing(true);
  }

  // The keys, when this is open in the list: digits pick the outcome and r
  // submits. Never while typing, and never under the review sheet.
  useEffect(() => {
    if (!inline) return;
    function key(event: KeyboardEvent) {
      if (document.body.dataset.sheet) return;
      const tag = (document.activeElement?.tagName ?? "").toUpperCase();
      const inField = /INPUT|TEXTAREA|SELECT/.test(tag);
      if ((event.ctrlKey || event.metaKey) && event.key === "Enter") {
        event.preventDefault();
        start();
        return;
      }
      if (inField) return;
      if ("12345".includes(event.key) && event.key !== "") {
        const chosen = OUTCOMES[Number(event.key) - 1];
        if (chosen) {
          event.preventDefault();
          setOutcome(chosen.value);
        }
      } else if (event.key === "r") {
        event.preventDefault();
        start();
      }
    }
    document.addEventListener("keydown", key);
    return () => document.removeEventListener("keydown", key);
  });

  const plan: Plan = {
    build: `${at.product} · ${at.stream} · ${at.variant}`,
    covered: covering.length,
    total: open.length,
    matching,
    offered,
    blocked: [],
    reasoning,
    versionHere: at.version,
  };

  const findingPath =
    `/products/${encodeURIComponent(at.product)}/streams/${encodeURIComponent(at.stream)}` +
    `/variants/${encodeURIComponent(at.variant)}/findings/${encodeURIComponent(at.vulnerability)}` +
    `/components/${encodeURIComponent(at.component)}` +
    (at.version ? `?version=${encodeURIComponent(at.version)}` : "");

  if (open.length === 0) {
    return (
      <div className={inline ? "inline-decide" : "card"}>
        <div>
          <h3 style={{ margin: "0 0 6px" }}>Decision</h3>
          <p className="reading">
            Every location this sits at has been decided. Revising one of those claims is done from
            the claim itself.
          </p>
        </div>
      </div>
    );
  }

  const form = (
    <div>
      {inline && (
        <div className="ihead">
          <span className="id">{at.vulnerability}</span> in <span className="id">{at.component}</span>
          <span className="hint" style={{ marginLeft: "auto" }}>
            {open.length} open {open.length === 1 ? "location" : "locations"}
            {answered > 0 ? ` · ${answered} decided` : ""}
          </span>
        </div>
      )}
      {!inline && answered > 0 && (
        <p className="hint" style={{ margin: "-4px 0 12px" }}>
          {answered} of {places.length} locations already decided. This covers what is left.
        </p>
      )}
      {extending && (
        <div className="alert info" style={{ marginBottom: 12 }}>
          <strong>Extends decision #{extending.decisionId}</strong>
          <span>The same argument, read once already. It still needs a second person.</span>
        </div>
      )}

      <div className="field">
        <label>Outcome</label>
        <div className="outcomes">
          {OUTCOMES.map((each, i) => (
            <button
              key={each.value}
              type="button"
              className="outcome"
              aria-pressed={outcome === each.value}
              onClick={() => setOutcome(each.value)}
            >
              {inline && <kbd>{i + 1}</kbd>} {each.label}
            </button>
          ))}
        </div>
      </div>

      {needsJustification && (
        <div className="field">
          <label htmlFor={`${draftKey}-just`}>Justification</label>
          <select
            id={`${draftKey}-just`}
            value={justification}
            onChange={(event) => setJustification(event.target.value as Justification)}
          >
            {JUSTIFICATIONS.map((each) => (
              <option key={each.value} value={each.value}>
                {each.value}
              </option>
            ))}
          </select>
        </div>
      )}

      {needsMitigation && (
        <div className="field">
          <label htmlFor={`${draftKey}-mit`}>What stops it</label>
          <input
            id={`${draftKey}-mit`}
            type="text"
            value={mitigation}
            placeholder="the firewall rule, the setting, the service that is not exposed"
            onChange={(event) => setMitigation(event.target.value)}
          />
          <span className="hint">
            Nothing here watches configuration, so this claim will not lapse when the thing that
            stops it is removed. Say what to go and check.
          </span>
        </div>
      )}

      {needsFixedVersion && (
        <div className="field">
          <label htmlFor={`${draftKey}-fixed`}>Fixed in</label>
          <input
            id={`${draftKey}-fixed`}
            type="text"
            value={fixedVersion}
            placeholder="the package version the fix arrived in, as the packager writes it"
            onChange={(event) => setFixedVersion(event.target.value)}
          />
          <span className="hint">
            A backported fix does not move the upstream version, so nothing here can see it. This is
            what the next person checks against the packager's own record — it is recorded and never
            compared against the version shipping.
          </span>
        </div>
      )}

      {needsDate && (
        <div className="field">
          <label htmlFor={`${draftKey}-until`}>Until</label>
          <input
            id={`${draftKey}-until`}
            type="date"
            value={until}
            style={{ width: 180 }}
            onChange={(event) => setUntil(event.target.value)}
          />
          <span className="hint">Under the deferral threshold, no approval is needed; over it, a second person.</span>
        </div>
      )}

      <div className="field" style={{ margin: 0 }}>
        <label>Reasoning</label>
        <Editor
          value={reasoning}
          onChange={setReasoning}
          draftKey={draftKey}
          label="Reasoning"
          mentions={{ product: at.product }}
          attachTo={{ product: at.product, vulnerability: at.vulnerability }}
          placeholder="Why this decision holds, and what to re-check later."
        />
      </div>

      {refused && (
        <p className="hint" style={{ color: "var(--sev-high)", marginTop: 8 }}>
          {refused}
        </p>
      )}
      {submit.error != null && <Failed error={submit.error} what="That could not be recorded." />}

      <div className="actions" style={{ marginTop: 12 }}>
        <button type="button" className="btn" disabled={!ready || submit.isPending} onClick={start}>
          {inline ? (
            <>
              Submit and next <kbd className="onbtn">r</kbd>
            </>
          ) : (
            "Submit decision"
          )}
        </button>
        {inline && onClose && (
          <button type="button" className="btn quiet" onClick={onClose}>
            Close
          </button>
        )}
        {inline && (
          <Link to={findingPath} className="linkish" style={{ marginLeft: 6 }}>
            Open the full finding →
          </Link>
        )}
        {position && (
          <span className="consequence">
            Row {position.row} of {position.of}
          </span>
        )}
      </div>
    </div>
  );

  const aside = (
    <aside style={{ alignSelf: "start", display: "flex", flexDirection: "column", gap: 10 }}>
      <Covering
        places={open}
        excluded={excluded}
        onChange={setExcluded}
        matching={matching.length}
        differing={offered.length}
      />
      <div className="tier auto" style={{ margin: 0 }}>
        <p className="said">
          {outcome === "affected"
            ? "Affected needs no approval. It goes to remediation."
            : outcome === "deferred"
              ? "Under the deferral threshold this stands on its own; over it, a second person."
              : "Dismissals take effect only after a second person approves."}
        </p>
      </div>
    </aside>
  );

  return (
    <>
      {inline ? (
        <div className="inline-decide">
          {form}
          {aside}
        </div>
      ) : (
        <div className="card">
          <h3>Decision</h3>
          <div className="writing">
            {form}
            {aside}
          </div>
        </div>
      )}
      <Review
        open={reviewing}
        plan={plan}
        busy={submit.isPending}
        error={submit.error}
        onCancel={() => setReviewing(false)}
        onConfirm={(applied) => submit.mutate(applied)}
      />
    </>
  );
}
