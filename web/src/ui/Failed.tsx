import { Refused } from "../api/queries";

// A failure says what the server said. Inventing a friendlier sentence here
// would hide the one the server wrote — which names the line to fix, or which
// part of a declaration is missing, and is the more useful of the two.
export function Failed({ error, what }: { error: unknown; what: string }) {
  const said = error instanceof Refused ? error.message : null;
  return (
    <div className="rounded-lg border border-critical/40 bg-critical/8 px-4 py-3">
      <p className="text-sm font-medium text-ink">{what}</p>
      {said && <p className="mt-1 text-sm text-muted">{said}</p>}
    </div>
  );
}
