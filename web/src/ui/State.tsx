// Where a claim has got to. "Proposed" and "approved" are different in the one
// way that matters — a claim nobody has agreed to suppresses nothing — so they
// never share a colour.
const means: Record<string, string> = {
  proposed: "waiting for a second person. It suppresses nothing yet",
  approved: "agreed to, and in force",
  withdrawn: "taken back. Kept on the record",
  lapsed: "the code moved out from under it, so it no longer applies",
};

const colour: Record<string, string> = {
  proposed: "var(--wait)",
  approved: "var(--ok)",
  withdrawn: "var(--faint)",
  lapsed: "var(--faint)",
};

export function State({ state }: { state?: string }) {
  if (!state) return null;
  return (
    <span
      className="chip"
      title={means[state]}
      style={{ color: colour[state] ?? "var(--muted)" }}
    >
      {state}
    </span>
  );
}
