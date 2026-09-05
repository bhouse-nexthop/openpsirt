import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import { unwrap } from "../api/queries";
import { Failed } from "../ui/Failed";

// What this deployment has decided for everybody in it, grouped the way the
// mockup groups them. Every setting the server exposes renders; a setting no
// group names lands under "Other", so nothing offered is hidden.
export function Settings() {
  const queries = useQueryClient();
  const settings = useQuery({
    queryKey: ["settings"],
    queryFn: async () => unwrap(await api.GET("/v1/settings", {})),
  });

  const set = useMutation({
    mutationFn: async ({ name, value }: { name: string; value: string }) =>
      unwrap(await api.PUT("/v1/settings/{name}", { params: { path: { name } }, body: { value } })),
    onSuccess: () => void queries.invalidateQueries({ queryKey: ["settings"] }),
  });

  if (settings.isPending) return <p className="hint">Loading…</p>;
  if (settings.isError) {
    return <Failed error={settings.error} what="The settings could not be read." />;
  }

  const items = settings.data?.items ?? [];
  const named = new Set<string>();
  const group = (test: (name: string) => boolean) =>
    items.filter((each) => {
      const name = each.name ?? "";
      if (named.has(name) || !test(name)) return false;
      named.add(name);
      return true;
    });
  const deadlines = group((name) => name.startsWith("remediation.due."));
  const floor = group((name) => name === "triage.floor");
  const threshold = group(
    (name) => name === "triage.deferral-threshold" || name === "triage.together-cap",
  );
  const rest = group(() => true);

  const field = (each: (typeof items)[number]) => (
    <Field
      key={each.name}
      setting={each}
      onSet={(value) => set.mutate({ name: each.name ?? "", value })}
    />
  );

  return (
    <>
      <div className="screen-head">
        <h2>Settings</h2>
        <p>Applies to everyone here</p>
      </div>

      {set.error != null && <Failed error={set.error} what="That could not be recorded." />}

      {deadlines.length > 0 && (
        <div className="card" style={{ marginBottom: 14 }}>
          <h3>Remediation deadlines</h3>
          <div style={{ display: "flex", gap: 18, flexWrap: "wrap", alignItems: "flex-end" }}>
            {deadlines.map(field)}
          </div>
          <p className="reading" style={{ marginTop: 10 }}>
            Counted from when a finding was first seen, for what is undecided. Being exploited sets
            its own clock, whatever the severity says; an unrated finding takes the medium window.
            Being late is reported, never acted on.
          </p>
        </div>
      )}

      {floor.length > 0 && (
        <div className="card" style={{ marginBottom: 14 }}>
          <h3>Severity floor</h3>
          <div style={{ display: "flex", gap: 12, flexWrap: "wrap", alignItems: "flex-end" }}>
            {floor.map(field)}
          </div>
          <p className="reading" style={{ marginTop: 10 }}>
            Every shared figure carries this and says so. A person may narrow their own screen
            further, and that changes no number anybody else is shown. Below the line, nothing has a
            deadline.
          </p>
        </div>
      )}

      {threshold.length > 0 && (
        <div className="card" style={{ marginBottom: 14 }}>
          <h3>Approval thresholds</h3>
          <div style={{ display: "flex", gap: 18, flexWrap: "wrap", alignItems: "flex-end" }}>
            {threshold.map(field)}
          </div>
          <p className="reading" style={{ marginTop: 10 }}>
            A deferral is measured against everything the finding has already been put off for, not
            against the deferral being asked for. A bulk decision always needs a second person, and
            is bounded.
          </p>
        </div>
      )}

      {rest.map((each) => (
        <div className="card" key={each.name} style={{ marginBottom: 14 }}>
          <h3>{title(each.name)}</h3>
          {field(each)}
          <p className="reading" style={{ marginTop: 10 }}>
            {each.means}
          </p>
        </div>
      ))}

      <p className="hint">
        Shipped values are a starting point rather than a recommendation. Zero or negative reads as
        unset and is refused rather than stored.
      </p>
    </>
  );
}

function title(name?: string): string {
  switch (name) {
    case "session.lifetime":
      return "Session lifetime";
    case "token.max-lifetime":
      return "Personal token lifetime";
    case "upstream.currency":
      return "Upstream currency";
    case "scanning.quiet-after":
      return "Quiet after";
    default:
      return (
        (name ?? "")
          .split(".")
          .pop()
          ?.replace(/-/g, " ")
          .replace(/^\w/, (c) => c.toUpperCase()) ?? ""
      );
  }
}

function label(name?: string): string {
  const last = (name ?? "").split(".").pop() ?? "";
  return last.replace(/-/g, " ");
}

// The settings whose value is one of a few words rather than a length of time.
// A select rather than a text box, because a free field invites a value the
// server then refuses.
const choices: Record<string, string[]> = {
  "triage.floor": ["everything", "low", "medium", "high", "critical"],
  "upstream.currency": ["off", "on"],
};

// A length of time as a person reads it. The server takes and returns the
// duration syntax its own clock uses, "72h" or "12h0m0s", and a number of
// hours is not how anybody thinks about a deadline: "3 days" is. Shown beside
// the field rather than in it, so what is stored is still what was typed.
function humane(value: string): string {
  const m = /^(?:(\d+)h)?(?:(\d+)m)?(?:(\d+)s)?$/.exec(value.trim());
  if (!m || !value.trim()) return "";
  const hours = Number(m[1] ?? 0) + Number(m[2] ?? 0) / 60 + Number(m[3] ?? 0) / 3600;
  if (hours === 0) return "";
  if (hours % 24 === 0) {
    const days = hours / 24;
    if (days % 365 === 0) return days === 365 ? "1 year" : `${days / 365} years`;
    return days === 1 ? "1 day" : `${days} days`;
  }
  if (hours >= 1 && Number.isInteger(hours)) return hours === 1 ? "1 hour" : `${hours} hours`;
  return "";
}

function Field({
  setting,
  onSet,
}: {
  setting: { name?: string; value?: string; default?: boolean; means?: string };
  onSet: (value: string) => void;
}) {
  const [value, setValue] = useState(setting.value ?? "");
  const changed = value !== (setting.value ?? "");
  const words = choices[setting.name ?? ""];

  return (
    <div className="field" style={{ margin: 0, maxWidth: 240 }}>
      <label htmlFor={setting.name}>
        {label(setting.name)}
        {setting.default && <span className="hint"> · default</span>}
      </label>
      <div style={{ display: "flex", gap: 6 }}>
        {words ? (
          <select
            id={setting.name}
            value={value}
            onChange={(event) => setValue(event.target.value)}
            title={setting.means}
          >
            {words.map((word) => (
              <option key={word} value={word}>
                {word}
              </option>
            ))}
          </select>
        ) : (
          <input
            id={setting.name}
            type="text"
            value={value}
            onChange={(event) => setValue(event.target.value)}
            title={setting.means}
          />
        )}
        {changed && (
          <button type="button" className="btn" onClick={() => onSet(value)}>
            Save
          </button>
        )}
      </div>
      {!words && humane(value) && <span className="hint">= {humane(value)}</span>}
    </div>
  );
}
