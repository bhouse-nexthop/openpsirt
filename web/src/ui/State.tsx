// Where a claim has got to. "Proposed" and "approved" are different in the one
// way that matters — a claim nobody has agreed to suppresses nothing — so they
// never share a colour.
const tone: Record<string, string> = {
  proposed: "bg-medium/12 text-medium ring-medium/30",
  approved: "bg-low/12 text-low ring-low/30",
  withdrawn: "bg-sunken text-muted ring-edge",
  lapsed: "bg-sunken text-muted ring-edge",
};

const means: Record<string, string> = {
  proposed: "waiting for a second person. It suppresses nothing yet",
  approved: "agreed to, and in force",
  withdrawn: "taken back. Kept on the record",
  lapsed: "the code moved out from under it, so it no longer applies",
};

export function State({ state }: { state?: string }) {
  if (!state) return null;
  return (
    <span
      title={means[state]}
      className={`inline-flex items-center rounded px-1.5 py-0.5 text-xs font-medium ring-1 ring-inset ${
        tone[state] ?? "bg-sunken text-muted ring-edge"
      }`}
    >
      {state}
    </span>
  );
}
