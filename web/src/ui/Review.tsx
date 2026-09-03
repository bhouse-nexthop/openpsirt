import { useEffect, useState } from "react";
import { Failed } from "./Failed";

// Where a decision applies beyond this build, as a guided review (UIX-45).
//
// Three kinds of other build, and only one of them is a choice (TRI-30):
// builds already matching are covered by lookup and are named, not asked
// about; builds at other versions are walked one at a time with the reasoning
// beside each, because a tick is a claim about a version nobody has looked at;
// a build already past the fix is shown and not offered. The walk ends on a
// summary that is confirmed, and that count is what the approval records.

export type Other = {
  // One entry per build and version; the build alone is the label.
  key: string;
  build: string;
  stream: string;
  variant: string;
  version: string;
  places: number;
  blocked?: boolean;
  note: string;
  tone: "ok" | "warn" | "off";
};

export type Plan = {
  build: string;
  covered: number;
  total: number;
  matching: string[];
  offered: Other[];
  blocked: Other[];
  reasoning: string;
  versionHere: string;
};

export function Review({
  open,
  plan,
  busy,
  error,
  onCancel,
  onConfirm,
}: {
  open: boolean;
  plan: Plan;
  busy: boolean;
  error: unknown;
  onCancel: () => void;
  onConfirm: (applied: Other[]) => void;
}) {
  const [step, setStep] = useState(0);
  const [applied, setApplied] = useState<Set<string>>(() => new Set());
  const n = plan.offered.length;

  useEffect(() => {
    if (open) {
      setStep(0);
      setApplied(new Set());
      document.body.dataset.sheet = "open";
      // The keys are the sheet's now, not the field's that opened it.
      (document.activeElement as HTMLElement | null)?.blur?.();
    } else {
      delete document.body.dataset.sheet;
    }
    return () => {
      delete document.body.dataset.sheet;
    };
  }, [open]);

  function go(action: "next" | "back" | "apply" | "skip") {
    if (action === "next") setStep((s) => Math.min(n + 1, s + 1));
    else if (action === "back") setStep((s) => Math.max(0, s - 1));
    else {
      const each = plan.offered[step - 1];
      if (each) {
        setApplied((prev) => {
          const next = new Set(prev);
          if (action === "apply") next.add(each.key);
          else next.delete(each.key);
          return next;
        });
      }
      setStep((s) => s + 1);
    }
  }

  const chosen = plan.offered.filter((o) => applied.has(o.key));

  useEffect(() => {
    if (!open) return;
    function key(event: KeyboardEvent) {
      if (event.key === "Escape") {
        onCancel();
        return;
      }
      const tag = (document.activeElement?.tagName ?? "").toUpperCase();
      if (/INPUT|TEXTAREA|SELECT/.test(tag)) return;
      if (event.key === "Enter") {
        event.preventDefault();
        if (step === 0) go("next");
        else if (step > n && !busy) onConfirm(chosen);
      } else if (step >= 1 && step <= n && (event.key === "a" || event.key === "ArrowRight")) {
        event.preventDefault();
        go("apply");
      } else if (step >= 1 && step <= n && event.key === "s") {
        event.preventDefault();
        go("skip");
      } else if (event.key === "ArrowLeft" || event.key === "Backspace") {
        event.preventDefault();
        go("back");
      }
    }
    document.addEventListener("keydown", key);
    return () => document.removeEventListener("keydown", key);
  });

  if (!open) return null;

  const excerpt = plan.reasoning.trim()
    ? plan.reasoning.trim().split("\n")[0]?.slice(0, 160) + (plan.reasoning.length > 160 ? "…" : "")
    : "(no reasoning written yet)";

  const progress = (
    <div className="revprog">
      {["This build", ...plan.offered.map((o) => o.build), "Confirm"].map((label, i) => (
        <span key={label + i} className={i === step ? "on" : i < step ? "done" : ""}>
          {label}
        </span>
      ))}
    </div>
  );

  let title = "";
  let body: React.ReactNode;
  let foot: React.ReactNode;

  if (step === 0) {
    title = "Where this decision applies";
    body = (
      <>
        {progress}
        <div className="revcard">
          <h5>
            This build · <span className="id">{plan.build}</span>
          </h5>
          <p>
            <b>
              {plan.covered} of {plan.total} {plan.total === 1 ? "location" : "locations"}
            </b>
            {plan.covered < plan.total && <> — {plan.total - plan.covered} left open by you</>}.
          </p>
        </div>
        <div className="revcard auto">
          <h5>Covered automatically · {plan.matching.length}</h5>
          <p>
            {plan.matching.length === 0 ? (
              "No other build matches this one's versions and chain."
            ) : (
              <>
                {plan.matching.map((m, i) => (
                  <span key={m}>
                    {i > 0 && ", "}
                    <span className="id">{m}</span>
                  </span>
                ))}
                . Same upstream versions, same chain. A decision reaches these by lookup; nothing is
                copied and there is nothing to agree to.
              </>
            )}
          </p>
        </div>
        <div className="revcard ask">
          <h5>Other versions · {n}</h5>
          <p>
            {n === 0 ? (
              "No other build holds this issue at a different version."
            ) : (
              <>
                {plan.offered.map((o, i) => (
                  <span key={o.key}>
                    {i > 0 && ", "}
                    <span className="id">{o.build}</span>
                  </span>
                ))}
                . Different code, so each is a separate question. They come next, one at a time,
                with your reasoning beside each.
              </>
            )}
          </p>
        </div>
        {plan.blocked.length > 0 && (
          <div className="revcard off">
            <h5>Not offered · {plan.blocked.length}</h5>
            <p>
              {plan.blocked.map((o, i) => (
                <span key={o.key}>
                  {i > 0 && <br />}
                  <span className="id">{o.build}</span> — {o.note}
                </span>
              ))}
            </p>
          </div>
        )}
      </>
    );
    foot = (
      <>
        <button type="button" className="btn" onClick={() => go("next")}>
          Start → <kbd className="onbtn">↵</kbd>
        </button>
        <button type="button" className="btn quiet" onClick={onCancel}>
          Cancel
        </button>
        <span className="note">Nothing is written until you confirm</span>
      </>
    );
  } else if (step <= n) {
    const each = plan.offered[step - 1]!;
    const state = applied.has(each.key) ? "applied" : "";
    title = `Does the same reasoning hold here? — ${step} of ${n}`;
    body = (
      <>
        {progress}
        <div className={`revcard ${each.tone}`}>
          <h5>
            <span className="id">{each.build}</span>
          </h5>
          <p>
            <span className="id">{each.version}</span>
            <br />
            <span className="hint">
              {each.places} {each.places === 1 ? "location" : "locations"} there · here it is{" "}
              <span className="id">{plan.versionHere || "unstated"}</span>
            </span>
          </p>
          <p className={each.tone === "warn" ? "warnline" : "okline"}>{each.note}</p>
        </div>
        <div className="revcard quote">
          <h5>Your reasoning</h5>
          <p>{excerpt}</p>
          <p className="hint" style={{ margin: "6px 0 0" }}>
            The same words are recorded against that build if you apply them there, keyed to{" "}
            <span className="id">{each.version}</span>, so they lapse on their own when it moves.
          </p>
        </div>
        {state && (
          <p className="hint">
            Currently: <b>{state}</b>
          </p>
        )}
      </>
    );
    foot = (
      <>
        <button type="button" className="btn" onClick={() => go("apply")}>
          Apply here <kbd className="onbtn">a</kbd>
        </button>
        <button type="button" className="btn ghost" onClick={() => go("skip")}>
          Skip <kbd className="onbtn" style={{ borderColor: "var(--accent-line)", background: "none" }}>s</kbd>
        </button>
        <button type="button" className="btn quiet" onClick={() => go("back")}>
          ← Back
        </button>
        <span className="note">Skipped builds stay open</span>
      </>
    );
  } else {
    title = "Confirm";
    const skipped = plan.offered.filter((o) => !applied.has(o.key));
    const records = plan.covered + chosen.reduce((sum, o) => sum + (o.places || 1), 0);
    body = (
      <>
        {progress}
        <div className="revcard">
          <h5>What will be recorded</h5>
          <ul className="revlist">
            <li>
              <b>{plan.covered}</b> {plan.covered === 1 ? "location" : "locations"} in this build
            </li>
            <li>
              <b>{chosen.length}</b> other {chosen.length === 1 ? "build" : "builds"} at other versions
              {chosen.length > 0 && (
                <>
                  :{" "}
                  {chosen.map((o, i) => (
                    <span key={o.key}>
                      {i > 0 && ", "}
                      <span className="id">{o.build}</span>
                    </span>
                  ))}
                </>
              )}
            </li>
          </ul>
        </div>
        <div className="revcard auto">
          <h5>Reached without a record</h5>
          <p>
            <b>{plan.matching.length}</b> matching {plan.matching.length === 1 ? "build" : "builds"}, by
            lookup.
          </p>
        </div>
        {skipped.length > 0 && (
          <div className="revcard off">
            <h5>Left open</h5>
            <p>
              {skipped.map((o, i) => (
                <span key={o.key}>
                  {i > 0 && ", "}
                  <span className="id">{o.build}</span>
                </span>
              ))}{" "}
              — skipped; each stays open and asks nothing further.
            </p>
          </div>
        )}
        {error != null && <Failed error={error} what="That could not be recorded." />}
        <p className="hint">The approver sees this same summary, and the count is kept with the approval.</p>
      </>
    );
    foot = (
      <>
        <button type="button" className="btn" disabled={busy} onClick={() => onConfirm(chosen)}>
          {busy ? "Recording…" : "Confirm and submit"} <kbd className="onbtn">↵</kbd>
        </button>
        <button type="button" className="btn quiet" disabled={busy} onClick={() => go("back")}>
          ← Back
        </button>
        <span className="note">
          <b>{records}</b> {records === 1 ? "record" : "records"} written
        </span>
      </>
    );
  }

  return (
    <div className="backdrop open" onClick={(event) => event.target === event.currentTarget && onCancel()}>
      <div className="sheet" role="dialog" aria-modal="true" aria-label={title}>
        <header>
          <h3>{title}</h3>
          <button type="button" className="shut" onClick={onCancel}>
            Close (Esc)
          </button>
        </header>
        <div className="body">{body}</div>
        <footer>{foot}</footer>
      </div>
    </div>
  );
}
