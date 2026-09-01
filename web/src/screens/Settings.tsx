import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import { unwrap } from "../api/queries";
import { Failed } from "../ui/Failed";

// What this deployment has decided for everybody in it. An administrator is
// generally not the person who can reach the filesystem and restart the
// process, so anything they are expected to tune is an action here with a
// record of who did it rather than a deployment.
export function Settings() {
  const queries = useQueryClient();
  const settings = useQuery({
    queryKey: ["settings"],
    queryFn: async () => unwrap(await api.GET("/v1/settings", {})),
  });

  const set = useMutation({
    mutationFn: async ({ name, value }: { name: string; value: string }) =>
      unwrap(
        await api.PUT("/v1/settings/{name}", {
          params: { path: { name } },
          body: { value },
        }),
      ),
    onSuccess: () => void queries.invalidateQueries({ queryKey: ["settings"] }),
  });

  if (settings.isPending) return <p className="hint">Loading…</p>;
  if (settings.isError) {
    return <Failed error={settings.error} what="The settings could not be read." />;
  }

  const items = settings.data?.items ?? [];
  const deadlines = items.filter((each) => (each.name ?? "").startsWith("remediation.due."));
  const rest = items.filter((each) => !(each.name ?? "").startsWith("remediation.due."));

  return (
    <>
      <div className="screen-head">
        <h2>Settings</h2>
        <p>What this deployment has decided for everybody in it.</p>
      </div>

      {set.error != null && <Failed error={set.error} what="That could not be recorded." />}

      <div className="card" style={{ marginBottom: 14 }}>
        <h3 style={{ marginTop: 0 }}>How long something may stay open</h3>
        <div style={{ display: "flex", gap: 12, flexWrap: "wrap" }}>
          {deadlines.map((each) => (
            <Field key={each.name} setting={each} onSet={(value) => set.mutate({ name: each.name ?? "", value })} />
          ))}
        </div>
        <p className="reading" style={{ marginTop: 10 }}>
          <b>Being exploited sets its own clock, whatever the severity says.</b> Otherwise the two
          disagree: a medium somebody is actively using sorts above an unexploited critical, and
          then the list says look at this first while the deadline says you have ninety days.
          Severity is how bad the flaw is; being exploited is a fact about the world, and it is
          the one that decides how long you have.
        </p>
        <p className="reading" style={{ marginTop: 10 }}>
          Anything the reports did not rate takes the medium window. Being late is reported,
          never acted on — the failure to design against is a deadline nobody agreed to, applied
          to everything, so that the whole estate is permanently overdue and the signal is
          ignored.
        </p>
      </div>

      {rest.map((each) => (
        <div className="card" key={each.name} style={{ marginBottom: 14 }}>
          <h3 style={{ marginTop: 0 }}>{title(each.name)}</h3>
          <Field setting={each} onSet={(value) => set.mutate({ name: each.name ?? "", value })} />
          <p className="reading" style={{ marginTop: 10 }}>{each.means}</p>
        </div>
      ))}

      <p className="hint">
        The shipped numbers are a starting point rather than a recommendation. What a deployment
        can actually hold to is a question about that deployment.
      </p>
    </>
  );
}

function title(name?: string): string {
  switch (name) {
    case "triage.deferral-threshold":
      return "When a postponement needs a second person";
    case "session.lifetime":
      return "How long a sign-in lasts";
    case "token.max-lifetime":
      return "The longest a personal token may last";
    case "triage.together-cap":
      return "How much one action may claim about";
    default:
      return name ?? "";
  }
}

function label(name?: string): string {
  const last = (name ?? "").split(".").pop() ?? "";
  return last.replace(/-/g, " ");
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

  return (
    <div className="field" style={{ margin: 0, maxWidth: 240 }}>
      <label htmlFor={setting.name}>
        {label(setting.name)}
        {setting.default && <span className="hint"> · shipped default</span>}
      </label>
      <div style={{ display: "flex", gap: 6 }}>
        <input
          id={setting.name}
          type="text"
          value={value}
          onChange={(event) => setValue(event.target.value)}
          title={setting.means}
        />
        {changed && (
          <button type="button" className="btn" onClick={() => onSet(value)}>
            Save
          </button>
        )}
      </div>
    </div>
  );
}
