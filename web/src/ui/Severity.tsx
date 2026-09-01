// Severity reads at a glance and never borrows the accent colour: "urgent" and
// "clickable" must never look like the same thing.
//
// The rating and the fact are shown separately, because they are separate.
// Severity says how bad the flaw is; being exploited says somebody is using
// it. Replacing "medium" with "exploited" answers one question by destroying
// the other — and the two together are what explain why an exploited medium
// sits above an unexploited high in the list.
export function Severity({ word }: { word?: string }) {
  const shown = word || "unrated";
  const known = ["critical", "high", "medium", "low"].includes(shown);
  return <span className={`sev ${known ? shown : "low"}`}>{shown}</span>;
}

// Known-exploited, said outright rather than left to a colour. It is a fact
// about the world rather than a judgment, and it is what decides the order.
export function Exploited({ when }: { when?: boolean }) {
  if (!when) return null;
  return (
    <span className="kev" title="Somebody is known to be using this. It sorts above everything else, whatever the severity says">
      Exploited
    </span>
  );
}
