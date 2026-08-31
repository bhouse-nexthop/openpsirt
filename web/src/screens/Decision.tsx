import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useParams } from "react-router-dom";
import { api } from "../api/client";
import { unwrap } from "../api/queries";
import { useComment, useRevise, useWithdraw } from "../api/mutations";
import { Failed } from "../ui/Failed";
import { Outcome } from "../ui/Outcome";
import { State } from "../ui/State";
import { Markdown } from "../ui/Markdown";
import { Editor, forget } from "../ui/Editor";
import type { Who } from "../app/session";

// One claim, everything about it, and what can be done to it. The revision
// history is here rather than hidden behind another request because an
// approval names one revision — so what an approver actually saw is a
// different question from what the text says now, and both get asked.
export function Decision({ who }: { who: Who }) {
  const { id: raw = "" } = useParams();
  const id = Number(raw);

  const decision = useQuery({
    queryKey: ["decision", id],
    queryFn: async () => unwrap(await api.GET("/v1/decisions/{id}", { params: { path: { id } } })),
  });

  if (decision.isPending) return <p className="text-sm text-muted">Loading…</p>;
  if (decision.isError) {
    return <Failed error={decision.error} what="That decision could not be read." />;
  }
  const it = decision.data;
  if (!it) return null;

  const mine = it.proposed_by === who.identity || it.proposed_by === who.name;

  return (
    <div className="max-w-3xl">
      <header className="mb-4">
        <div className="mb-2 flex flex-wrap items-center gap-2">
          <h1 className="text-xl font-semibold tracking-tight">{it.place?.vulnerability}</h1>
          <Outcome outcome={it.decision?.outcome} />
          <State state={it.decision?.state} />
        </div>
        <p className="text-sm text-muted">
          {it.place?.product}
          {it.proposed_by && <> · claimed by {it.proposed_by}</>}
          {it.proposed_at && <> on {it.proposed_at.slice(0, 10)}</>}
          {typeof it.age_days === "number" && it.age_days > 365 && (
            <> · <span className="text-medium">a judgment this old is worth re-reading</span></>
          )}
        </p>
        {it.decision?.sent_back_at && (
          <p className="mt-2 rounded border border-medium/40 bg-medium/8 px-3 py-2 text-sm">
            An approver asked for more on {it.decision.sent_back_at.slice(0, 10)}. It is back with
            whoever wrote it, and out of the review queue until they revise it.
          </p>
        )}
        {it.decision?.selected_by && (
          <p className="mt-2 text-sm text-muted">
            One of many recorded together. Narrowed by: {it.decision.selected_by}
          </p>
        )}
      </header>

      <Reasoning
        id={id}
        current={it.reasoning ?? ""}
        mine={mine}
        state={it.decision?.state}
      />

      <Revisions id={id} />
      <Approvals id={id} />
      <Comments id={id} />
    </div>
  );
}

// Revising is how a disagreement is expressed: it keeps the old words
// readable, takes back the approval given for them, and returns the claim to
// the queue. Only whoever wrote the current words may do it.
function Reasoning({
  id,
  current,
  mine,
  state,
}: {
  id: number;
  current: string;
  mine: boolean;
  state?: string;
}) {
  const [editing, setEditing] = useState(false);
  const [text, setText] = useState(current);
  const revise = useRevise();
  const withdraw = useWithdraw();
  const draftKey = `revise:${id}`;
  const live = state === "proposed" || state === "approved";

  return (
    <section className="mb-6">
      <div className="mb-2 flex items-center gap-2">
        <h2 className="text-sm font-semibold">Why</h2>
        {mine && live && !editing && (
          <button
            type="button"
            onClick={() => {
              setText(current);
              setEditing(true);
            }}
            className="text-sm text-accent hover:underline"
          >
            Revise
          </button>
        )}
      </div>

      {editing ? (
        <>
          <p className="mb-2 text-sm text-muted">
            Revising takes back any approval this has and returns it to the queue, marked as
            agreed before. The old words stay readable.
          </p>
          <Editor value={text} onChange={setText} draftKey={draftKey} label="Reasoning" />
          {revise.error != null && <Failed error={revise.error} what="That could not be stored." />}
          <div className="mt-2 flex gap-2">
            <button
              type="button"
              disabled={!text.trim() || revise.isPending}
              onClick={() =>
                revise.mutate(
                  { id, reasoning: text },
                  {
                    onSuccess: () => {
                      forget(draftKey);
                      setEditing(false);
                    },
                  },
                )
              }
              className="rounded bg-accent px-3 py-1.5 text-sm font-medium text-accent-ink disabled:opacity-50"
            >
              Save
            </button>
            <button
              type="button"
              onClick={() => setEditing(false)}
              className="rounded border border-edge px-3 py-1.5 text-sm"
            >
              Cancel
            </button>
          </div>
        </>
      ) : (
        <div className="rounded-lg border border-edge bg-raised p-3 text-sm">
          {current ? <Markdown source={current} /> : <p className="text-muted">Nothing written.</p>}
        </div>
      )}

      {mine && live && !editing && (
        <div className="mt-3">
          {withdraw.error != null && (
            <Failed error={withdraw.error} what="That could not be withdrawn." />
          )}
          <button
            type="button"
            disabled={withdraw.isPending}
            onClick={() => withdraw.mutate({ id })}
            className="text-sm text-muted hover:text-critical"
          >
            Withdraw this claim
          </button>
          <span className="ml-2 text-sm text-muted">
            It stops applying and stays on the record.
          </span>
        </div>
      )}
    </section>
  );
}

// What an approver actually saw. An approval points at one revision, so the
// text as it stands now is not necessarily the text anybody agreed to.
function Revisions({ id }: { id: number }) {
  const revisions = useQuery({
    queryKey: ["decision", id, "revisions"],
    queryFn: async () =>
      unwrap(await api.GET("/v1/decisions/{id}/revisions", { params: { path: { id } } })),
  });

  const items = revisions.data?.items ?? [];
  if (items.length < 2) return null;

  return (
    <section className="mb-6">
      <h2 className="mb-2 text-sm font-semibold">
        How the reasoning changed
        <span className="ml-2 font-normal text-muted">{items.length} versions</span>
      </h2>
      <ul className="flex flex-col gap-2">
        {items.map((revision) => (
          <li key={revision.id} className="rounded-lg border border-edge bg-sunken p-3 text-sm">
            <p className="mb-1 text-muted">
              #{revision.ordinal} · {revision.written_by}
              {revision.written_at && <> on {revision.written_at.slice(0, 10)}</>}
            </p>
            <Markdown source={revision.body ?? ""} />
          </li>
        ))}
      </ul>
    </section>
  );
}

// Who agreed, to which words, and how much it covered when they did. A
// withdrawn approval is kept because who agreed to what is part of the record.
function Approvals({ id }: { id: number }) {
  const approvals = useQuery({
    queryKey: ["decision", id, "approvals"],
    queryFn: async () =>
      unwrap(await api.GET("/v1/decisions/{id}/approvals", { params: { path: { id } } })),
  });

  const items = approvals.data?.items ?? [];
  if (items.length === 0) return null;

  return (
    <section className="mb-6">
      <h2 className="mb-2 text-sm font-semibold">Who agreed</h2>
      <ul className="flex flex-col gap-2 text-sm">
        {items.map((approval) => (
          <li
            key={approval.id}
            className={`rounded-lg border border-edge p-3 ${
              approval.withdrawn_at ? "bg-sunken text-muted" : "bg-raised"
            }`}
          >
            <span>{approval.approved_by}</span>
            {approval.approved_at && <span className="text-muted"> on {approval.approved_at.slice(0, 10)}</span>}
            {typeof approval.covered === "number" && (
              <span
                className="text-muted"
                title="How much it covered when it was agreed to. A decision reaches by matching, so it covers more as builds appear — with nobody having acted"
              >
                {" "}· covered {approval.covered} then
              </span>
            )}
            {approval.withdrawn_at && (
              <span> · taken back on {approval.withdrawn_at.slice(0, 10)}</span>
            )}
          </li>
        ))}
      </ul>
    </section>
  );
}

// Comments are separate from the reasoning and never affect an approval, so an
// approved claim can be annotated at any time without disturbing it.
function Comments({ id }: { id: number }) {
  const [text, setText] = useState("");
  const comment = useComment();
  const draftKey = `comment:${id}`;

  const comments = useQuery({
    queryKey: ["comments", id],
    queryFn: async () =>
      unwrap(await api.GET("/v1/decisions/{id}/comments", { params: { path: { id } } })),
  });

  const items = comments.data?.items ?? [];

  return (
    <section>
      <h2 className="mb-2 text-sm font-semibold">Comments</h2>
      <p className="mb-3 text-sm text-muted">
        Separate from the reasoning, and they never affect an approval.
      </p>

      {items.length > 0 && (
        <ul className="mb-4 flex flex-col gap-2">
          {items.map((each) => (
            <li key={each.id} className="rounded-lg border border-edge bg-raised p-3 text-sm">
              <p className="mb-1 text-muted">
                {each.written_by}
                {each.written_at && <> on {each.written_at.slice(0, 10)}</>}
                {each.edited_at && <> · edited</>}
              </p>
              <Markdown source={each.body ?? ""} />
            </li>
          ))}
        </ul>
      )}

      <Editor
        value={text}
        onChange={setText}
        draftKey={draftKey}
        rows={4}
        label="Comment"
        placeholder="Add a note"
      />
      {comment.error != null && <Failed error={comment.error} what="That could not be added." />}
      <button
        type="button"
        disabled={!text.trim() || comment.isPending}
        onClick={() =>
          comment.mutate(
            { id, body: text },
            {
              onSuccess: () => {
                forget(draftKey);
                setText("");
              },
            },
          )
        }
        className="mt-2 rounded bg-accent px-3 py-1.5 text-sm font-medium text-accent-ink disabled:opacity-50"
      >
        Comment
      </button>
    </section>
  );
}
