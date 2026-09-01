// The four outcomes, and what each one claims. Two of them hide risk and two
// do not, which is the distinction that decides whether a second person has to
// agree — so it is on the screen rather than only in the rules.
const said: Record<string, { label: string; colour: string; means: string }> = {
  affected: {
    label: "affected",
    colour: "var(--sev-high)",
    means: "this applies to us and needs fixing",
  },
  "not-applicable": {
    label: "does not apply",
    colour: "var(--ok)",
    means: "this does not affect us, for one of the recognized reasons",
  },
  deferred: {
    label: "deferred",
    colour: "var(--wait)",
    means: "it applies, and is being put off until a date",
  },
  "wont-fix": {
    label: "will not fix",
    colour: "var(--sev-critical)",
    means: "it applies and will not be fixed",
  },
};

export function Outcome({ outcome }: { outcome?: string }) {
  const it = said[outcome ?? ""];
  if (!it) return null;
  return (
    <span className="sev" title={it.means} style={{ color: it.colour }}>
      {it.label}
    </span>
  );
}

export const JUSTIFICATIONS: { value: string; label: string }[] = [
  { value: "component_not_present", label: "the component is not present" },
  { value: "vulnerable_code_not_present", label: "the vulnerable code is not present" },
  { value: "vulnerable_code_not_in_execute_path", label: "the vulnerable code is never reached" },
  {
    value: "vulnerable_code_cannot_be_controlled_by_adversary",
    label: "the vulnerable code cannot be controlled by an attacker",
  },
  { value: "inline_mitigations_already_exist", label: "mitigations already exist" },
];
