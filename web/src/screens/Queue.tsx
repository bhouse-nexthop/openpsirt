import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { api } from "../api/client";
import { unwrap } from "../api/queries";
import { useApprove, useSendBack } from "../api/mutations";
import { Empty } from "../ui/Empty";
import { Failed } from "../ui/Failed";
import { Outcome } from "../ui/Outcome";
import { Markdown } from "../ui/Markdown";
import { Editor, forget } from "../ui/Editor";

// What is waiting on somebody. Three things rather than one: a claim awaiting
// agreement, a deferral whose date has passed, and a decision the code moved
// out from under — each is somebody having to look again, and a queue that
// showed only the first would let the other two disappear silently.
export function Queue() {
  const queue = useQuery({
    queryKey: ["queue"],
    queryFn: async () => unwrap(await api.GET("/v1/review-queue", {})),
  });

  if (queue.isPending) return <p className="text-sm text-muted">Loading…</p>;
  if (queue.isError) {
    return <Failed error={queue.error} what="The review queue could not be read." />;
  }

  const items = queue.data?.items ?? [];
  return (
    <div className="max-w-3xl">
      <header className="mb-4 flex items-baseline justify-between gap-2">
        <h1 className="text-lg font-semibold tracking-tight">Waiting on somebody</h1>
        <p className="text-sm text-muted">{queue.data?.total ?? 0}</p>
      </header>

      {items.length === 0 ? (
        <Empty
          title="Nothing is waiting."
          detail="A claim needing a second person, a deferral that has run out, or a decision the code moved out from under would appear here."
        />
      ) : (
        <ul className="flex flex-col gap-3">
          {items.map((row) => (
            <Waiting key={row.decision?.id ?? Math.random()} row={row} />
          ))}
        </ul>
      )}
    </div>
  );
}

type Row = {
  decision?: { id?: number; outcome?: string; state?: string; deferred_until?: string };
  place?: { product?: string; vulnerability?: string; place?: string };
  reasoning?: string;
  proposed_by?: string;
  previously_approved?: boolean;
  deferred_days?: number;
  age_days?: number;
};

function Waiting({ row }: { row: Row }) {
  const id = row.decision?.id;
  const approve = useApprove();
  const sendBack = useSendBack();
  const [asking, setAsking] = useState(false);
  const [because, setBecause] = useState("");
  const draftKey = id ? `send-back:${id}` : undefined;

  const refusal = approve.error ?? sendBack.error;

  return (
    <li className="rounded-lg border border-edge bg-raised p-4">
      <div className="mb-2 flex flex-wrap items-center gap-2">
        <span className="font-medium">{row.place?.vulnerability}</span>
        <Outcome outcome={row.decision?.outcome} />
        {row.previously_approved && (
          <span
            title="This was agreed to before and came back, so what changed is worth reading"
            className="rounded bg-sunken px-1.5 py-0.5 text-xs text-muted ring-1 ring-inset ring-edge"
          >
            agreed before
          </span>
        )}
        <span className="ml-auto flex items-center gap-3 text-sm text-muted">
          {row.place?.product}
          {id && (
            <Link to={`/decisions/${id}`} className="text-accent hover:underline">
              Read it
            </Link>
          )}
        </span>
      </div>

      <p className="mb-2 text-sm text-muted">
        {row.proposed_by && <>claimed by {row.proposed_by}</>}
        {/* An old judgment should look like one. A decision's age travels with
            it everywhere it appears. */}
        {typeof row.age_days === "number" && <> · {ageWord(row.age_days)}</>}
        {typeof row.deferred_days === "number" && row.deferred_days > 0 && (
          <> · put off {row.deferred_days} days in total</>
        )}
      </p>

      {row.reasoning && (
        <div className="mb-3 rounded border border-edge bg-sunken px-3 py-2 text-sm">
          <Markdown source={row.reasoning} />
        </div>
      )}

      {refusal && <Failed error={refusal} what="That could not be recorded." />}

      {asking ? (
        <div className="mt-3">
          <p className="mb-2 text-sm text-muted">
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
          <div className="mt-2 flex gap-2">
            <button
              type="button"
              disabled={!because.trim() || sendBack.isPending}
              onClick={() => {
                if (!id) return;
                sendBack.mutate(
                  { id, because },
                  {
                    onSuccess: () => {
                      // Cleared only once the server has taken it. A failed
                      // submission keeps every word.
                      forget(draftKey);
                      setBecause("");
                      setAsking(false);
                    },
                  },
                );
              }}
              className="rounded bg-accent px-3 py-1.5 text-sm font-medium text-accent-ink disabled:opacity-50"
            >
              Send it back
            </button>
            <button
              type="button"
              onClick={() => setAsking(false)}
              className="rounded border border-edge px-3 py-1.5 text-sm"
            >
              Cancel
            </button>
          </div>
        </div>
      ) : (
        <div className="flex flex-wrap gap-2">
          <button
            type="button"
            disabled={!id || approve.isPending}
            onClick={() => id && approve.mutate({ id })}
            className="rounded bg-accent px-3 py-1.5 text-sm font-medium text-accent-ink disabled:opacity-50"
          >
            Agree
          </button>
          <button
            type="button"
            onClick={() => setAsking(true)}
            className="rounded border border-edge px-3 py-1.5 text-sm"
          >
            Ask for more
          </button>
        </div>
      )}
    </li>
  );
}

function ageWord(days: number): string {
  if (days <= 0) return "today";
  if (days === 1) return "1 day old";
  if (days < 365) return `${days} days old`;
  const years = Math.floor(days / 365);
  return years === 1 ? "over a year old" : `over ${years} years old`;
}
