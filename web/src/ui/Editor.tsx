import { useEffect, useRef, useState } from "react";
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
}) {
  const [showing, setShowing] = useState<"write" | "preview">("write");
  const box = useRef<HTMLTextAreaElement>(null);

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

      {showing === "write" ? (
        <textarea
          ref={box}
          value={value}
          rows={rows}
          aria-label={label ?? "Markdown"}
          placeholder={placeholder}
          onChange={(event) => onChange(event.target.value)}
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
