import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { api } from "../api/client";
import { unwrap } from "../api/queries";
import { useApprove, useSendBack } from "../api/mutations";
import { Empty } from "../ui/Empty";
import { Failed } from "../ui/Failed";
import { Markdown } from "../ui/Markdown";
import { Editor, forget } from "../ui/Editor";

// What is waiting on somebody. Three things rather than one: a claim awaiting
// agreement, a deferral whose date has passed, and a decision the code moved
// out from under. A queue showing only the first lets the other two disappear.
//
// Every card carries the reasoning as it stands, whether this was agreed
// before and came back, and how long the finding has been put off — a list
// where judging a row means opening it is a list that gets approved unread.
export function Queue() {
  const [picked, setPicked] = useState<Set<number>>(new Set());
  const [batch, setBatch] = useState("");
  const approve = useApprove();

  const queue = useQuery({
    queryKey: ["queue"],
    queryFn: async () => unwrap(await api.GET("/v1/review-queue", {})),
  });

  if (queue.isPending) return <p className="hint">Loading…</p>;
  if (queue.isError) {
    return <Failed error={queue.error} what="The review queue could not be read." />;
  }

  const items = queue.data?.items ?? [];
  const ids = items.map((row) => row.decision?.id).filter((id): id is number => !!id);

  async function agreeToPicked() {
    // One name over several, so they can be undone together. Sequential
    // rather than parallel: each is a separate claim and a refusal on one
    // should not decide the fate of the rest.
    for (const id of picked) {
      await approve.mutateAsync({ id, batch: batch.trim() || undefined });
    }
    setPicked(new Set());
  }

  return (
    <>
      <div className="screen-head">
        <h2>Review queue</h2>
        <p>
          {queue.data?.total ?? 0} waiting · across every product you may approve on
        </p>
      </div>

      {items.length > 0 && (
        <div className="batchbar">
          <label style={{ display: "flex", gap: 7, alignItems: "center" }}>
            <input
              type="checkbox"
              checked={picked.size > 0 && picked.size === ids.length}
              onChange={(event) => setPicked(event.target.checked ? new Set(ids) : new Set())}
              aria-label="Select every row shown"
            />
            <b>{picked.size === 0 ? "Nothing selected" : `${picked.size} selected`}</b>
          </label>
          <span className="hint">
            Agreeing to several under one name lets them be undone together.
          </span>
          <span className="grow" />
          <input
            type="text"
            value={batch}
            onChange={(event) => setBatch(event.target.value)}
            placeholder="a name for this batch"
            aria-label="Batch name"
            style={{ width: 150 }}
          />
          <button
            type="button"
            className="btn"
            disabled={picked.size === 0 || approve.isPending}
            onClick={() => void agreeToPicked()}
          >
            {picked.size === 0 ? "Select some first" : `Agree to ${picked.size}`}
          </button>
        </div>
      )}

      {approve.error != null && (
        <Failed error={approve.error} what="That could not be agreed to." />
      )}

      {items.length === 0 ? (
        <Empty
          title="Nothing is waiting."
          detail="A claim needing a second person, a deferral that has run out, or a decision the code moved out from under would appear here."
        />
      ) : (
        <div className="queue">
          {items.map((row) => (
            <Card
              key={row.decision?.id}
              row={row}
              picked={picked.has(row.decision?.id ?? -1)}
              onPick={(on) => {
                const next = new Set(picked);
                const id = row.decision?.id;
                if (!id) return;
                if (on) next.add(id);
                else next.delete(id);
                setPicked(next);
              }}
            />
          ))}
        </div>
      )}
    </>
  );
}

type Row = {
  decision?: { id?: number; outcome?: string; justification?: string; deferred_until?: string };
  place?: { product?: string; vulnerability?: string };
  reasoning?: string;
  proposed_by?: string;
  previously_approved?: boolean;
  deferred_days?: number;
  age_days?: number;
};

function Card({
  row,
  picked,
  onPick,
}: {
  row: Row;
  picked: boolean;
  onPick: (on: boolean) => void;
}) {
  const id = row.decision?.id;
  const approve = useApprove();
  const sendBack = useSendBack();
  const [asking, setAsking] = useState(false);
  const [because, setBecause] = useState("");
  const draftKey = id ? `send-back:${id}` : undefined;

  return (
    <article className={row.previously_approved ? "qcard returning" : "qcard"}>
      <header>
        <input
          type="checkbox"
          className="qpick"
          checked={picked}
          onChange={(event) => onPick(event.target.checked)}
          aria-label="Select this decision"
        />
        <Link to={`/decisions/${id}`} className="linkish id">
          {row.place?.vulnerability}
        </Link>
        <span style={{ color: "var(--muted)" }}>
          {row.decision?.outcome}
          {row.decision?.justification && (
            <> · <span className="mono">{row.decision.justification}</span></>
          )}
          {row.decision?.deferred_until && <> to {row.decision.deferred_until}</>}
        </span>
        <span className="state waiting" style={{ marginLeft: "auto" }}>
          {row.previously_approved ? "Approved before" : "Waiting"}
        </span>
      </header>

      {row.reasoning && (
        <div className="why">
          <Markdown source={row.reasoning} />
        </div>
      )}

      <div className="qmeta">
        {row.proposed_by && <span>Proposed by <b>{row.proposed_by}</b></span>}
        {typeof row.age_days === "number" && (
          <span>Standing <b>{row.age_days === 0 ? "today" : `${row.age_days} days`}</b></span>
        )}
        {row.place?.product && <span>Product <b>{row.place.product}</b></span>}
        {typeof row.deferred_days === "number" && row.deferred_days > 0 && (
          <span>Put off <b>{row.deferred_days} days</b> in total</span>
        )}
      </div>

      {(approve.error ?? sendBack.error) != null && (
        <Failed error={approve.error ?? sendBack.error} what="That could not be recorded." />
      )}

      {asking ? (
        <div>
          <p className="reading" style={{ marginBottom: 8 }}>
            Say what is missing. Sending something back without saying what is missing is a
            round trip nobody learns from.
          </p>
          <Editor
            value={because}
            onChange={setBecause}
            draftKey={draftKey}
            rows={4}
            label="Why this is going back"
            placeholder="What would you need in order to agree?"
          />
          <div className="actions" style={{ marginTop: 8 }}>
            <button
              type="button"
              className="btn"
              disabled={!because.trim() || sendBack.isPending}
              onClick={() =>
                id &&
                sendBack.mutate(
                  { id, because },
                  {
                    onSuccess: () => {
                      forget(draftKey);
                      setBecause("");
                      setAsking(false);
                    },
                  },
                )
              }
            >
              Send it back
            </button>
            <button type="button" className="btn ghost" onClick={() => setAsking(false)}>
              Cancel
            </button>
          </div>
        </div>
      ) : (
        <div className="actions">
          <button
            type="button"
            className="btn"
            disabled={!id || approve.isPending}
            onClick={() => id && approve.mutate({ id })}
          >
            Approve
          </button>
          <button type="button" className="btn ghost" onClick={() => setAsking(true)}>
            Send it back
          </button>
        </div>
      )}
    </article>
  );
}
