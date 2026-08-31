// What a list says when it has nothing in it. A blank panel reads as broken,
// and "no results" reads as a filter problem even when nothing was filtered —
// so the caller says which of the two this is.
export function Empty({ title, detail }: { title: string; detail?: string }) {
  return (
    <div className="rounded-lg border border-edge bg-raised px-6 py-10 text-center">
      <p className="text-sm font-medium text-ink">{title}</p>
      {detail && <p className="mt-1 text-sm text-muted">{detail}</p>}
    </div>
  );
}
