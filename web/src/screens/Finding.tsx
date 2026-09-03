import { Fragment, useMemo, useState } from "react";
import { useMutation, useQueries, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useParams, useSearchParams } from "react-router-dom";
import { api, type Body } from "../api/client";
import { at as choicesAt, unwrap } from "../api/queries";
import { useComment, useRevise, useWithdraw } from "../api/mutations";
import { useWho } from "../app/session";
import { Failed } from "../ui/Failed";
import { Severity, Exploited } from "../ui/Severity";
import { Markdown } from "../ui/Markdown";
import { Editor, forget } from "../ui/Editor";
import { Decide, type Recorded } from "../ui/Decide";
import { UNPLACED, type Sitting } from "../ui/Covering";

// One finding: what the issue is, how bad, what upstream has done, where it
// sits, the evidence — and the working screen for deciding it, before and
// after (UIX-46). When a claim stands it is shown in its state, with one
// activity timeline, the revision history, the comments, and the decisions
// made at this location before, whose reasoning is offered back.

type Detail = Body<"DecisionDetail">;

type Similar = Body<"SimilarBody">;

// How many of the finding's places are asked which decision stood there
// before. The finding carries the earlier decisions themselves; what it does
// not carry is which place each was at, and reaffirming one needs the place.
const SAMPLE = 8;

export function Finding() {
  const { product = "", stream = "", variant = "", vulnerability = "", component = "" } = useParams();
  const [params] = useSearchParams();
  const version = params.get("version") ?? "";
  const who = useWho();
  const at = { product, stream, variant, vulnerability, component };
  const [recorded, setRecorded] = useState<Recorded | null>(null);
  const [prefill, setPrefill] = useState<{ outcome?: string; justification?: string; reasoning?: string } | null>(null);
  const [extending, setExtending] = useState<{ claimId: number; decisionId: number } | null>(null);

  const finding = useQuery({
    queryKey: ["finding", at, version],
    queryFn: async () =>
      unwrap(
        await api.GET(
          "/v1/products/{product}/streams/{stream}/variants/{variant}/findings/{vulnerability}/components/{component}",
          { params: { path: at, query: version ? { version } : {} } },
        ),
      ),
  });

  const it = finding.data;
  const places = useMemo(() => it?.places ?? [], [it]);
  // A location is the component and what pulls it in; two chains reaching the
  // same pair are one location, and the head counts what a decision covers.
  const distinct = new Set(places.map((place) => place.place)).size;
  // The claims standing here, one representative decision each, read for the
  // reasoning and the approvals the claim row does not carry.
  const standingIds = useMemo(() => (it?.standing ?? []).map((c) => c.decision_id), [it]);
  const standing = useQueries({
    queries: standingIds.slice(0, SAMPLE).map((id) => ({
      queryKey: ["decision", id],
      queryFn: async () => unwrap(await api.GET("/v1/decisions/{id}", { params: { path: { id } } })),
    })),
  });
  // Which place each earlier decision was at, for reaffirming it.
  const openPlaces = places.filter((p) => p.decision == null).slice(0, SAMPLE);
  const history = useQueries({
    queries: ((it?.previous ?? []).some((p) => p.ended === "lapsed") ? openPlaces : []).map((place) => ({
      queryKey: ["decided", at, place.place],
      queryFn: async () =>
        unwrap(
          await api.GET(
            "/v1/products/{product}/streams/{stream}/variants/{variant}/findings/{vulnerability}/places/{place}/decision",
            { params: { path: { ...at, place: place.place ?? "" } } },
          ),
        ),
    })),
  });

  if (finding.isPending) return <p className="hint">Loading…</p>;
  if (finding.isError) {
    const choices = choicesAt(finding.error, "query.version");
    if (choices.length > 0) {
      return (
        <div className="card">
          <h3>Which {component}?</h3>
          <p className="reading" style={{ marginBottom: 10 }}>
            This build ships that name at more than one version. Pick the one you mean.
          </p>
          <ul className="refs">
            {choices.map((choice) => (
              <li key={`${choice.version} ${choice.ecosystem ?? ""}`}>
                <Link
                  className="linkish id"
                  to={`?version=${encodeURIComponent(choice.version)}`}
                >
                  {choice.version}
                </Link>
                {choice.ecosystem && <span className="hint">{choice.ecosystem}</span>}
              </li>
            ))}
          </ul>
        </div>
      );
    }
    return <Failed error={finding.error} what="This finding could not be read." />;
  }
  if (!it) return null;

  const claims = standing.map((q) => q.data).filter((d): d is Detail => !!d);
  const placeOf = new Map<number, string>();
  history.forEach((q, i) => {
    for (const d of q.data?.previously ?? []) {
      if (d.decision?.id && !placeOf.has(d.decision.id)) placeOf.set(d.decision.id, openPlaces[i]?.place ?? "");
    }
  });
  const previous: Previous[] = (it.previous ?? []).map((p) => ({
    id: p.decision_id,
    outcome: p.outcome,
    justification: p.justification ?? "",
    deferredUntil: p.deferred_until ?? "",
    proposedBy: p.proposed_by,
    proposedAt: p.proposed_at,
    state: p.ended,
    endedAt: p.ended_at ?? "",
    about: p.about ?? "",
    approvedBy: p.approved_by ?? "",
    reasoning: p.reasoning,
    place: placeOf.get(p.decision_id) ?? "",
  }));
  const similar: Similar[] = it.similar ?? [];

  // Counted as locations, the way the decision counts them, not as chain rows.
  const decided = new Set(places.filter((p) => p.decision != null).map((p) => p.place)).size;
  const state = stateOf(claims, decided, distinct, (it.standing ?? []).map((each) => each.state ?? ""));
  const back =
    `/products/${encodeURIComponent(product)}` +
    `/streams/${encodeURIComponent(stream)}` +
    `/variants/${encodeURIComponent(variant)}/findings`;
  const build =
    `/products/${encodeURIComponent(product)}` +
    `/streams/${encodeURIComponent(stream)}` +
    `/variants/${encodeURIComponent(variant)}`;
  const mine = (proposedBy: string) =>
    !!who.data && (proposedBy === who.data.identity || proposedBy === who.data.name);

  return (
    <>
      <div className="screen-head">
        <span className="crumbs">
          <Link to={back} className="linkish" style={{ fontWeight: 500, color: "var(--muted)" }}>
            Findings
          </Link>{" "}
          › <b>{it.vulnerability}</b>
        </span>
        <h2>
          <span className="id">{it.vulnerability}</span> in <span className="id">{it.component}</span>{" "}
          <Severity word={it.assessed || it.severity} /> {it.exploited && <Exploited when />}{" "}
          <span className={`state ${state.cls}`}>{state.label}</span>
        </h2>
        <p>
          {product} · {stream} · {variant} · {distinct} {distinct === 1 ? "location" : "locations"} ·{" "}
          <b style={{ color: "var(--ink)" }}>
            {decided} of {distinct} decided
          </b>
        </p>
      </div>

      {recorded && (
        <div className="alert info" style={{ marginBottom: 14 }}>
          <strong>Submitted</strong>
          <span>
            Recorded against {recorded.recorded} {recorded.recorded === 1 ? "location" : "locations"} here
            {recorded.applied.filter((a) => a.ok).length > 0 && (
              <>
                , and in {recorded.applied.filter((a) => a.ok).map((a) => a.build).join(", ")}
              </>
            )}
            ; {recorded.matching} matching {recorded.matching === 1 ? "build is" : "builds are"} reached by lookup.{" "}
            {recorded.needsApproval
              ? "The dismissal takes effect once a second person approves it. It is now in the review queue."
              : "In force now."}
            {recorded.applied.filter((a) => !a.ok).map((a) => (
              <Fragment key={a.build}>
                <br />
                <b>{a.build}</b>: {a.said}
              </Fragment>
            ))}{" "}
            <Link to="/review-queue" className="linkish">
              Go to the review queue →
            </Link>
          </span>
        </div>
      )}

      {it.arrived_from && (
        <div className="shortfall">
          <span className="icon">◭</span>
          <div>
            <h4>Upgraded, but not to a fixed version</h4>
            <p>
              <span className="id">{it.component}</span> moved <b>{it.arrived_from} → {it.version}</b>
              {it.fixed_in && (
                <>
                  ; this issue is fixed in <b>{it.fixed_in}</b>
                </>
              )}
              , so the bump could not have resolved it. The old reasoning probably still stands, and
              somebody's remediation did not land.
            </p>
            <div className="ladder">
              <span className="v was">{it.arrived_from}</span>
              <span className="arrow">→</span>
              <span className="v now">{it.version} shipped</span>
              {it.fixed_in && (
                <>
                  <span className="arrow">·</span>
                  <span className="v need">{it.fixed_in} fixes it</span>
                </>
              )}
            </div>
          </div>
        </div>
      )}

      <div className="evidence">
        {it.description && (
          <div className="evblock">
            <h4>Description</h4>
            {/* What a scan file said is shown, never rendered (SEC-16). */}
            <p style={{ whiteSpace: "pre-wrap" }}>{it.description}</p>
          </div>
        )}

        <div className="evblock">
          <h4>Severity</h4>
          <div className="scores">
            <div className="score">
              <span className="n">{it.score ? it.score.toFixed(1) : "—"}</span>
              <span className="l">CVSS</span>
            </div>
            <div className="score">
              <span className="n">{typeof it.likelihood === "number" ? it.likelihood.toFixed(3) : "—"}</span>
              <span className="l">EPSS</span>
            </div>
            <div className="score">
              <span className="n">{(it.weaknesses ?? [])[0] ?? "—"}</span>
              <span className="l">Weakness</span>
            </div>
            <div className="score">
              <span className="n">{it.exploited ? "Yes" : "No"}</span>
              <span className="l">Exploited</span>
            </div>
          </div>
          {it.vector && (
            <p className="mono" style={{ fontSize: "var(--step--1)", color: "var(--muted)" }}>
              {it.vector}
            </p>
          )}
          <p className="hint">
            {it.assessed ? (
              <>
                Assessed <Severity word={it.assessed} /> · published <Severity word={it.severity} />. The
                assessment orders it and sets its deadline.
              </>
            ) : (
              <>
                Published <Severity word={it.severity} />. Being exploited outranks the score.
              </>
            )}
          </p>
        </div>

        <References advisory={it.advisory} refs={it.references ?? []} />

        <div className="evblock">
          <h4>Upstream</h4>
          <p>
            <span className="id">
              {it.component} {it.version}
            </span>
            {it.upstream && (
              <>
                {" "}
                — cut from <span className="id">{it.upstream}</span>
              </>
            )}
            {". "}
            {it.fix_state === "fixed" && it.fixed_in ? (
              <>
                Fixed upstream in <span className="id">{it.fixed_in}</span>
                {it.fixed_at && <>, available since {it.fixed_at.slice(0, 10)}</>}.
              </>
            ) : it.fix_state === "wont-fix" ? (
              <>Upstream has declined to fix this.</>
            ) : (
              <>No fix has been published.</>
            )}
          </p>
          {it.latest_version && (
            <p className="hint" style={{ margin: 0 }}>
              Newest upstream release is <span className="id">{it.latest_version}</span>
              {it.latest_released_at && <>, which shipped {it.latest_released_at}</>}.
            </p>
          )}
          {it.nothing_since && (
            <p className="alert" style={{ margin: 0 }}>
              <span>
                Nothing has been released upstream since {it.latest_released_at ?? "well before"}, over
                a year before this issue was named, and there is no fix. Replacing or patching the
                component is the response available.
              </span>
            </p>
          )}
        </div>

        <Places places={places} build={build} />

        {(it.aliases ?? []).length > 0 && (
          <div className="evblock">
            <h4>Aliases</h4>
            <p className="id">{(it.aliases ?? []).join(" · ")}</p>
          </div>
        )}
      </div>

      <div className="deciding">
        {claims.map((claim, i) => (
          <Standing
            key={claim.decision?.id}
            claim={claim}
            summary={it.standing?.[i]}
            mine={mine(claim.proposed_by ?? "")}
            mayApprove={!!who.data?.reach.find((r) => r.product === product)?.may_agree}
            onRevised={() => void finding.refetch()}
          />
        ))}

        {claims.length > 0 && claims[0]?.decision?.id && (
          <>
            <Activity decisionId={claims[0].decision.id} claim={claims[0]} places={it.standing?.[0]?.places} previous={previous} />
            <Revisions decisionId={claims[0].decision.id} />
            <Comments decisionId={claims[0].decision.id} />
          </>
        )}

        <Holder at={at} assigned={it.assigned_to ?? ""} />

        <FixingIn at={at} />

        {places.some((p) => p.decision == null) && (
          <>
            <Assess vulnerability={vulnerability} published={it.severity ?? ""} assessed={it.assessed ?? ""} />
            {similar.length > 0 && !extending && (
              <div className="card">
                <h3>Approved decisions at this component</h3>
                <p className="reading" style={{ marginBottom: 8 }}>
                  The same component under the same consumer, with the same justification. Applying one
                  extends its argument to this issue; it still needs a second person.
                </p>
                {similar.map((s) => (
                  <div key={s.decision_id} className="prior">
                    <header>
                      <span className="id">#{s.decision_id}</span>{" "}
                      <span className="mono">{s.justification}</span>
                      <span className="hint">
                        approved by {s.approved_by}
                        {s.approved_at && <> on {s.approved_at.slice(0, 10)}</>}
                        {s.issues ? <> · {s.issues} issues rest on it</> : null}
                      </span>
                    </header>
                    <div className="why">
                      <Markdown source={s.reasoning ?? ""} />
                    </div>
                    <div className="actions">
                      <button
                        type="button"
                        className="btn ghost"
                        onClick={() => {
                          setExtending({ claimId: s.claim_id, decisionId: s.decision_id });
                          setPrefill({
                            outcome: "not-applicable",
                            justification: s.justification,
                            reasoning: s.reasoning,
                          });
                        }}
                      >
                        Apply decision #{s.decision_id} to this issue →
                      </button>
                    </div>
                  </div>
                ))}
              </div>
            )}
            <Decide
              at={{ ...at, version }}
              places={places}
              onDone={(r) => {
                setRecorded(r);
                setPrefill(null);
                setExtending(null);
              }}
              extending={extending}
              prefill={prefill}
            />
          </>
        )}

        {previous.length > 0 && (
          <PreviousCard
            items={previous}
            at={at}
            undecided={places.some((p) => p.decision == null)}
            onReuse={(reasoning, outcome, justification) => {
              setPrefill({ reasoning, outcome, justification });
              document.querySelector(".deciding .card")?.scrollIntoView({ behavior: "smooth", block: "start" });
            }}
          />
        )}
      </div>
    </>
  );
}

type Previous = {
  id: number;
  outcome: string;
  justification: string;
  deferredUntil: string;
  proposedBy: string;
  proposedAt: string;
  state: string;
  endedAt: string;
  about: string;
  approvedBy: string;
  reasoning: string;
  place: string;
};

// The head's pill reads each claim's state as a whole where the finding
// reports it, not the representative row's: one row approved and forty
// returned is pending, not approved.
function stateOf(claims: Detail[], decided: number, total: number, overall: string[]): { label: string; cls: string } {
  if (claims.length === 0) return decided > 0 && total > 0 ? { label: "Decided", cls: "agreed" } : { label: "Undecided", cls: "open" };
  const states = overall.length === claims.length ? overall : claims.map((c) => c.decision?.state ?? "");
  if (states.some((s) => s === "proposed")) return { label: "Pending approval", cls: "waiting" };
  if (states.every((s) => s === "approved")) {
    const outcome = claims[0]?.decision?.outcome ?? "";
    return { label: `${OUTCOME[outcome] ?? outcome} · approved`, cls: "agreed" };
  }
  if (states.some((s) => s === "lapsed")) return { label: "Lapsed", cls: "lapsed" };
  return { label: "Decided", cls: "agreed" };
}

const OUTCOME: Record<string, string> = {
  affected: "Affected",
  "not-applicable": "Not applicable",
  deferred: "Deferred",
  "wont-fix": "Won't fix",
};

// The claim that stands, in its state, with outcome, justification, scope,
// approval, the reasoning rendered, and what may be done to it.
function Standing({
  claim,
  summary,
  mine,
  mayApprove,
  onRevised,
}: {
  claim: Detail;
  summary?: Body<"StandingClaimBody">;
  mine: boolean;
  mayApprove: boolean;
  onRevised: () => void;
}) {
  const id = claim.decision?.id ?? 0;
  // The claim's state as a whole, not its representative row's: a claim with
  // one row approved and forty sent back is not approved.
  const state = summary?.state ?? claim.decision?.state ?? "";
  const rows = summary?.rows;
  const mixed = !!rows && [rows.proposed ?? 0, rows.sent_back ?? 0, rows.approved ?? 0].filter((n) => n > 0).length > 1;
  const sentBackAt = summary?.sent_back_at ?? claim.decision?.sent_back_at;
  const [editing, setEditing] = useState(false);
  const [text, setText] = useState(claim.reasoning ?? "");
  const revise = useRevise();
  const withdraw = useWithdraw();
  const approvals = useQuery({
    queryKey: ["decision", id, "approvals"],
    queryFn: async () => unwrap(await api.GET("/v1/decisions/{id}/approvals", { params: { path: { id } } })),
  });
  const live = (approvals.data?.items ?? []).filter((a) => !a.withdrawn_at);
  const last = live[live.length - 1];
  const draftKey = `revise:${id}`;
  const stripe = state === "proposed" ? "pending" : state === "approved" ? "approved" : state === "lapsed" ? "lapsed" : "";

  return (
    <div className={`card standing ${stripe}`}>
      <header className="dhead">
        <h3>
          Decision <span className="id">#{summary?.claim_id ?? id}</span>
        </h3>
        <span className="hint">
          proposed by <b>{claim.proposed_by}</b>
          {claim.proposed_at && <> · {claim.proposed_at.replace("T", " ").slice(0, 16)}</>}
          {typeof claim.age_days === "number" && claim.age_days > 365 && (
            <>
              {" "}
              · <span style={{ color: "var(--sev-medium)" }}>a judgment this old is worth re-reading</span>
            </>
          )}
        </span>
      </header>
      {sentBackAt && (
        <div className="alert" style={{ marginBottom: 12 }}>
          <strong>
            Rejected on {sentBackAt.slice(0, 10)}
            {rows && (rows.sent_back ?? 0) > 0 && (rows.sent_back ?? 0) < (rows.proposed ?? 0) + (rows.sent_back ?? 0) + (rows.approved ?? 0)
              ? ` — ${rows.sent_back} of ${(rows.proposed ?? 0) + (rows.sent_back ?? 0) + (rows.approved ?? 0)} records`
              : ""}
          </strong>
          <span>
            {summary?.sent_back_because ? <>{summary.sent_back_because} — </> : null}
            back with whoever wrote it, and out of the review queue until it is revised.
          </span>
        </div>
      )}
      <div className="dgrid">
        <div>
          <span className="l">Outcome</span>
          <span className="v">
            {OUTCOME[claim.decision?.outcome ?? ""] ?? claim.decision?.outcome}
            {claim.decision?.deferred_until && <> until {claim.decision.deferred_until}</>}
          </span>
        </div>
        {claim.decision?.justification && (
          <div>
            <span className="l">Justification</span>
            <span className="v mono">{claim.decision.justification}</span>
            {claim.decision.mitigation && <span className="hint">stopped by: {claim.decision.mitigation}</span>}
          </div>
        )}
        <div>
          <span className="l">Scope</span>
          <span className="v">
            {summary?.places ?? claim.decision?.places ?? 1}{" "}
            {(summary?.places ?? claim.decision?.places ?? 1) === 1 ? "location" : "locations"} here
            {(summary?.builds ?? []).length > 0 && <> · also {summary?.builds?.join(", ")}</>}
            {summary?.kind === "extension" && <> · extends an approved claim</>}
            {claim.decision?.selected_by && <> · narrowed by: {claim.decision.selected_by}</>}
          </span>
        </div>
        <div>
          <span className="l">Approval</span>
          <span className="v">
            {state === "proposed" ? (
              <>
                <span className="state waiting">Pending</span> waiting for a second person
                {mixed && rows && (
                  <> · {rows.approved ?? 0} approved · {rows.proposed ?? 0} pending · {rows.sent_back ?? 0} returned</>
                )}
                {mine && " — you proposed this, so you cannot approve it"}
                {!mine && mayApprove && " — you may approve or reject it from the review queue"}
              </>
            ) : state === "approved" && last ? (
              <>
                <span className="state agreed">Approved</span> by <b>{last.approved_by}</b>
                {last.approved_at && <>, {last.approved_at.replace("T", " ").slice(0, 16)}</>}
                {typeof last.covered === "number" && <> · covered {last.covered} records at the time</>}
              </>
            ) : state === "approved" ? (
              <>
                <span className="state agreed">Approved</span>
                {claim.decision?.needs_approval === false && " — needed nobody: under the deferral threshold"}
              </>
            ) : (
              <span className={`state ${state === "lapsed" ? "lapsed" : "open"}`}>{state}</span>
            )}
          </span>
        </div>
      </div>

      {editing ? (
        <div style={{ marginTop: 12, maxWidth: "78ch" }}>
          <div className="alert" style={{ marginBottom: 10 }}>
            <strong>Revising the reasoning withdraws the approval</strong>
            <span>
              The earlier words stay readable in the revision history, and the decision returns to the
              review queue marked as previously approved.
            </span>
          </div>
          <Editor value={text} onChange={setText} draftKey={draftKey} label="Reasoning" />
          {revise.error != null && <Failed error={revise.error} what="That could not be stored." />}
          <div className="actions" style={{ marginTop: 8 }}>
            <button
              type="button"
              className="btn"
              disabled={!text.trim() || revise.isPending}
              onClick={() =>
                revise.mutate(
                  { id, reasoning: text },
                  {
                    onSuccess: () => {
                      forget(draftKey);
                      setEditing(false);
                      onRevised();
                    },
                  },
                )
              }
            >
              Save revision
            </button>
            <button type="button" className="btn quiet" onClick={() => setEditing(false)}>
              Cancel
            </button>
          </div>
        </div>
      ) : (
        <div className="why rendered" style={{ marginTop: 12 }}>
          {claim.reasoning ? <Markdown source={claim.reasoning} /> : <p className="hint">Nothing written.</p>}
        </div>
      )}

      {withdraw.error != null && <Failed error={withdraw.error} what="That could not be withdrawn." />}
      {!editing && (state === "proposed" || state === "approved") && (
        <div className="actions" style={{ marginTop: 12 }}>
          <button
            type="button"
            className="btn ghost"
            onClick={() => {
              setText(claim.reasoning ?? "");
              setEditing(true);
            }}
          >
            Revise reasoning
          </button>
          <button
            type="button"
            className="btn quiet"
            disabled={withdraw.isPending}
            onClick={() => withdraw.mutate({ id }, { onSuccess: onRevised })}
          >
            Withdraw
          </button>
          <span className="consequence">
            {state === "approved" ? "Revising withdraws the approval; withdrawing needs nobody" : "Withdrawing needs nobody"}
          </span>
        </div>
      )}
    </div>
  );
}

type Event = { when: string; who: string; what: string; earlier?: boolean };

// One timeline: what happened to the claim that stands, and what happened at
// this location before it.
function Activity({ decisionId, claim, places, previous }: { decisionId: number; claim: Detail; places?: number; previous: Previous[] }) {
  const approvals = useQuery({
    queryKey: ["decision", decisionId, "approvals"],
    queryFn: async () =>
      unwrap(await api.GET("/v1/decisions/{id}/approvals", { params: { path: { id: decisionId } } })),
  });
  const revisions = useQuery({
    queryKey: ["decision", decisionId, "revisions"],
    queryFn: async () =>
      unwrap(await api.GET("/v1/decisions/{id}/revisions", { params: { path: { id: decisionId } } })),
  });
  const comments = useQuery({
    queryKey: ["comments", decisionId],
    queryFn: async () =>
      unwrap(await api.GET("/v1/decisions/{id}/comments", { params: { path: { id: decisionId } } })),
  });

  const now: Event[] = [];
  now.push({
    when: claim.proposed_at ?? "",
    who: claim.proposed_by ?? "",
    what: `proposed #${decisionId} — ${OUTCOME[claim.decision?.outcome ?? ""] ?? claim.decision?.outcome} · ${places ?? claim.decision?.places ?? 1} ${(places ?? claim.decision?.places ?? 1) === 1 ? "location" : "locations"}`,
  });
  for (const r of revisions.data?.items ?? []) {
    if ((r.ordinal ?? 1) > 1) now.push({ when: r.written_at ?? "", who: r.written_by ?? "", what: `revised the reasoning (revision ${r.ordinal})` });
  }
  for (const a of approvals.data?.items ?? []) {
    now.push({ when: a.approved_at ?? "", who: a.approved_by ?? "", what: `approved revision ${a.revision_id}${a.batch ? ` (batch ${a.batch})` : ""}` });
    if (a.withdrawn_at) now.push({ when: a.withdrawn_at, who: a.approved_by ?? "", what: "approval withdrawn" });
  }
  for (const c of comments.data?.items ?? []) {
    now.push({ when: c.written_at ?? "", who: c.written_by ?? "", what: "commented" });
  }
  if (claim.decision?.sent_back_at) now.push({ when: claim.decision.sent_back_at, who: "", what: "rejected — returned for revision" });
  now.sort((a, b) => b.when.localeCompare(a.when));

  const earlier: Event[] = previous.map((p) => ({
    when: p.proposedAt,
    who: p.proposedBy,
    what: `proposed #${p.id} — ${OUTCOME[p.outcome] ?? p.outcome}${p.state ? ` · ${p.state}` : ""}`,
    earlier: true,
  }));
  earlier.sort((a, b) => b.when.localeCompare(a.when));

  const line = (e: Event, i: number) => (
    <li key={`${e.when} ${e.what} ${i}`}>
      <span className="when">{e.when.replace("T", " ").slice(0, 16) || "—"}</span>
      <span className={`avatar${e.who ? "" : " none"}`}>{initials(e.who)}</span>
      <span className="what">
        {e.who && <b>{e.who} </b>}
        {e.what}
      </span>
    </li>
  );

  return (
    <div className="card">
      <h3>Activity</h3>
      <ul className="timeline">{now.map(line)}</ul>
      {earlier.length > 0 && (
        <>
          <p className="eyebrow" style={{ margin: "12px 0 6px" }}>
            Earlier at this location
          </p>
          <ul className="timeline earlier">{earlier.map(line)}</ul>
        </>
      )}
    </div>
  );
}

function initials(name: string): string {
  const parts = name.replace(/^[a-z]+:/, "").split(/[\s._@-]+/).filter(Boolean);
  if (parts.length === 0) return "⟳";
  if (parts.length === 1) return (parts[0] ?? "").slice(0, 2).toUpperCase();
  return ((parts[0]?.[0] ?? "") + (parts[1]?.[0] ?? "")).toUpperCase();
}

// Every revision is kept. An approval names the revision it was given for,
// and revising withdraws it.
function Revisions({ decisionId }: { decisionId: number }) {
  const revisions = useQuery({
    queryKey: ["decision", decisionId, "revisions"],
    queryFn: async () =>
      unwrap(await api.GET("/v1/decisions/{id}/revisions", { params: { path: { id: decisionId } } })),
  });
  const approvals = useQuery({
    queryKey: ["decision", decisionId, "approvals"],
    queryFn: async () =>
      unwrap(await api.GET("/v1/decisions/{id}/approvals", { params: { path: { id: decisionId } } })),
  });
  const items = revisions.data?.items ?? [];
  if (items.length === 0) return null;
  const agreed = new Map<number, { by: string; at: string; withdrawn: boolean }>();
  for (const a of approvals.data?.items ?? []) {
    if (a.revision_id) agreed.set(a.revision_id, { by: a.approved_by ?? "", at: a.approved_at ?? "", withdrawn: !!a.withdrawn_at });
  }

  return (
    <div className="card">
      <h3>Revision history</h3>
      <div className="history">
        {items.map((r) => {
          const a = r.id ? agreed.get(r.id) : undefined;
          return (
            <div key={r.id} className={`version${a && !a.withdrawn ? " agreed" : ""}`}>
              <span className="stamp">
                <b>Revision {r.ordinal}</b>
                {r.written_at?.replace("T", " ").slice(0, 16)} · {r.written_by}
              </span>
              <div>
                <div className="tagline">
                  {a ? (
                    <span className={a.withdrawn ? "state open" : "state agreed"}>
                      {a.withdrawn ? "Approval withdrawn" : "Approved"} by {a.by}
                      {a.at && <>, {a.at.slice(0, 10)}</>}
                    </span>
                  ) : (
                    <span className="state waiting">Never approved</span>
                  )}
                </div>
                <div className="words">
                  <Markdown source={r.body ?? ""} />
                </div>
              </div>
            </div>
          );
        })}
      </div>
      <p className="hint" style={{ margin: "10px 0 0" }}>
        Every revision is kept. An approval names the revision it was given for, and revising withdraws
        it.
      </p>
    </div>
  );
}

// Comments are separate from the reasoning and never affect an approval.
function Comments({ decisionId }: { decisionId: number }) {
  const [text, setText] = useState("");
  const comment = useComment();
  const draftKey = `comment:${decisionId}`;
  const comments = useQuery({
    queryKey: ["comments", decisionId],
    queryFn: async () =>
      unwrap(await api.GET("/v1/decisions/{id}/comments", { params: { path: { id: decisionId } } })),
  });
  const items = comments.data?.items ?? [];

  return (
    <div className="card">
      <h3>Comments</h3>
      {items.length > 0 && (
        <div className="thread">
          {items.map((each) => (
            <div key={each.id} className="said2">
              <span className="avatar">{initials(each.written_by ?? "")}</span>
              <div>
                <div className="meta">
                  <b>{each.written_by}</b>
                  <span className="when">{each.written_at?.replace("T", " ").slice(0, 16)}</span>
                  {each.edited_at && <span className="edited">edited</span>}
                </div>
                <div className="bubble">
                  <Markdown source={each.body ?? ""} />
                </div>
              </div>
            </div>
          ))}
        </div>
      )}
      <div className="field" style={{ margin: "14px 0 0", maxWidth: "78ch" }}>
        <label>Add a comment</label>
        <Editor
          value={text}
          onChange={setText}
          draftKey={draftKey}
          rows={4}
          label="Comment"
          placeholder="A question, a note, something worth knowing later."
        />
      </div>
      {comment.error != null && <Failed error={comment.error} what="That could not be added." />}
      <div className="actions">
        <button
          type="button"
          className="btn"
          disabled={!text.trim() || comment.isPending}
          onClick={() =>
            comment.mutate(
              { id: decisionId, body: text },
              {
                onSuccess: () => {
                  forget(draftKey);
                  setText("");
                },
              },
            )
          }
        >
          Comment
        </button>
        <span className="consequence">Does not affect the approval</span>
      </div>
    </div>
  );
}

// The decisions made here before — lapsed, withdrawn — with their reasoning
// offered back rather than thrown away, and a lapsed one reaffirmed in place.
function PreviousCard({
  items,
  at,
  undecided,
  onReuse,
}: {
  items: Previous[];
  at: { product: string; stream: string; variant: string; vulnerability: string };
  undecided: boolean;
  onReuse: (reasoning: string, outcome: string, justification: string) => void;
}) {
  return (
    <div className="card">
      <h3>Previous decisions at this location</h3>
      <p className="hint" style={{ margin: "0 0 10px" }}>
        A decision that lapsed or was withdrawn covers nothing, and is kept: what was argued last time is
        offered back rather than thrown away.
      </p>
      <div className="priors">
        {items.map((d) => (
          <Prior key={d.id} item={d} at={at} undecided={undecided} onReuse={onReuse} />
        ))}
      </div>
    </div>
  );
}

function Prior({
  item,
  at,
  undecided,
  onReuse,
}: {
  item: Previous;
  at: { product: string; stream: string; variant: string; vulnerability: string };
  undecided: boolean;
  onReuse: (reasoning: string, outcome: string, justification: string) => void;
}) {
  const queries = useQueryClient();
  const [note, setNote] = useState("");
  const [asking, setAsking] = useState(false);
  const reaffirm = useMutation({
    mutationFn: async () =>
      unwrap(
        await api.POST(
          "/v1/products/{product}/streams/{stream}/variants/{variant}/findings/{vulnerability}/places/{place}/decision/reaffirmation",
          {
            params: { path: { ...at, place: item.place } },
            body: { previous: item.id, reasoning: `${item.reasoning}\n\n${note}`.trim() },
          },
        ),
      ),
    onSuccess: () => {
      setAsking(false);
      void queries.invalidateQueries({ queryKey: ["finding"] });
      void queries.invalidateQueries({ queryKey: ["decided"] });
      void queries.invalidateQueries({ queryKey: ["queue"] });
    },
  });
  const lapsed = item.state === "lapsed";

  return (
    <article className="prior">
      <header>
        <span className="id">#{item.id}</span> <b>{OUTCOME[item.outcome] ?? item.outcome}</b>
        {item.justification && (
          <>
            {" "}
            · <span className="mono">{item.justification}</span>
          </>
        )}
        {item.deferredUntil && <> to {item.deferredUntil}</>}
        <span className={`state ${lapsed ? "lapsed" : "open"}`} style={{ marginLeft: "auto" }}>
          {item.state}
        </span>
      </header>
      <p className="hint" style={{ margin: "4px 0 6px" }}>
        {item.about && (
          <>
            About <span className="id">{item.about}</span> ·{" "}
          </>
        )}
        Proposed by {item.proposedBy}
        {item.proposedAt && <> on {item.proposedAt.slice(0, 10)}</>}
        {item.approvedBy && <> · approved by {item.approvedBy}</>}
        {item.endedAt && (
          <>
            {" "}
            · {item.state} {item.endedAt.slice(0, 10)}
          </>
        )}
      </p>
      {item.reasoning && (
        <div className="why">
          <Markdown source={item.reasoning} />
        </div>
      )}
      {reaffirm.error != null && <Failed error={reaffirm.error} what="That could not be reaffirmed." />}
      {asking && (
        <div className="field" style={{ margin: "8px 0 0", maxWidth: "78ch" }}>
          <label>Reaffirmation note</label>
          <textarea
            value={note}
            style={{ minHeight: 64 }}
            placeholder="What you checked against the version that ships now — the earlier reasoning stays above; this is the addendum."
            onChange={(event) => setNote(event.target.value)}
          />
        </div>
      )}
      <div className="actions">
        {lapsed && undecided && item.place && !asking && (
          <button type="button" className="btn" onClick={() => setAsking(true)}>
            Reaffirm
          </button>
        )}
        {asking && (
          <>
            <button
              type="button"
              className="btn"
              disabled={!note.trim() || reaffirm.isPending}
              onClick={() => reaffirm.mutate()}
            >
              Reaffirm
            </button>
            <button type="button" className="btn quiet" onClick={() => setAsking(false)}>
              Cancel
            </button>
            <span className="consequence">No approval needed: same justification, severity unchanged</span>
          </>
        )}
        {undecided && item.reasoning && !asking && (
          <button
            type="button"
            className="btn ghost"
            onClick={() => onReuse(item.reasoning, item.outcome, item.justification)}
          >
            Reuse this reasoning
          </button>
        )}
        <Link to={`/decisions/${item.id}`} className="linkish">
          Open #{item.id} →
        </Link>
      </div>
    </article>
  );
}

const RATINGS = ["low", "medium", "high", "critical"] as const;

// What we think of the issue itself, as against what was published. About the
// issue rather than where it sits, so it holds wherever the issue appears
// (TRI-40); rating it milder waits for a second person (TRI-41).
function Assess({ vulnerability, published, assessed }: { vulnerability: string; published: string; assessed: string }) {
  const queries = useQueryClient();
  const [open, setOpen] = useState(false);
  const [severity, setSeverity] = useState<string>(published || "medium");
  const [reasoning, setReasoning] = useState("");

  const assess = useMutation({
    mutationFn: async () =>
      unwrap(
        await api.POST("/v1/issues/{vulnerability}/assessment", {
          params: { path: { vulnerability } },
          body: { severity: severity as (typeof RATINGS)[number], reasoning },
        }),
      ),
    onSuccess: () => {
      setOpen(false);
      setReasoning("");
      void queries.invalidateQueries({ queryKey: ["finding"] });
    },
  });

  const milder =
    RATINGS.indexOf(severity as (typeof RATINGS)[number]) <
    RATINGS.indexOf((published || "medium") as (typeof RATINGS)[number]);

  if (assessed) {
    return (
      <div className="assess">
        <h3>Issue assessment</h3>
        <p className="reading" style={{ margin: 0 }}>
          Assessed <Severity word={assessed} />, published <Severity word={published} />. The assessment
          orders it and sets its deadline everywhere this issue appears.
        </p>
      </div>
    );
  }

  if (!open) {
    return (
      <div className="assess">
        <h3>Issue assessment</h3>
        <p className="reading" style={{ margin: "0 0 8px" }}>
          Published as <Severity word={published} />. A rating of ours holds wherever the issue appears.
        </p>
        <button type="button" className="linkish" onClick={() => setOpen(true)}>
          Rate it differently
        </button>
      </div>
    );
  }

  return (
    <div className="assess">
      <h3>Issue assessment</h3>
      {assess.error != null && <Failed error={assess.error} what="That could not be recorded." />}
      <div className="ourview">
        <div className="pair">
          <span className="l">Published</span>
          <span className="theirs">{published || "unrated"}</span>
        </div>
        <div className="field" style={{ margin: 0 }}>
          <label htmlFor="rating">Assessed</label>
          <select id="rating" value={severity} style={{ width: "auto" }} onChange={(event) => setSeverity(event.target.value)}>
            {RATINGS.map((each) => (
              <option key={each} value={each}>
                {each}
              </option>
            ))}
          </select>
        </div>
      </div>
      <p className="hint" style={{ margin: "0 0 8px" }}>
        {milder
          ? "Milder than published, so a second person has to agree before it takes effect."
          : "At or above what was published, so it takes effect at once."}
      </p>
      <div className="field" style={{ marginBottom: 8, maxWidth: "78ch" }}>
        <label htmlFor="why">Reasoning</label>
        <textarea
          id="why"
          style={{ minHeight: 64 }}
          value={reasoning}
          placeholder="What makes the published rating wrong for this issue, anywhere it appears?"
          onChange={(event) => setReasoning(event.target.value)}
        />
      </div>
      <div className="actions">
        <button type="button" className="btn" disabled={reasoning.trim() === "" || assess.isPending} onClick={() => assess.mutate()}>
          Save assessment
        </button>
        <button type="button" className="btn quiet" onClick={() => setOpen(false)}>
          Cancel
        </button>
      </div>
    </div>
  );
}

// Every place the component sits at, as the complete chain (UIX-14). A place
// is the component and what directly pulled it in, which is what a decision
// is recorded against.
const CHAINS = 6;

function Places({ places, build }: { places: Sitting[]; build: string }) {
  const [all, setAll] = useState(false);
  if (places.length === 0) return null;
  const shown = all ? places : places.slice(0, CHAINS);
  return (
    <div className="evblock">
      <h4>Dependency path</h4>
      <div className="tree">
        {shown.map((place, i) => {
          const chain = place.chain ?? [];
          const bare = chain.length <= 1;
          if (bare) {
            return (
              <div key={`${place.place} ${i}`} className="node here">
                <span className="rule">└</span>
                <span className="id">{chain[chain.length - 1]?.component ?? ""}</span>
                <span className="hint">{UNPLACED}</span>
                {place.decision != null && (
                  <Link to={`/decisions/${place.decision}`} className="linkish">
                    decided
                  </Link>
                )}
              </div>
            );
          }
          return (
            <Fragment key={`${place.place} ${i}`}>
              {chain.map((step, depth) => {
                const last = depth === chain.length - 1;
                return (
                  <div key={`${place.place} ${i} ${depth}`} className={`node${last ? " here" : ""}`} style={{ paddingLeft: depth * 18 }}>
                    <span className="rule">└</span>
                    <span className="id">{step.component}</span>
                    {step.version && <span className="ver">{step.version}</span>}
                    {last && place.suppressed && <span className="state open">suppressed by the build</span>}
                    {last && place.decision != null && (
                      <Link to={`/decisions/${place.decision}`} className="linkish">
                        decided
                      </Link>
                    )}
                  </div>
                );
              })}
            </Fragment>
          );
        })}
      </div>
      {places.length > CHAINS && (
        <button type="button" className="linkish" onClick={() => setAll(!all)}>
          {all ? `Show ${CHAINS}` : `Show all ${places.length} locations`}
        </button>
      )}
      {/* The tree is handed the whole chain, not only the name: it opens
          each step on the way down and lands on the component, so arriving
          from a finding shows where it sits rather than the root (UIX-14). */}
      <Link
        to={
          `${build}/components?at=${encodeURIComponent(places[0]?.chain?.at(-1)?.component ?? "")}` +
          `&path=${encodeURIComponent((places[0]?.chain ?? []).map((step) => step.component ?? "").join("\u001f"))}` +
          (places[0]?.chain?.at(-1)?.version ? `&version=${encodeURIComponent(places[0]?.chain?.at(-1)?.version ?? "")}` : "")
        }
        className="linkish"
      >
        View in dependency tree →
      </Link>
    </div>
  );
}

// Patches first: for somebody deciding whether to backport rather than
// upgrade, the change itself is the answer.
function References({ advisory, refs }: { advisory?: string; refs: { url?: string; kind?: string }[] }) {
  const all = advisory ? [{ url: advisory, kind: "advisory" }, ...refs] : refs;
  if (all.length === 0) return null;
  const order: Record<string, number> = { patch: 0, advisory: 1, report: 2, other: 3 };
  const sorted = [...all].sort((a, b) => (order[a.kind ?? "other"] ?? 9) - (order[b.kind ?? "other"] ?? 9));
  return (
    <div className="evblock">
      <h4>References</h4>
      <ul className="refs">
        {sorted.slice(0, 12).map((ref) => (
          <li key={ref.url}>
            <span className={ref.kind === "patch" ? "kind patch" : "kind"}>{ref.kind}</span>{" "}
            <a href={ref.url} target="_blank" rel="noreferrer noopener">
              {(ref.url ?? "").replace(/^https?:\/\//, "")}
            </a>
          </li>
        ))}
      </ul>
      <p className="hint">Patches first.</p>
    </div>
  );
}

// Who is dealing with this, and a way to change it.
//
// On the finding rather than only on the list of what nobody holds. Being able
// to record a judgment about something and not to say who is dealing with it is
// a strange half of the same job — and the screen somebody reads a finding on
// is the one they are on when they decide it needs a person.
//
// It covers every build of the product holding this component, which is what
// assigning means: the same code built several ways is one piece of work.
// Which releases this is meant to be fixed in, and what the scans say became of
// that.
//
// Nothing here is ticked off as done. A build clears when it stops holding the
// issue, which the next scan of it answers; a build chosen and still holding it
// after a scan has run is a missed target, and the scan is evidence against the
// claim rather than a reminder. A build nobody chose says so rather than
// sitting among the outstanding ones — nobody is made to answer the same
// question for six releases, but silence has to read as silence.
function FixingIn({
  at,
}: {
  at: { product: string; stream: string; variant: string; vulnerability: string; component: string };
}) {
  const queries = useQueryClient();
  const path =
    "/v1/products/{product}/streams/{stream}/variants/{variant}/findings/{vulnerability}/components/{component}/fix-targets" as const;
  const plan = useQuery({
    queryKey: ["fix-targets", at],
    queryFn: async () => unwrap(await api.GET(path, { params: { path: at } })),
  });
  const set = useMutation({
    mutationFn: async (builds: { stream: string; variant: string }[]) =>
      unwrap(await api.PUT(path, { params: { path: at }, body: { builds } })),
    onSuccess: () => {
      void queries.invalidateQueries({ queryKey: ["fix-targets"] });
      void queries.invalidateQueries({ queryKey: ["finding"] });
    },
  });

  const items = plan.data?.items ?? [];
  if (plan.isPending) return null;

  // The set is written whole, so a tick sends the whole list rather than one
  // build: intent spans several releases and is decided in one sitting.
  const chosen = items.filter(
    (row) => row.state === "fixing" || row.state === "missed" || row.state === "clear",
  );
  const toggle = (row: { stream?: string; variant?: string; state?: string }) => {
    const named = { stream: row.stream ?? "", variant: row.variant ?? "" };
    const now = chosen.map((each) => ({ stream: each.stream ?? "", variant: each.variant ?? "" }));
    const already = now.some((each) => each.stream === named.stream && each.variant === named.variant);
    set.mutate(
      already
        ? now.filter((each) => !(each.stream === named.stream && each.variant === named.variant))
        : [...now, named],
    );
  };

  const declared = plan.data?.declared ?? 0;
  const clear = plan.data?.clear ?? 0;
  const missed = plan.data?.missed ?? 0;

  return (
    <div className="card">
      <h3>Which releases this is fixed in</h3>
      {set.error != null && <Failed error={set.error} what="That could not be recorded." />}
      {items.length === 0 ? (
        <p className="hint" style={{ margin: 0 }}>
          No build of this product holds this issue.
        </p>
      ) : (
        <>
          <p className="reading" style={{ marginBottom: 10 }}>
            {declared === 0
              ? "Nobody has said where this will be fixed."
              : plan.data?.resolved
                ? `Fixed in all ${declared} of the releases chosen.`
                : `${clear} of ${declared} chosen ${clear === 1 ? "release is" : "releases are"} clear` +
                  (missed > 0 ? `, and ${missed} ${missed === 1 ? "was" : "were"} scanned since and still hold it.` : ".")}
          </p>
          <div className="tablewrap">
            <table>
              <thead>
                <tr>
                  <th />
                  <th>Release</th>
                  <th className="num">Places</th>
                  <th>State</th>
                  <th>Chosen</th>
                </tr>
              </thead>
              <tbody>
                {items.map((row) => (
                  <tr key={`${row.stream}/${row.variant}`}>
                    <td>
                      <input
                        type="checkbox"
                        aria-label={`Fix this in ${row.stream} ${row.variant}`}
                        checked={row.state === "fixing" || row.state === "missed" || row.state === "clear"}
                        disabled={set.isPending || row.state === "retired" || row.state === "gone"}
                        onChange={() => toggle(row)}
                      />
                    </td>
                    <td>
                      {row.stream} <span className="hint">{row.variant}</span>
                    </td>
                    <td className="num">{row.places || "—"}</td>
                    <td>
                      <FixState state={row.state ?? ""} />
                    </td>
                    <td className="hint">
                      {row.declared_by ? `${row.declared_by}, ${(row.declared_at ?? "").slice(0, 10)}` : "—"}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <p className="hint" style={{ margin: "10px 0 0" }}>
            Declared intent, not commits. A release clears when the next scan of it stops finding
            the issue &mdash; nothing here is marked done by hand, and a release it has left says
            so whether anybody planned it or not.
          </p>
        </>
      )}
    </div>
  );
}

// Where one release stands, in one word.
function FixState({ state }: { state: string }) {
  const label: Record<string, string> = {
    missed: "Missed",
    fixing: "Fixing",
    undecided: "Not decided",
    clear: "Clear",
    gone: "Gone",
    retired: "Out of support",
  };
  const means: Record<string, string> = {
    missed: "Chosen, scanned since, and the issue is still there",
    fixing: "Chosen, and no scan has looked since",
    undecided: "Nobody has said whether it will be fixed here",
    clear: "Chosen, and the issue is gone",
    gone: "Nobody chose it, and the issue has left anyway",
    retired: "Out of support, so nothing here is a target",
  };
  const cls: Record<string, string> = {
    missed: "lapsed",
    fixing: "waiting",
    undecided: "open",
    clear: "agreed",
    gone: "agreed",
    retired: "open",
  };
  return (
    <span className={`state ${cls[state] ?? "open"}`} title={means[state]}>
      {label[state] ?? state}
    </span>
  );
}

function Holder({
  at,
  assigned,
}: {
  at: { product: string; stream: string; variant: string; vulnerability: string; component: string };
  assigned: string;
}) {
  const queries = useQueryClient();
  const people = useQuery({
    queryKey: ["people"],
    queryFn: async () => unwrap(await api.GET("/v1/people", {})),
  });
  const hand = useMutation({
    mutationFn: async (person: string) =>
      unwrap(
        await api.PUT(
          "/v1/products/{product}/streams/{stream}/variants/{variant}/findings/{vulnerability}/components/{component}/assignment",
          { params: { path: at }, body: { person } },
        ),
      ),
    onSuccess: () => {
      void queries.invalidateQueries({ queryKey: ["finding"] });
      void queries.invalidateQueries({ queryKey: ["findings"] });
      void queries.invalidateQueries({ queryKey: ["unassigned"] });
      void queries.invalidateQueries({ queryKey: ["holdings"] });
    },
  });

  return (
    <div className="card">
      <h3>Who is dealing with it</h3>
      {hand.error != null && <Failed error={hand.error} what="That could not be recorded." />}
      <div style={{ display: "flex", gap: 8, alignItems: "center" }}>
        <select
          value={assigned}
          aria-label="Who is dealing with this"
          disabled={hand.isPending}
          onChange={(event) => hand.mutate(event.target.value)}
        >
          <option value="">Nobody</option>
          {(people.data?.items ?? []).map((each) => (
            <option key={each.identity} value={each.identity}>
              {each.display_name || each.identity}
            </option>
          ))}
        </select>
        {hand.isPending && <span className="hint">Recording…</span>}
      </div>
      <p className="hint" style={{ margin: "8px 0 0" }}>
        Covers every place this sits at, and every build of the product holding the same component
        — the same code built several ways is one piece of work. Handing it back to nobody is the
        same action.
      </p>
    </div>
  );
}
