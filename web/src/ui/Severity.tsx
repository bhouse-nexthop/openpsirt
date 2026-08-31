// Severity reads at a glance and never borrows the accent colour. A page that
// paints "critical" in the brand colour has nothing left that means "act on
// this" (see the palette in index.css).
const tone: Record<string, string> = {
  exploited: "bg-exploited/12 text-exploited ring-exploited/30",
  critical: "bg-critical/12 text-critical ring-critical/30",
  high: "bg-high/12 text-high ring-high/30",
  medium: "bg-medium/12 text-medium ring-medium/30",
  low: "bg-low/12 text-low ring-low/30",
};

export function Severity({ word, exploited }: { word?: string; exploited?: boolean }) {
  // Exploited outranks whatever the score says. Severity is how bad the flaw
  // is; being exploited is a fact about the world, and the ordering everywhere
  // else in this tool puts the fact first.
  const shown = exploited ? "exploited" : (word || "unrated");
  const style = tone[shown] ?? "bg-sunken text-muted ring-edge";
  return (
    <span
      className={`inline-flex items-center rounded px-1.5 py-0.5 text-xs font-medium ring-1 ring-inset ${style}`}
      title={exploited && word ? `known exploited — rated ${word}` : undefined}
    >
      {shown}
    </span>
  );
}
