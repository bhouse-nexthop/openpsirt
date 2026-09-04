import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "react-router-dom";
import { api } from "../api/client";
import { at as choicesAt, unwrap } from "../api/queries";
import { mayOf, type Who } from "../app/session";
import { Drawer } from "./Drawer";
import { Failed } from "./Failed";
import { Icon } from "./Icons";

// Recording a flaw in what we ship: the endpoint existed and nothing called
// it, so a private finding could be entered through the API and not through
// the interface — which is where the person who knows about it is.
//
// It is an action rather than a screen of its own. What somebody is doing is
// adding to the findings of a build they are already looking at, and a form
// that lived somewhere else would ask them to name the build again.

// The severities a person may record. The same words a report carries, so a
// finding somebody typed ranks and expires beside the ones a scanner found
// rather than in a scheme of its own.
const SEVERITIES = ["critical", "high", "medium", "low", "negligible", "none"] as const;

type Build = { product: string; stream: string; variant: string };

export function RecordDrawer({
  open,
  onClose,
  who,
  at,
}: {
  open: boolean;
  onClose: () => void;
  who: Who | null | undefined;
  at: Build;
}) {
  const navigate = useNavigate();
  const queries = useQueryClient();
  const [summary, setSummary] = useState("");
  const [severity, setSeverity] = useState("");
  const [component, setComponent] = useState("");
  // Only ever set by picking one of the choices a refusal offered. Asking for
  // a version up front would ask everybody to answer a question that arises
  // for a handful of names in a build.
  const [version, setVersion] = useState("");
  const [ecosystem, setEcosystem] = useState("");
  const [disclosed, setDisclosed] = useState(false);

  const may = mayOf(who, at.product);
  // Recording something nobody has announced is triage work on undisclosed
  // findings, and that is the right it asks for. Somebody who may argue about
  // known issues in shipped components has not been given the undisclosed
  // ones, and may still record one that is already public.
  const mayHide = !!may?.may_hide;
  const mayRecord = mayHide || !!may?.may_triage;

  useEffect(() => {
    if (!open) return;
    setSummary("");
    setSeverity("");
    setComponent("");
    setVersion("");
    setEcosystem("");
    // Undisclosed unless the person cannot record one, which is the case this
    // exists for. Defaulting the other way makes the dangerous mistake the
    // quiet one.
    setDisclosed(!mayHide);
  }, [open, mayHide]);

  // What the build holds, to offer back as they type. A name typed from
  // memory is a name the server refuses, and a build holds thousands of
  // components, so the list is searched rather than loaded.
  const holding = useQuery({
    queryKey: ["components", at.product, at.stream, at.variant, component],
    enabled: open && component.trim().length >= 2,
    queryFn: async () =>
      unwrap(
        await api.GET("/v1/products/{product}/streams/{stream}/variants/{variant}/components", {
          params: { path: at, query: { q: component.trim(), limit: 20 } },
        }),
      ),
  });

  const record = useMutation({
    mutationFn: async () =>
      unwrap(
        await api.POST("/v1/products/{product}/streams/{stream}/variants/{variant}/findings", {
          params: { path: at },
          body: {
            summary: summary.trim(),
            severity: severity as (typeof SEVERITIES)[number],
            ...(component.trim() ? { component: component.trim() } : {}),
            ...(version ? { version } : {}),
            ...(ecosystem ? { ecosystem } : {}),
            ...(disclosed ? { disclosed: true } : {}),
          },
        }),
      ),
    onSuccess: (made) => {
      void queries.invalidateQueries({ queryKey: ["findings"] });
      void queries.invalidateQueries({ queryKey: ["disclosing"] });
      onClose();
      // Onto the finding. From here it behaves like any other one, and the
      // next thing somebody does with a flaw they have just recorded is work
      // on it.
      navigate(
        `/products/${encodeURIComponent(at.product)}/streams/${encodeURIComponent(at.stream)}` +
          `/variants/${encodeURIComponent(at.variant)}/findings/` +
          `${encodeURIComponent(made.identifier)}/components/` +
          `${encodeURIComponent(made.component)}` +
          (version ? `?version=${encodeURIComponent(version)}` : ""),
      );
    },
  });

  // The build ships that name more than once, and the server said which ones
  // rather than choosing. Picking one is the whole of the answer.
  const choices = choicesAt(record.error, "component");
  const ready = summary.trim() !== "" && severity !== "" && !record.isPending;

  return (
    <Drawer
      open={open}
      title="Record a flaw"
      onClose={onClose}
      footer={
        <>
          <button
            type="button"
            className="btn"
            disabled={!ready || !mayRecord}
            onClick={() => record.mutate()}
          >
            {record.isPending ? "Recording…" : disclosed ? "Record" : "Record, undisclosed"}
          </button>
          <button type="button" className="btn quiet" onClick={onClose}>
            Cancel
          </button>
        </>
      }
    >
      <p className="reading" style={{ margin: "0 0 14px" }}>
        A vulnerability in your own product — one no scanner reported, usually because nobody
        outside knows about it yet. It is filed under an identifier this deployment mints, and
        from there it is an ordinary finding: triaged, assigned, decided, on the same clock and
        in the same reports.
      </p>

      {!mayRecord && (
        <div className="alert">
          <strong>Not yours to record</strong>
          <span>
            Recording a flaw is triage work on {at.product}, and you hold no triage role there.
          </span>
        </div>
      )}

      {record.error != null && choices.length === 0 && (
        <Failed error={record.error} what="That could not be recorded." />
      )}

      {choices.length > 0 && (
        <div className="alert">
          <strong>Which {component}?</strong>
          <span>
            This build ships that name as more than one component. Pick the one that carries it —
            recording against whichever came first would file this against a version nobody named.
          </span>
          <ul className="refs" style={{ marginTop: 8 }}>
            {choices.map((choice) => (
              <li key={`${choice.version} ${choice.ecosystem ?? ""}`}>
                <button
                  type="button"
                  className="chip"
                  aria-pressed={version === choice.version && ecosystem === (choice.ecosystem ?? "")}
                  onClick={() => {
                    setVersion(choice.version);
                    setEcosystem(choice.ecosystem ?? "");
                    record.reset();
                  }}
                >
                  {choice.version}
                </button>
                {choice.ecosystem && <span className="hint">{choice.ecosystem}</span>}
              </li>
            ))}
          </ul>
        </div>
      )}

      <div className="field">
        <label htmlFor="rec-summary">
          What the flaw is{" "}
          <span style={{ textTransform: "none", letterSpacing: 0, color: "var(--sev-high)" }}>
            required
          </span>
        </label>
        <textarea
          id="rec-summary"
          value={summary}
          placeholder="The management socket answers a request before anyone has authenticated."
          onChange={(event) => setSummary(event.target.value)}
        />
        <span className="hint">
          In your own words. It is what a triager reads first and often all they read.
        </span>
      </div>

      <div className="field">
        <label htmlFor="rec-severity">
          Severity{" "}
          <span style={{ textTransform: "none", letterSpacing: 0, color: "var(--sev-high)" }}>
            required
          </span>
        </label>
        <select
          id="rec-severity"
          value={severity}
          onChange={(event) => setSeverity(event.target.value)}
        >
          {/* No default. A severity nobody chose, sitting in the field as
              though somebody had, is a judgment this screen would be making
              on their behalf. */}
          <option value="">how bad is it?</option>
          {SEVERITIES.map((word) => (
            <option key={word} value={word}>
              {word}
            </option>
          ))}
        </select>
        <span className="hint">
          The same words a scanner's findings carry, so this ranks and comes due beside them.
        </span>
      </div>

      <div className="field">
        <label htmlFor="rec-component">What carries it</label>
        <input
          id="rec-component"
          type="text"
          list="rec-holding"
          value={component}
          placeholder="the build itself"
          onChange={(event) => {
            setComponent(event.target.value);
            setVersion("");
            setEcosystem("");
          }}
        />
        {/* Names, not name-and-version rows: a name the build holds three
            times would otherwise offer the same value three times, and which
            of the three is meant is the question the refusal asks properly. */}
        <datalist id="rec-holding">
          {[...new Set((holding.data?.items ?? []).map((each) => each.component))].map((name) => (
            <option key={name} value={name} />
          ))}
        </datalist>
        <span className="hint">
          As the build calls it. Leave it empty for the build itself, which is the honest answer
          where the flaw is in how the pieces fit together rather than in one of them.
          {version && (
            <>
              {" "}
              Recording against <b>{version}</b>
              {ecosystem && <> ({ecosystem})</>}.
            </>
          )}
        </span>
      </div>

      <div className="field">
        <span className="l">Who knows</span>
        <div className="seg">
          <button
            type="button"
            aria-pressed={!disclosed}
            disabled={!mayHide}
            title={
              mayHide
                ? "Nobody outside has been told. It gets a disclosure date"
                : "Recording something nobody has announced needs the private triage role here"
            }
            onClick={() => setDisclosed(false)}
          >
            Nobody outside yet
          </button>
          <button type="button" aria-pressed={disclosed} onClick={() => setDisclosed(true)}>
            Already public
          </button>
        </div>
        <span className="hint">
          {disclosed ? (
            <>
              Already disclosed, so it gets no embargo — a date on it would be a deadline for
              something that has already happened.
            </>
          ) : (
            <>
              It starts undisclosed and gets a disclosure date. Reaching that date discloses
              nothing: it escalates, and somebody decides.
            </>
          )}
        </span>
      </div>
    </Drawer>
  );
}

// The header control that opens it. Offered only where the person may record
// something, because a button that leads to a refusal is worse than no button.
export function RecordButton({ who, product, onClick }: { who: Who | null | undefined; product: string; onClick: () => void }) {
  const may = mayOf(who, product);
  if (!may?.may_triage && !may?.may_hide) return null;
  return (
    <button type="button" className="btn quiet" onClick={onClick}>
      <Icon name="bug" size={14} /> Record a flaw
    </button>
  );
}
