// The four outcomes, and what each one claims. Two of them hide risk and two
// do not, which is the distinction that decides whether a second person has to
// agree — so it is on the screen rather than only in the rules.
const said: Record<string, { label: string; tone: string; means: string }> = {
  affected: {
    label: "affected",
    tone: "bg-high/12 text-high ring-high/30",
    means: "this applies to us and needs fixing",
  },
  "not-applicable": {
    label: "does not apply",
    tone: "bg-low/12 text-low ring-low/30",
    means: "this does not affect us, for one of the recognized reasons",
  },
  deferred: {
    label: "deferred",
    tone: "bg-medium/12 text-medium ring-medium/30",
    means: "it applies, and is being put off until a date",
  },
  "wont-fix": {
    label: "will not fix",
    tone: "bg-critical/12 text-critical ring-critical/30",
    means: "it applies and will not be fixed",
  },
};

export function Outcome({ outcome }: { outcome?: string }) {
  const it = said[outcome ?? ""];
  if (!it) return null;
  return (
    <span
      title={it.means}
      className={`inline-flex items-center rounded px-1.5 py-0.5 text-xs font-medium ring-1 ring-inset ${it.tone}`}
    >
      {it.label}
    </span>
  );
}

// The vocabulary a claim of "does not apply" has to choose from. It is the
// standard set, and it is the claim itself rather than a note beside it.
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
