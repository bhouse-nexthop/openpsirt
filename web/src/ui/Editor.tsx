import { useEffect, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import { unwrap } from "../api/queries";
import { Markdown } from "./Markdown";

// Write and Preview over a plain textarea, with a formatting toolbar. Not a
// rich-text editor: what is stored is markdown, and an editor that hides that
// eventually disagrees with what gets published.
//
// Preview goes through the same renderer as published text, so the two cannot
// disagree — there is no second implementation to drift.

type Mark = { label: string; title: string; wrap: [string, string] };

// Enough to write a justification with, and no more. Each has to be reachable
// on a phone, so they are buttons rather than keyboard shortcuts.
const MARKS: Mark[] = [
  { label: "B", title: "Bold", wrap: ["**", "**"] },
  { label: "I", title: "Italic", wrap: ["_", "_"] },
  { label: "`", title: "Code", wrap: ["`", "`"] },
  { label: "{ }", title: "Code block", wrap: ["```\n", "\n```"] },
  { label: "•", title: "List item", wrap: ["- ", ""] },
  { label: "❝", title: "Quote", wrap: ["> ", ""] },
  { label: "🔗", title: "Link", wrap: ["[", "](https://)"] },
];

export function Editor({
  value,
  onChange,
  draftKey,
  rows = 8,
  placeholder,
  label,
  mentions,
}: {
  value: string;
  onChange: (next: string) => void;
  // Where an unsent draft is kept. Text somebody typed is theirs, and losing
  // it to a failed request, an expired session or a closed tab is the thing
  // that teaches people to write less.
  draftKey?: string;
  rows?: number;
  placeholder?: string;
  label?: string;
  // Where mentions may be offered from. Omitted where there is no product in
  // hand, in which case nothing is offered rather than everybody.
  mentions?: { product: string; visibility?: "public" | "private" };
}) {
  const [showing, setShowing] = useState<"write" | "preview">("write");
  const [typing, setTyping] = useState<string | null>(null);
  const box = useRef<HTMLTextAreaElement>(null);

  // Only people who can already read what this text is about. An autocomplete
  // listing everybody teaches somebody to name a colleague who then cannot
  // open what they were called to — and on an undisclosed finding the mention
  // itself would say a finding exists.
  const offerable = useQuery({
    queryKey: ["mentionable", mentions?.product, mentions?.visibility],
    enabled: typing !== null && !!mentions?.product,
    staleTime: 5 * 60_000,
    queryFn: async () =>
      unwrap(
        await api.GET("/v1/products/{product}/mentionable", {
          params: {
            path: { product: mentions?.product ?? "" },
            query: { visibility: mentions?.visibility ?? "public", limit: 50 },
          },
        }),
      ),
  });

  const candidates = (offerable.data?.items ?? []).filter((each) =>
    typing ? (each.identity ?? "").toLowerCase().startsWith(typing.toLowerCase()) : true,
  );

  // What is being typed after an @, if anything. Read from the text before the
  // cursor rather than tracked as state, so it stays right however somebody
  // edits — pasting, deleting, clicking elsewhere in the line.
  function partial(field: HTMLTextAreaElement): string | null {
    const upto = field.value.slice(0, field.selectionStart);
    const match = /(?:^|\s)@([A-Za-z0-9._-]*)$/.exec(upto);
    return match ? (match[1] ?? "") : null;
  }

  function complete(identity: string) {
    const field = box.current;
    if (!field) return;
    const upto = value.slice(0, field.selectionStart);
    const start = upto.lastIndexOf("@");
    if (start < 0) return;
    const next = value.slice(0, start) + "@" + identity + " " + value.slice(field.selectionStart);
    onChange(next);
    setTyping(null);
    queueMicrotask(() => {
      field.focus();
      const at = start + identity.length + 2;
      field.setSelectionRange(at, at);
    });
  }

  // Restore whatever was left behind, once, and only over an empty field —
  // a draft must never overwrite something the caller supplied.
  const restored = useRef(false);
  useEffect(() => {
    if (restored.current || !draftKey || value !== "") return;
    restored.current = true;
    try {
      const kept = window.localStorage.getItem(draftKey);
      if (kept) onChange(kept);
    } catch {
      // A browser that refuses storage is not a reason to fail. The draft is
      // a convenience; the text in front of somebody is the real thing.
    }
  }, [draftKey, value, onChange]);

  useEffect(() => {
    if (!draftKey) return;
    try {
      if (value) window.localStorage.setItem(draftKey, value);
      else window.localStorage.removeItem(draftKey);
    } catch {
      // As above.
    }
  }, [draftKey, value]);

  function surround(mark: Mark) {
    const field = box.current;
    if (!field) return;
    const [before, after] = mark.wrap;
    const start = field.selectionStart;
    const end = field.selectionEnd;
    const chosen = value.slice(start, end);
    const next = value.slice(0, start) + before + chosen + after + value.slice(end);
    onChange(next);
    // Put the cursor where somebody would carry on typing, which is inside
    // the marks when nothing was selected and after them when something was.
    queueMicrotask(() => {
      field.focus();
      const at = start + before.length + chosen.length;
      field.setSelectionRange(chosen ? at + after.length : at, chosen ? at + after.length : at);
    });
  }

  return (
    <div className="rounded-lg border border-edge bg-raised">
      <div className="flex flex-wrap items-center gap-1 border-b border-edge px-2 py-1.5">
        {MARKS.map((mark) => (
          <button
            key={mark.title}
            type="button"
            title={mark.title}
            aria-label={mark.title}
            onClick={() => surround(mark)}
            className="min-h-8 min-w-8 rounded px-2 text-sm text-muted hover:bg-sunken hover:text-ink"
          >
            {mark.label}
          </button>
        ))}
        <div className="ml-auto flex gap-1">
          {(["write", "preview"] as const).map((tab) => (
            <button
              key={tab}
              type="button"
              onClick={() => setShowing(tab)}
              aria-pressed={showing === tab}
              className={`min-h-8 rounded px-2 text-sm ${
                showing === tab ? "bg-sunken text-ink" : "text-muted hover:text-ink"
              }`}
            >
              {tab === "write" ? "Write" : "Preview"}
            </button>
          ))}
        </div>
      </div>

      {showing === "write" && typing !== null && candidates.length > 0 && (
        <ul className="mx-3 mt-1 max-h-40 overflow-y-auto rounded border border-edge bg-sunken text-sm">
          {candidates.map((each) => (
            <li key={each.identity}>
              <button
                type="button"
                onMouseDown={(event) => event.preventDefault()}
                onClick={() => complete(each.identity ?? "")}
                className="block w-full px-3 py-1.5 text-left hover:bg-raised"
              >
                <span className="font-medium">@{each.identity}</span>
                {each.name && each.name !== each.identity && (
                  <span className="ml-2 text-muted">{each.name}</span>
                )}
              </button>
            </li>
          ))}
        </ul>
      )}

      {showing === "write" ? (
        <textarea
          ref={box}
          value={value}
          rows={rows}
          aria-label={label ?? "Markdown"}
          placeholder={placeholder}
          onChange={(event) => {
            onChange(event.target.value);
            setTyping(mentions?.product ? partial(event.target) : null);
          }}
          onKeyUp={(event) => setTyping(mentions?.product ? partial(event.currentTarget) : null)}
          onBlur={() => {
            // Left open, a list floating over the next control is worse than
            // no list. Delayed so a click on an entry still lands.
            window.setTimeout(() => setTyping(null), 150);
          }}
          className="w-full resize-y bg-transparent px-3 py-2 font-mono text-sm outline-none"
        />
      ) : (
        <div className="px-3 py-2 text-sm">
          {value ? (
            <Markdown source={value} />
          ) : (
            <p className="text-muted">Nothing written yet.</p>
          )}
        </div>
      )}
    </div>
  );
}

// forget clears a draft once its text has actually been accepted. Called on
// success only: a failed submission keeps what somebody wrote.
export function forget(draftKey?: string) {
  if (!draftKey) return;
  try {
    window.localStorage.removeItem(draftKey);
  } catch {
    // Nothing to clear if storage was refused in the first place.
  }
}
