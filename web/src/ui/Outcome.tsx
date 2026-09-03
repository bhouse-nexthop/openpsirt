// The four outcomes. Two of them hide risk and two do not, which is the
// distinction that decides whether a second person has to agree.
const said: Record<string, { label: string; color: string; means: string }> = {
  affected: {
    label: "Affected",
    color: "var(--sev-high)",
    means: "this applies to us and needs fixing",
  },
  "not-applicable": {
    label: "Not applicable",
    color: "var(--ok)",
    means: "this does not affect us, for one of the recognized reasons",
  },
  deferred: {
    label: "Deferred",
    color: "var(--wait)",
    means: "it applies, and is being put off until a date",
  },
  "wont-fix": {
    label: "Won't fix",
    color: "var(--sev-critical)",
    means: "it applies and will not be fixed",
  },
};

export function Outcome({ outcome }: { outcome?: string }) {
  const it = said[outcome ?? ""];
  if (!it) return null;
  return (
    <span className="sev" title={it.means} style={{ "--c": it.color } as React.CSSProperties}>
      {it.label}
    </span>
  );
}

// The exchange format's own vocabulary (TRI-06), named as it is stored.
export type Justification =
  | "component_not_present"
  | "vulnerable_code_not_present"
  | "vulnerable_code_not_in_execute_path"
  | "vulnerable_code_cannot_be_controlled_by_adversary"
  | "inline_mitigations_already_exist";

export const JUSTIFICATIONS: { value: Justification; label: string }[] = [
  { value: "component_not_present", label: "component_not_present" },
  { value: "vulnerable_code_not_present", label: "vulnerable_code_not_present" },
  { value: "vulnerable_code_not_in_execute_path", label: "vulnerable_code_not_in_execute_path" },
  {
    value: "vulnerable_code_cannot_be_controlled_by_adversary",
    label: "vulnerable_code_cannot_be_controlled_by_adversary",
  },
  { value: "inline_mitigations_already_exist", label: "inline_mitigations_already_exist" },
];
