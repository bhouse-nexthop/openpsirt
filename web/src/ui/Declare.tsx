import { useState, type ReactNode } from "react";
import { Failed } from "./Failed";

// A form for declaring something into the catalogue.
//
// Products, branches and variants are declared rather than discovered, which
// is a decision rather than a limitation: a misspelled release is
// indistinguishable from a real one, so a scan naming something nobody
// declared is refused instead of quietly creating it (ING-11). That only works
// if declaring is something a person can actually do, which until now it was
// not — the endpoints existed and no screen reached them.
export function Declare({
  what,
  hint,
  children,
  onSubmit,
  error,
  busy,
  can,
}: {
  what: string;
  hint: string;
  children: ReactNode;
  onSubmit: () => void;
  error: unknown;
  busy: boolean;
  can: boolean;
}) {
  const [open, setOpen] = useState(false);

  if (!can) return null;

  return (
    <div className="card" style={{ marginBottom: 14 }}>
      {!open ? (
        <button type="button" className="linkish" onClick={() => setOpen(true)}>
          Declare {what}
        </button>
      ) : (
        <>
          <h3 style={{ marginTop: 0 }}>Declare {what}</h3>
          {error != null && <Failed error={error} what={`That ${what} could not be declared.`} />}
          <form
            onSubmit={(event) => {
              event.preventDefault();
              onSubmit();
            }}
            style={{ display: "flex", gap: 12, flexWrap: "wrap", alignItems: "flex-end" }}
          >
            {children}
            <button type="submit" className="btn" disabled={busy}>
              Declare
            </button>
            <button type="button" className="linkish" onClick={() => setOpen(false)}>
              Cancel
            </button>
          </form>
          <p className="reading" style={{ marginTop: 10 }}>
            {hint}
          </p>
        </>
      )}
    </div>
  );
}

// One labelled input, so the three forms look like one thing.
export function Field({
  label,
  value,
  onChange,
  placeholder,
  hint,
  wide,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  hint?: string;
  wide?: boolean;
}) {
  const id = `declare-${label.replace(/\s+/g, "-").toLowerCase()}`;
  return (
    <div className="field" style={{ margin: 0, minWidth: wide ? 220 : 150 }}>
      <label htmlFor={id}>{label}</label>
      <input
        id={id}
        type="text"
        value={value}
        placeholder={placeholder}
        onChange={(event) => onChange(event.target.value)}
      />
      {hint && (
        <span style={{ fontSize: "var(--step--1)", color: "var(--faint)" }}>{hint}</span>
      )}
    </div>
  );
}
