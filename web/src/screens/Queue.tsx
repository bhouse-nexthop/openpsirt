import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { api, type Body } from "../api/client";
import { unwrap } from "../api/queries";
import { claimOf, useApproveClaim, useRejectClaim, type Claim } from "../api/claims";
import { Empty } from "../ui/Empty";
import { Failed } from "../ui/Failed";
import { Markdown } from "../ui/Markdown";
import { Editor, forget } from "../ui/Editor";
import { Severity, Exploited } from "../ui/Severity";

// The review queue at the grain of a claim (TRI-45): one card per proposer's
// action, however many records it wrote. The approver reads one argument and
// its reach, and approving, rejecting and undoing all work at that size. A
// bulk claim carries its outliers (TRI-46); an extension says what it rests
// on (TRI-47). Lapsed decisions and deferrals that ran out sit underneath:
// nobody has to agree to those again, but each needs a fresh reason.
export function Queue() {
  const [picked, setPicked] = useState<Set<string>>(new Set());
  const [batch, setBatch] = useState("");
  const approveClaim = useApproveClaim();

  const queue = useQuery({
    queryKey: ["queue"],
    queryFn: async () => unwrap(await api.GET("/v1/review-queue", {})),
  });
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
          {claims.length.toLocaleString()} pending
          {records > claims.length && <> · {records.toLocaleString()} records between them</>} · across
          every product you may approve on
        </p>
      </div>

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
          <span className="grow" />
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

      <div className="screen-head" id="lapsed" style={{ marginTop: 22 }}>
        <h2>Lapsed decisions</h2>
        <p>
          {stopped.length.toLocaleString()} · nobody has to agree to these again — two people already
          did — but each needs a fresh reason, because what it was a claim about has moved.
        </p>
      </div>
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

function Card({ claim, picked, onPick }: { claim: Claim; picked: boolean; onPick: (on: boolean) => void }) {
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
    <article className={`qcard${claim.previouslyApproved ? " returning" : ""}${bulk ? " bulkclaim" : ""}${extension ? " extension" : ""}`}>
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
        <span style={{ color: "var(--muted)" }}>
          {f?.component && (
            <>
              in <span className="id">{f.component}</span>
              {f.version && <span className="id" style={{ color: "var(--faint)" }}> {f.version}</span>} ·{" "}
            </>
          )}
          {f ? `${f.product} · ${f.stream} · ${f.variant}` : claim.product}
          {bulk && claim.issues > 1 && <> · {claim.issues.toLocaleString()} issues</>} ·{" "}
          {OUTCOME[claim.outcome] ?? claim.outcome}
          {claim.justification && (
            <>
              {" "}
              · <span className="mono">{claim.justification}</span>
            </>
          )}
          {claim.deferredUntil && <> to {claim.deferredUntil}</>}
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
            Selected rows are excluded from the approval and returned to {claim.proposedBy} as a separate
            item, with this table as the reason.
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
              Returns to <b>{claim.proposedBy}</b>
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
