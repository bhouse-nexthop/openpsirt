import type { ReactNode } from "react";
import { Link, NavLink } from "react-router-dom";
import { useScope } from "./scope";
import { Scope } from "./Scope";
import { api } from "../api/client";
import type { Who } from "./session";

// The frame the mockup settled on: a navy bar carrying the mark, what you are
// looking at and who you are; a rail down the side grouped by what the entries
// span; the screen itself in the rest.
//
// The grouping is the point. "Across products" and a named build are different
// kinds of place, and a flat list of links hides that the findings list is
// bound to one build while the queue is not (UIX-07 against UIX-08).
export function Shell({
  who,
  children,
  counts,
}: {
  who: Who;
  children: ReactNode;
  counts?: { queue?: number; unassigned?: number };
}) {
  const { product, stream, variant } = useScope();
  const scope = [product, stream, variant].filter(Boolean).join(" · ");

  return (
    <>
      <div className="chrome">
        <Link to="/" className="mark" style={{ color: "#fff", textDecoration: "none" }}>
          <img src="/brand/mark.svg" alt="" width={20} height={20} />
          OpenPSIRT
        </Link>

        <Scope />

        <span className="spacer" />
        <span className="hint" style={{ color: "rgba(255,255,255,.72)" }}>{who.name}</span>
        <button
          type="button"
          className="who"
          title={`Sign out of ${who.identity}`}
          aria-label="Sign out"
          onClick={async () => {
            await api.DELETE("/v1/session", {});
            // A full load rather than a route change: signing out has to drop
            // every cached answer, and starting again is the way to be sure.
            window.location.assign("/");
          }}
        >
          {initials(who.name)}
        </button>
      </div>

      <div className="body">
        <nav className="rail">
          <span className="group">Across products</span>
          <Rail to="/" end label="Home" />
          <Rail to="/review-queue" label="Review queue" count={counts?.queue} />
          <Rail to="/unassigned" label="Nobody owns" count={counts?.unassigned} quiet />

          {product && stream && variant && (
            <>
              <span className="group">{scope}</span>
              <Rail
                to={`/products/${product}/streams/${stream}/variants/${variant}/findings`}
                label="Findings"
              />
              <Rail
                to={`/products/${product}/streams/${stream}/variants/${variant}/components`}
                label="Dependencies"
              />
              <Rail
                to={`/products/${product}/streams/${stream}/variants/${variant}/scans`}
                label="Scans"
              />
              {/* Comparison belongs to the product rather than to one build —
                  it is the screen for picking two of them — but it is reached
                  from here because this is where somebody already is. */}
              <Rail to={`/products/${product}/comparison`} label="Release comparison" />
            </>
          )}

          <span className="group">Manage</span>
          <Rail to="/products" end label="Products" />
          {who.admin && <Rail to="/people" label="People and access" />}
          {who.admin && <Rail to="/settings" label="Settings" />}
        </nav>

        <main className="stage">{children}</main>
      </div>
    </>
  );
}

function Rail({
  to,
  label,
  count,
  quiet,
  end,
}: {
  to: string;
  label: string;
  count?: number;
  quiet?: boolean;
  end?: boolean;
}) {
  // NavLink marks the active entry with aria-current itself, which is what the
  // rail styles key off — the state is announced to a screen reader and drawn
  // from the same fact, rather than a class that only one of them can see.
  return (
    <NavLink to={to} end={end} className="nav">
      {label}
      {typeof count === "number" && count > 0 && (
        <span className={quiet ? "count quiet" : "count"}>{count}</span>
      )}
    </NavLink>
  );
}

// Two letters for the corner. A display name people set is usually a full
// name; an identity is usually not, and either has to fit in 26 pixels.
function initials(name: string): string {
  const parts = name.replace(/^[a-z]+:/, "").split(/[\s._-]+/).filter(Boolean);
  if (parts.length === 0) return "?";
  if (parts.length === 1) return (parts[0] ?? "").slice(0, 2).toUpperCase();
  return ((parts[0]?.[0] ?? "") + (parts[1]?.[0] ?? "")).toUpperCase();
}
