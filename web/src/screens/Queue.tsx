import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useSearchParams } from "react-router-dom";
import { api, type Body } from "../api/client";
import { unwrap } from "../api/queries";
import { claimOf, useApproveClaim, useRejectClaim, type Claim } from "../api/claims";
import { Empty } from "../ui/Empty";
import { Failed } from "../ui/Failed";
import { Markdown } from "../ui/Markdown";
import { Editor, forget } from "../ui/Editor";
import { Severity, Exploited } from "../ui/Severity";
import { Paged } from "../ui/Paged";

// AssessmentRow is one claim about how bad an issue is.
type AssessmentRow = Body<"AssessmentBody">;

// A page of claims. The queue is read at the grain of a claim, and a claim
// is a card with its whole argument, so a page is what fits a sitting.
const PAGE = 50;

// The review queue at the grain of a claim (TRI-45): one card per proposer's
// action, however many records it wrote. The approver reads one argument and
// its reach, and approving, rejecting and undoing all work at that size. A
// bulk claim carries its outliers (TRI-46); an extension says what it rests
// on (TRI-47). Lapsed decisions and deferrals that ran out sit underneath:
// nobody has to agree to those again, but each needs a fresh reason.
export function Queue() {
  const [params, setParams] = useSearchParams();
  const offset = Number(params.get("offset") ?? 0);
  // A claim somebody was sent to, by a notice or a link. Found on the page
  // and shown, or said to be missing — a link that lands on the queue with
  // nothing marked leaves somebody hunting through cards for the one meant.
  const wanted = Number(params.get("claim") ?? 0);
  const [picked, setPicked] = useState<Set<string>>(new Set());
  const [batch, setBatch] = useState("");
  const approveClaim = useApproveClaim();

  // Whose claims. The queue proper is what is waiting on you; your own are a
  // different question — what did I propose that nobody has agreed to — and
  // they were mixed in, which made the queue a list containing work the reader
  // cannot do, because approving your own is refused.
  const mine = params.get("mine") === "1";
  const queue = useQuery({
    queryKey: ["queue", offset, mine],
    queryFn: async () =>
      unwrap(
        await api.GET("/v1/review-queue", {
          params: { query: { limit: PAGE, offset, ...(mine ? { mine: true } : {}) } },
        }),
      ),
  });
  // The other side's count, so the tab can carry it without being opened.
  const otherSide = useQuery({
    queryKey: ["queue", "count", !mine],
    queryFn: async () =>
      unwrap(
        await api.GET("/v1/review-queue", {
          params: { query: { limit: 1, ...(mine ? {} : { mine: true }) } },
        }),
      ),
  });

  const found = wanted > 0 && (queue.data?.items ?? []).some((row) => claimOf(row).id === wanted);
  useEffect(() => {
    if (!found) return;
    document.getElementById(`claim-${wanted}`)?.scrollIntoView({ block: "center" });
  }, [found, wanted]);
  const lapsed = useQuery({
    queryKey: ["queue", "lapsed"],
    queryFn: async () =>
      unwrap(await api.GET("/v1/decisions", { params: { query: { state: "lapsed", limit: 50 } } })),
  });
  const expired = useQuery({
    queryKey: ["queue", "expired"],
    queryFn: async () =>
      unwrap(await api.GET("/v1/decisions", { params: { query: { expired: true, limit: 50 } } })),
  });
  // A milder rating of an issue waits for a second person the same way a
  // dismissal does, and there was nowhere to be that second person: the route
  // existed and no screen reached it.
  const ratings = useQuery({
    queryKey: ["queue", "assessments"],
    queryFn: async () =>
      unwrap(await api.GET("/v1/assessments", { params: { query: { state: "proposed", limit: 50 } } })),
  });

  if (queue.isPending) return <p className="hint">Loading…</p>;
  if (queue.isError) {
    return <Failed error={queue.error} what="The review queue could not be read." />;
  }

  const claims = (queue.data?.items ?? []).map(claimOf);
  const records = claims.reduce((sum, c) => sum + c.records, 0);
  const seen = new Set<number>();
  const stopped = [...(lapsed.data?.items ?? []), ...(expired.data?.items ?? [])].filter((row) => {
    const id = row.decision?.id;
    if (!id || seen.has(id)) return false;
    seen.add(id);
    return true;
  });

  async function approvePicked() {
    // Sequential rather than parallel: each is a separate claim and a refusal
    // on one should not decide the fate of the rest.
    for (const claim of claims.filter((c) => picked.has(c.key))) {
      await approveClaim.mutateAsync({ id: claim.id, batch: batch.trim() || undefined });
    }
    setPicked(new Set());
  }

  return (
    <>
      <div className="screen-head">
        <h2>Review queue</h2>
        <p>
          {(queue.data?.total ?? claims.length).toLocaleString()} pending
          {records > claims.length && <> · {records.toLocaleString()} records between those shown</>} ·
          {mine
            ? " proposed by you, waiting for somebody else"
            : " across every product you may approve on"}
        </p>
      </div>

      <div className="tabs2">
        <button
          type="button"
          className="tab2"
          aria-selected={!mine}
          onClick={() => {
            const now = new URLSearchParams(params);
            now.delete("mine");
            now.delete("offset");
            setParams(now);
          }}
        >
          Waiting on me{" "}
          <span className="n">
            {(mine ? (otherSide.data?.total ?? 0) : (queue.data?.total ?? 0)).toLocaleString()}
          </span>
        </button>
        <button
          type="button"
          className="tab2"
          aria-selected={mine}
          onClick={() => {
            const now = new URLSearchParams(params);
            now.set("mine", "1");
            now.delete("offset");
            setParams(now);
          }}
        >
          Mine, pending{" "}
          <span className="n">
            {(mine ? (queue.data?.total ?? 0) : (otherSide.data?.total ?? 0)).toLocaleString()}
          </span>
        </button>
      </div>

      {wanted > 0 && !found && (
        <div className="alert" style={{ marginBottom: 10 }}>
          <strong>Claim {wanted} is not waiting here.</strong>
          <span>
            It may have been decided, or it may sit on another page.{" "}
            <Link to="/unassigned" className="linkish">
              Unassigned →
            </Link>
          </span>
        </div>
      )}

      {claims.length > 0 && (
        <div className="batchbar">
          <label style={{ display: "flex", gap: 7, alignItems: "center" }}>
            <input
              type="checkbox"
              checked={picked.size > 0 && picked.size === claims.length}
              onChange={(event) =>
                setPicked(event.target.checked ? new Set(claims.map((c) => c.key)) : new Set())
              }
              aria-label="Select every claim shown"
            />
            <b>{picked.size === 0 ? "Nothing selected" : `${picked.size} selected`}</b>
          </label>
          <span className="hint">Approvals under one batch name can be undone together.</span>
          <span className="spacer" />
          <input
            type="text"
            value={batch}
            onChange={(event) => setBatch(event.target.value)}
            placeholder="batch name"
            aria-label="Batch name"
            style={{ width: 150 }}
          />
          <button
            type="button"
            className="btn"
            disabled={picked.size === 0 || approveClaim.isPending}
            onClick={() => void approvePicked()}
          >
            {picked.size === 0 ? "Approve selected" : `Approve ${picked.size} selected`}
          </button>
        </div>
      )}

      {approveClaim.error != null && <Failed error={approveClaim.error} what="That could not be approved." />}

      {claims.length === 0 ? (
        <Empty
          title="Nothing is pending."
          detail="A claim needing a second person would appear here."
        />
      ) : (
        <div className="queue">
          {claims.map((claim) => (
            <Card
              key={claim.key}
              claim={claim}
              marked={claim.id === wanted}
              picked={picked.has(claim.key)}
              onPick={(on) => {
                const next = new Set(picked);
                if (on) next.add(claim.key);
                else next.delete(claim.key);
                setPicked(next);
              }}
            />
          ))}
        </div>
      )}
      <Paged
        shown={claims.length}
        total={queue.data?.total}
        offset={offset}
        limit={PAGE}
        onGo={(next) => {
          const now = new URLSearchParams(params);
          if (next === 0) now.delete("offset");
          else now.set("offset", String(next));
          setParams(now);
        }}
      />

      <Ratings waiting={(ratings.data?.items ?? []).filter((each) => each.needs_approval)} />

      <div className="screen-head" id="lapsed" style={{ marginTop: 22 }}>
        <h2>Lapsed decisions</h2>
        <p>
          {((lapsed.data?.total ?? 0) + (expired.data?.total ?? 0)).toLocaleString()} · nobody has to
          agree to these again — two people already did — but each needs a fresh reason, because what
          it was a claim about has moved.
        </p>
      </div>
      {/* Two lists of fifty, merged. Not paged: a decision can be in both,
          so pages of the two do not add up — but a full list is still said
          to be one. */}
      <Paged
        shown={Math.max(lapsed.data?.items?.length ?? 0, expired.data?.items?.length ?? 0)}
        limit={50}
        what="shown of each kind"
      />
      {stopped.length === 0 ? (
        <Empty title="Nothing has lapsed." detail="A decision the code moved out from under, or a deferral whose date has passed, would appear here." />
      ) : (
        <div className="queue">
          {stopped.map((row) => (
            <Stopped key={row.decision?.id} row={row} />
          ))}
        </div>
      )}
    </>
  );
}

type Standing = NonNullable<Body<"DecisionsOutputBody">["items"]>[number];

// A decision the code moved out from under, or a deferral whose date passed.
// It links to the finding rather than offering the judgment here: reaffirming
// is a claim about one place in one build, and the row does not carry the
// build, so the finding is where its locations are.
function Stopped({ row }: { row: Standing }) {
  const it = row.decision;
  const lapsed = it?.state === "lapsed";
  return (
    <article className="qcard lapsedcard">
      <header>
        <Link to={`/decisions/${it?.id}`} className="id linkish">
          {row.place?.vulnerability}
        </Link>
        <span style={{ color: "var(--muted)" }}>
          {row.place?.product} · {OUTCOME[it?.outcome ?? ""] ?? it?.outcome}
          {it?.justification && (
            <>
              {" "}
              · <span className="mono">{it.justification}</span>
            </>
          )}
        </span>
        <span className="state lapsed" style={{ marginLeft: "auto" }}>
          {lapsed ? "Lapsed" : "Deferral ran out"}
        </span>
      </header>
      {row.reasoning && (
        <div className="why">
          <Markdown source={row.reasoning} />
        </div>
      )}
      <div className="qmeta">
        <span>
          Proposed by <b>{row.proposed_by}</b>
        </span>
        <span>
          Stood <b>{row.age_days} days</b>
        </span>
        {it?.deferred_until && (
          <span>
            Put off until <b>{it.deferred_until}</b>
          </span>
        )}
      </div>
      <p className="hint" style={{ margin: 0 }}>
        {lapsed
          ? "The code moved out from under this judgment. Reaffirm it from the finding, with a fresh reason; no second person is needed."
          : "The date this was put off until has passed, so it is open again."}
      </p>
      <div className="actions">
        <Link to={`/decisions/${it?.id}`} className="btn ghost">
          Open the decision →
        </Link>
      </div>
    </article>
  );
}

const OUTCOME: Record<string, string> = {
  affected: "Affected",
  "not-applicable": "Not applicable",
  deferred: "Deferred",
  "wont-fix": "Won't fix",
};

function Card({
  claim,
  marked,
  picked,
  onPick,
}: {
  claim: Claim;
  // Whether somebody was sent to this claim in particular.
  marked: boolean;
  picked: boolean;
  onPick: (on: boolean) => void;
}) {
  const approveClaim = useApproveClaim();
  const rejectClaim = useRejectClaim();
  const [asking, setAsking] = useState(false);
  const [more, setMore] = useState(false);
  const f = claim.finding;
  const [because, setBecause] = useState("");
  const [aside, setAside] = useState<Set<number>>(new Set());
  const draftKey = `send-back:${claim.key}`;
  const bulk = claim.kind === "together";
  const extension = claim.kind === "extension";
  const busy = approveClaim.isPending || rejectClaim.isPending;
  const error = approveClaim.error ?? rejectClaim.error;

  function doApprove() {
    approveClaim.mutate({
      id: claim.id,
      ...(aside.size > 0
        ? { except: [...aside], because: "Set aside at approval: these do not match the shape of the claim." }
        : {}),
    });
  }

  function doReject() {
    rejectClaim.mutate(
      { id: claim.id, because },
      {
        onSuccess: () => {
          forget(draftKey);
          setBecause("");
          setAsking(false);
        },
      },
    );
  }

  return (
    <article
      id={`claim-${claim.id}`}
      className={`qcard${claim.previouslyApproved ? " returning" : ""}${bulk ? " bulkclaim" : ""}${extension ? " extension" : ""}`}
      style={marked ? { outline: "2px solid var(--accent)", outlineOffset: 2 } : undefined}
    >
      <header>
        <input type="checkbox" className="qpick" checked={picked} onChange={(event) => onPick(event.target.checked)} aria-label="Select this claim" />
        {bulk && <span className="bulkmark">Bulk claim</span>}
        {f?.severity && <Severity word={f.severity} />}
        {f?.exploited && <Exploited when />}
        {/* The issue opens its finding, which carries everything an approver
            could want; the decision record is one link further, from there. */}
        <Link to={f ? findingPath(f) : `/decisions/${claim.decisionId}`} className="linkish id">
          {claim.title}
        </Link>
        {/* What is being claimed, in its own right rather than as the fifth
            clause of a sentence about where the finding lives. It is the thing
            an approver is agreeing to, and it was reading as an afterthought
            behind the component, the version, the product, the branch and the
            variant. */}
        <Outcome
          outcome={claim.outcome}
          justification={claim.justification}
          until={claim.deferredUntil}
        />
        <span style={{ color: "var(--muted)" }}>
          {f?.component && (
            <>
              in <span className="id">{f.component}</span>
              {f.version && <span className="id" style={{ color: "var(--faint)" }}> {f.version}</span>} ·{" "}
            </>
          )}
          {f ? `${f.product} · ${f.stream} · ${f.variant}` : claim.product}
          {bulk && claim.issues > 1 && <> · {claim.issues.toLocaleString()} issues</>}
          {extension && claim.derivedFrom && (
            <>
              {" "}
              · extends <b>#{claim.derivedFrom}</b>
            </>
          )}
        </span>
        <span className="state waiting" style={{ marginLeft: "auto" }}>
          {extension ? "Extension" : claim.previouslyApproved ? "Approved before" : "Pending"}
        </span>
      </header>

      {/* What the issue is and where it sits, before the argument about it
          (TRI-09): an approver judging a claim without these is judging the
          prose. Shown from the scan's own text, escaped, never rendered. */}
      {f && (f.description || f.owner || f.parent) && (
        <div className="about">
          {f.description && (
            <p className={more ? "" : "clamp"} style={{ margin: 0 }}>
              {f.description}
              {f.description.length >= 400 && !more && "…"}
            </p>
          )}
          <p className="hint" style={{ margin: "4px 0 0", display: "flex", flexWrap: "wrap", gap: "4px 12px" }}>
            {(f.owner || f.parent) && (
              <span>
                <span className="id">{f.owner}</span>
                {f.parent && f.parent !== f.owner && <> › <span className="id">{f.parent}</span></>}
              </span>
            )}
            {typeof f.places === "number" && (
              <span>
                {f.places} {f.places === 1 ? "location" : "locations"}
                {typeof f.decided === "number" && f.decided > 0 && <> · {f.decided} covered by this claim</>}
              </span>
            )}
            {f.fixed_in ? <span>fixed in <span className="id">{f.fixed_in}</span></span> : f.fix_state === "wont-fix" ? <span>upstream declined</span> : null}
            {typeof f.score === "number" && <span>CVSS {f.score.toFixed(1)}</span>}
            {f.description && f.description.length >= 200 && (
              <button type="button" className="linkish" style={{ fontWeight: 500 }} onClick={() => setMore(!more)}>
                {more ? "less" : "more"}
              </button>
            )}
          </p>
        </div>
      )}

      {claim.reasoning && (
        <div className="why">
          <Markdown source={claim.reasoning} />
        </div>
      )}

      <div className="qmeta">
        {claim.proposedBy && (
          <span>
            Proposed by <b>{claim.proposedBy}</b>
          </span>
        )}
        <span>
          Standing <b>{claim.ageDays === 0 ? "today" : `${claim.ageDays} days`}</b>
        </span>
        {claim.deferredDays > 0 && (
          <span>
            Put off <b>{claim.deferredDays} days</b> in total
          </span>
        )}
        {claim.selectedBy && (
          <span>
            Narrowed by <b className="mono">{claim.selectedBy}</b>
          </span>
        )}
        {extension && claim.derivedFrom && (
          <span>
            Rests on <b>#{claim.derivedFrom}</b>
          </span>
        )}
        <span>
          Writes <b>{claim.records.toLocaleString()} {claim.records === 1 ? "record" : "records"}</b>
        </span>
      </div>

      {!bulk && (
        <div className="reachrow">
          <span className="r auto">
            <b>{claim.places}</b> {claim.places === 1 ? "location" : "locations"} in this build
          </span>
          {claim.builds.length > 0 && (
            <span className="r ask">
              <b>{claim.builds.length}</b> other {claim.builds.length === 1 ? "build" : "builds"}: {claim.builds.join(", ")}
            </span>
          )}
          <span className="hint">One approval covers every record; undo reverts them all</span>
        </div>
      )}

      {bulk && claim.outliers && (
        <div className="outliers">
          <header>
            <h5>Outliers in this set</h5>
            <span className="hint">Rows that do not match the shape of the claim.</span>
          </header>
          <div className="ostats">
            <span className={`o${claim.outliers.exploited ? " bad" : ""}`}>
              <b>{claim.outliers.exploited}</b> known exploited
            </span>
            <span className={`o${claim.outliers.severe ? " warn" : ""}`}>
              <b>{claim.outliers.severe}</b> critical or high
            </span>
            <span className="o">
              <b>{claim.outliers.fixable}</b> have a fix available
            </span>
            <span className={`o${claim.outliers.unmatched ? " warn" : ""}`}>
              <b>{claim.outliers.unmatched}</b> do not match the narrowing
            </span>
          </div>
          {(claim.outliers.rows ?? []).length > 0 && (
            <div className="tablewrap" style={{ boxShadow: "none" }}>
              <table>
                <thead>
                  <tr>
                    <th style={{ width: 30 }} />
                    <th>Severity</th>
                    <th>Issue</th>
                    <th>Description</th>
                    <th>Reason</th>
                  </tr>
                </thead>
                <tbody>
                  {(claim.outliers.rows ?? []).map((row) => (
                    <tr key={row.decision_id}>
                      <td>
                        <input
                          type="checkbox"
                          aria-label="Set aside"
                          checked={aside.has(row.decision_id)}
                          onChange={(event) => {
                            const next = new Set(aside);
                            if (event.target.checked) next.add(row.decision_id);
                            else next.delete(row.decision_id);
                            setAside(next);
                          }}
                        />
                      </td>
                      <td>
                        <Severity word={row.severity} />
                      </td>
                      <td>
                        <span className="id">{row.vulnerability}</span> <Exploited when={row.exploited} />
                      </td>
                      <td className="hint">{(row.description ?? "").slice(0, 120)}</td>
                      <td className="hint">{(row.why ?? []).join(", ")}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
          <p className="hint" style={{ margin: "8px 0 0" }}>
            Selected rows are excluded from the approval and rejected back to {claim.proposedBy} as a
            separate item, with this table as the reason.
          </p>
        </div>
      )}
      {bulk && !claim.outliers && (
        <p className="hint" style={{ margin: 0 }}>
          One claim over {claim.issues.toLocaleString()} issues, read once and approved once.
        </p>
      )}

      {claim.deferredDays > 0 && claim.previouslyApproved && (
        <p style={{ margin: 0, fontSize: "var(--step--1)", color: "var(--sev-high)" }}>
          Short is measured against everything this has already been put off for, not against the days
          being asked.
        </p>
      )}

      {error != null && <Failed error={error} what="That could not be recorded." />}

      {asking ? (
        <div>
          {/* No attach control: a rejection is about what is missing from the
              reasoning, and a claim may cover many issues, so there is not one
              a file would hang off. */}
          <Editor
            value={because}
            onChange={setBecause}
            draftKey={draftKey}
            rows={4}
            label="Reason for rejection"
            placeholder="What is missing or wrong."
          />
          <div className="actions" style={{ marginTop: 8 }}>
            <button type="button" className="btn" disabled={!because.trim() || busy} onClick={doReject}>
              Reject
            </button>
            <button type="button" className="btn quiet" onClick={() => setAsking(false)}>
              Cancel
            </button>
            <span className="consequence">
              Rejected, back to <b>{claim.proposedBy}</b>
            </span>
          </div>
        </div>
      ) : (
        <div className="actions">
          <button type="button" className="btn" disabled={busy} onClick={doApprove}>
            {bulk && aside.size > 0
              ? `Approve ${(claim.issues - aside.size).toLocaleString()}, reject ${aside.size}`
              : bulk
                ? `Approve all ${claim.issues.toLocaleString()}`
                : "Approve"}
          </button>
          <button type="button" className="btn ghost" onClick={() => setAsking(true)}>
            {bulk ? "Reject all" : "Reject"}
          </button>
          {bulk && (
            <span className="consequence">
              <b>{claim.records.toLocaleString()}</b> records, each expiring independently
            </span>
          )}
        </div>
      )}
    </article>
  );
}

// Where a claim's finding lives, with the version the build ships it at.
function findingPath(f: { product?: string; stream?: string; variant?: string; vulnerability?: string; component?: string; version?: string }): string {
  return (
    `/products/${encodeURIComponent(f.product ?? "")}` +
    `/streams/${encodeURIComponent(f.stream ?? "")}` +
    `/variants/${encodeURIComponent(f.variant ?? "")}` +
    `/findings/${encodeURIComponent(f.vulnerability ?? "")}` +
    `/components/${encodeURIComponent(f.component ?? "")}` +
    (f.version ? `?version=${encodeURIComponent(f.version)}` : "")
  );
}

// Ratings of issues waiting for a second person.
//
// A milder rating hides things, so it waits the way a dismissal does — and
// there was nowhere to be that second person, because the route existed and no
// screen reached it.
//
// **What it says beyond "agree or not" is the point.** Rating something milder
// pushes its deadline out, which is what the second person is there for. But
// where a product has said what it considers worth triaging at all, a rating
// that crosses that line does something different in kind: the findings stop
// being work rather than becoming later work, and they carry no deadline at
// all. Those are two different things to agree to, and an approver was shown
// neither.
function Ratings({ waiting }: { waiting: AssessmentRow[] }) {
  const queries = useQueryClient();
  const agree = useMutation({
    mutationFn: async (id: number) =>
      unwrap(await api.POST("/v1/assessments/{id}/agreement", { params: { path: { id } } })),
    onSuccess: () => void queries.invalidateQueries({ queryKey: ["queue"] }),
  });

  if (waiting.length === 0) return null;

  return (
    <>
      <div className="screen-head" id="ratings" style={{ marginTop: 22 }}>
        <h2>Ratings waiting</h2>
        <p>
          {waiting.length.toLocaleString()} · somebody says an issue is milder than the world does.
          A rating of ours holds wherever the issue appears, so it waits for a second person.
        </p>
      </div>
      {agree.error != null && <Failed error={agree.error} what="That could not be agreed to." />}
      <div className="queue">
        {waiting.map((row) => (
          <div className="card" key={row.id}>
            <div className="cardhead">
              <span className="id">{row.vulnerability}</span>
              <span>
                <Severity word={row.published ?? ""} /> → <Severity word={row.severity ?? ""} />
              </span>
            </div>
            <p className="reading">{row.reasoning}</p>
            <p className="hint">
              {(row.open ?? 0).toLocaleString()} open{" "}
              {(row.open ?? 0) === 1 ? "finding" : "findings"} you can see, in{" "}
              {(row.in_products ?? 0).toLocaleString()}{" "}
              {(row.in_products ?? 0) === 1 ? "product" : "products"}.
            </p>
            {(row.off_the_list ?? 0) > 0 ? (
              <p className="alert" style={{ margin: "6px 0 0" }}>
                <strong>
                  This takes {(row.off_the_list ?? 0).toLocaleString()} of them off the working list
                  in {(row.off_the_list_in_products ?? 0).toLocaleString()}{" "}
                  {(row.off_the_list_in_products ?? 0) === 1 ? "product" : "products"}.
                </strong>
                <span>
                  Below what a product considers worth triaging, a finding is still recorded,
                  counted and reportable — and it carries no deadline. You are agreeing that it is
                  not work, rather than that it is later work.
                </span>
              </p>
            ) : (
              <p className="hint" style={{ margin: "6px 0 0" }}>
                Still above what every product here triages from, so this makes them later work
                rather than no work.
              </p>
            )}
            <div className="cardfoot">
              <button
                type="button"
                className="btn"
                disabled={agree.isPending}
                onClick={() => agree.mutate(row.id ?? 0)}
              >
                Agree
              </button>
            </div>
          </div>
        ))}
      </div>
    </>
  );
}

// What is being claimed, said as its own thing.
//
// The outcome is what an approver is agreeing to, and it was the fifth clause
// of a line that led with the component, the version, the product, the branch
// and the variant — so the most important fact on the card read as an
// afterthought. It leads now, in its own mark, with the recognized reason
// beneath it in the vocabulary the record actually stores.
function Outcome({
  outcome,
  justification,
  until,
}: {
  outcome: string;
  justification?: string;
  until?: string;
}) {
  return (
    <span className={`claimed ${outcome}`}>
      <b>{OUTCOME[outcome] ?? outcome}</b>
      {justification && <span className="why mono">{justification}</span>}
      {until && <span className="why">until {until}</span>}
    </span>
  );
}
