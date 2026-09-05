import { useEffect, useRef, useState, type ReactNode } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link, NavLink, useLocation, useNavigate } from "react-router-dom";
import { findingsPath, useScope } from "./scope";
import { forgetAll } from "./drafts";
import { Scope } from "./Scope";
import { api } from "../api/client";
import { unwrap } from "../api/queries";
import { Notices } from "../ui/Notices";
import { Icon } from "../ui/Icons";
import { UploadDrawer } from "../ui/Upload";
import { LOOKS, applyLook, currentLook, type Look } from "./look";
import type { Who } from "./session";

// The frame the restyled mockup settled on (UIX-50): a rail down the side
// carrying the brand and the entries grouped by what they span, a bar across
// the top carrying what you are looking at, a way to find things, a way to
// upload, what is waiting on you, and who you are; the screen in the rest.
//
// The grouping is the point. "Across products" and a named build are
// different kinds of place, and a flat list of links hides that the findings
// list is bound to one build while the queue is not (UIX-07 against UIX-08).
export function Shell({ who, children }: { who: Who; children: ReactNode }) {
  const { product, stream, variant } = useScope();
  const whole = !!(product && stream && variant);
  const scope = [product ?? "all products", stream, variant].filter(Boolean).join(" · ");
  const build = whole
    ? `/products/${encodeURIComponent(product)}/streams/${encodeURIComponent(stream)}/variants/${encodeURIComponent(variant)}`
    : "";
  const [uploading, setUploading] = useState(false);
  // On a narrow screen the rail is a panel that opens from a menu control.
  // The tab bar carries the three places somebody reviews and responds from
  // (UIX-17), and everything else is one tap further rather than absent.
  const [menu, setMenu] = useState(false);
  const { pathname } = useLocation();
  useEffect(() => {
    setMenu(false);
  }, [pathname]);
  useEffect(() => {
    if (!menu) return;
    function key(event: KeyboardEvent) {
      if (event.key === "Escape") setMenu(false);
    }
    document.addEventListener("keydown", key);
    return () => document.removeEventListener("keydown", key);
  }, [menu]);

  // The counts on the rail. Asked for with a page of one, because the total
  // is what is wanted and the rows are not.
  const queue = useQuery({
    queryKey: ["queue", "count"],
    queryFn: async () =>
      unwrap(await api.GET("/v1/review-queue", { params: { query: { limit: 1 } } })),
    refetchInterval: 60_000,
  });
  const unassigned = useQuery({
    queryKey: ["unassigned", "count", product ?? ""],
    queryFn: async () =>
      unwrap(
        await api.GET("/v1/unassigned", {
          params: { query: { limit: 1, ...(product ? { product } : {}) } },
        }),
      ),
    refetchInterval: 60_000,
  });
  // Counted for whatever is selected rather than only for a whole build
  // (UIX-53): the entry beside this number opens the list that produced it,
  // and a rail that goes quiet the moment somebody widens the scope is one
  // that looks broken rather than one that declines.
  const open = useQuery({
    queryKey: ["findings", "count", product, stream, variant],
    enabled: !!product,
    queryFn: async () =>
      unwrap(
        await api.GET("/v1/products/{product}/findings", {
          params: {
            path: { product: product ?? "" },
            query: {
              limit: 1,
              ...(stream ? { stream } : {}),
              ...(variant ? { variant } : {}),
            },
          },
        }),
      ),
  });

  return (
    <div className="app">
      <div className="chrome">
        <button
          type="button"
          className="menubtn"
          aria-label={menu ? "Close the menu" : "Open the menu"}
          aria-expanded={menu}
          onClick={() => setMenu(!menu)}
        >
          <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M4 7h16M4 12h16M4 17h16" /></svg>
        </button>
        <Scope />
        <span className="spacer" />
        <Search build={build} />
        <button
          type="button"
          className="topact"
          title="Upload an inventory for any declared build"
          onClick={() => setUploading(true)}
        >
          <Icon name="upload" />
          <span>Upload</span>
        </button>
        <Notices />
        <Me who={who} />
      </div>

      {menu && <div className="railscrim" onClick={() => setMenu(false)} />}
      <nav className={menu ? "rail open" : "rail"}>
        <Link to="/" className="brand" style={{ textDecoration: "none" }}>
          <span className="glyph">
            <Icon name="shield" size={16} />
          </span>
          <span>
            OpenPSIRT
            <small>product security</small>
          </span>
        </Link>

        <span className="group">Across products</span>
        <Rail to="/" end icon="home" label="Home" />
        <Rail to="/review-queue" icon="inbox" label="Review queue" count={queue.data?.total} />
        <Rail to="/unassigned" icon="nobody" label="Unassigned" count={unassigned.data?.total} quiet />
        <Rail to="/work" icon="people" label="Assignments" />
        {/* The record of what was judged. Across products because that is how
            it is asked for — an auditor asks about a period, not a build. */}
        <Rail to="/audit" icon="scan" label="The record" />
        {/* How the work is going, as against what it is: how fast things are
            fixed, and what keeps being put off. */}
        <Rail to="/reports" icon="compare" label="Reports" />

        <span className="group" title={scope}>
          {scope}
        </span>
        {/* Five screens exist for one build and no other (UIX-39). Without one
            picked they decline rather than opening on a scope that means
            nothing, and say why. Findings is not one of them any more: it
            answers for whatever is selected and needs only a product
            (UIX-53). */}
        <Rail
          to={findingsPath({ product, stream, variant })}
          icon="bug"
          label="Findings"
          count={open.data?.total}
          quiet
          needs={!!product}
          why="Pick a product first"
        />
        <Rail to={`${build}/components`} icon="tree" label="Dependencies" needs={whole} />
        <Rail to={`${build}/scans`} icon="scan" label="Inventories" needs={whole} />
        {/* Comparison belongs to the product rather than to one build — it is
            the screen for picking two of them — but it is reached from here
            because this is where somebody already is. */}
        <Rail
          to={`/products/${encodeURIComponent(product ?? "")}/comparison`}
          icon="compare"
          label="Release comparison"
          needs={!!product}
          why="Pick a product first"
        />

        <span className="group">Manage</span>
        <Rail to="/products" end icon="box" label="Products" />
        <Rail
          to={`/products/${encodeURIComponent(product ?? "")}/streams`}
          end
          icon="branch"
          label="Branches and tags"
          needs={!!product}
          why="Pick a product first"
        />
        <Rail
          to={`/products/${encodeURIComponent(product ?? "")}/variants`}
          icon="layers"
          label="Variants"
          needs={!!product}
          why="Pick a product first"
        />
        {who.admin && <Rail to="/people" icon="users" label="Users and roles" />}
        {who.admin && <Rail to="/settings" icon="sliders" label="Settings" />}
      </nav>

      <main className="stage">{children}</main>

      {/* A narrow screen has no rail. The three places somebody reviews and
          responds from are a tab bar instead (UIX-17). */}
      <nav className="tabbar">
        <NavLink to="/" end>
          Home
        </NavLink>
        <NavLink to={findingsPath({ product, stream, variant })}>
          Findings
        </NavLink>
        <NavLink to="/review-queue">
          Queue
        </NavLink>
        <button type="button" aria-expanded={menu} onClick={() => setMenu(!menu)}>
          Menu
        </button>
      </nav>

      <UploadDrawer open={uploading} onClose={() => setUploading(false)} />
    </div>
  );
}

function Rail({
  to,
  icon,
  label,
  count,
  quiet,
  end,
  needs = true,
  why = "This screen is about one build — pick a product, a branch and a variant",
}: {
  to: string;
  icon: string;
  label: string;
  count?: number;
  quiet?: boolean;
  end?: boolean;
  needs?: boolean;
  why?: string;
}) {
  if (!needs) {
    return (
      <button type="button" className="nav" disabled title={why}>
        <Icon name={icon} />
        {label}
      </button>
    );
  }
  // NavLink marks the active entry with aria-current itself, which is what the
  // rail styles key off — the state is announced to a screen reader and drawn
  // from the same fact, rather than a class that only one of them can see.
  return (
    <NavLink to={to} end={end} className="nav">
      <Icon name={icon} />
      {label}
      {typeof count === "number" && count > 0 && (
        <span className={quiet ? "count quiet" : "count"}>{count.toLocaleString()}</span>
      )}
    </NavLink>
  );
}

// Finding a component or an issue from anywhere. It is the findings list's
// own search, reached without going there first; the list is bound to one
// build, so without one this says so rather than searching nothing.
function Search({ build }: { build: string }) {
  const navigate = useNavigate();
  const [typed, setTyped] = useState("");
  const box = useRef<HTMLInputElement>(null);

  // "/" focuses it, unless somebody is already typing somewhere.
  useEffect(() => {
    function key(event: KeyboardEvent) {
      const tag = (document.activeElement?.tagName ?? "").toUpperCase();
      if (event.key === "/" && !/INPUT|TEXTAREA|SELECT/.test(tag)) {
        event.preventDefault();
        box.current?.focus();
      }
    }
    document.addEventListener("keydown", key);
    return () => document.removeEventListener("keydown", key);
  }, []);

  return (
    <form
      className="topsearch"
      title={build ? undefined : "Pick a product, a branch and a variant to search its findings"}
      onSubmit={(event) => {
        event.preventDefault();
        if (!build) return;
        navigate(`${build}/findings?q=${encodeURIComponent(typed.trim())}`);
      }}
    >
      <Icon name="search" />
      <input
        ref={box}
        type="text"
        value={typed}
        disabled={!build}
        placeholder="Find a component or an issue…"
        aria-label="Find a component or an issue"
        onChange={(event) => setTyped(event.target.value)}
      />
      <kbd>/</kbd>
    </form>
  );
}

// Who you are, with the look menu and the way out underneath.
function Me({ who }: { who: Who }) {
  const [open, setOpen] = useState(false);
  const [look, setLook] = useState<Look>(() => currentLook());
  const box = useRef<HTMLDivElement>(null);

  useEffect(() => {
    applyLook(look);
  }, [look]);

  useEffect(() => {
    if (!open) return;
    function away(event: MouseEvent) {
      if (box.current && !box.current.contains(event.target as Node)) setOpen(false);
    }
    document.addEventListener("mousedown", away);
    return () => document.removeEventListener("mousedown", away);
  }, [open]);

  return (
    <div className="me" ref={box}>
      <span className="hint">
        <b>{who.name.replace(/^[a-z]+:/, "")}</b>
      </span>
      <button
        type="button"
        className="who"
        aria-expanded={open}
        aria-label="You"
        title={who.identity}
        onClick={() => setOpen(!open)}
      >
        {initials(who.name)}
      </button>
      {open && (
        <div className="lookmenu" role="menu">
          <h5>Look</h5>
          {LOOKS.map((each) => (
            <button
              key={each.name}
              type="button"
              className="opt"
              role="menuitemradio"
              aria-checked={look === each.name}
              aria-current={look === each.name ? "true" : undefined}
              onClick={() => setLook(each.name)}
            >
              <span className="swatch" style={{ "--a": each.a, "--b": each.b } as React.CSSProperties} />
              {each.label}
              <span className="tag">{each.said}</span>
            </button>
          ))}
          <hr />
          <button
            type="button"
            className="opt"
            role="menuitem"
            onClick={async () => {
              // Cleared first, and whatever the server says. Drafts hold
              // triage text, private findings included, and text that
              // survived a sign-out would be exposed in a way the
              // application itself is not (UIX-31). A sign-out that failed
              // to reach the server is the case where clearing matters most.
              forgetAll();
              try {
                await api.DELETE("/v1/session", {});
              } finally {
                // A full load rather than a route change: signing out has to
                // drop every cached answer, and starting again is the way to
                // be sure.
                window.location.assign("/");
              }
            }}
          >
            Sign out
            <span className="tag">{who.identity}</span>
          </button>
        </div>
      )}
    </div>
  );
}

// Two letters for the corner. A display name people set is usually a full
// name; an identity is usually not, and either has to fit in 30 pixels.
function initials(name: string): string {
  const parts = name.replace(/^[a-z]+:/, "").split(/[\s._-]+/).filter(Boolean);
  if (parts.length === 0) return "?";
  if (parts.length === 1) return (parts[0] ?? "").slice(0, 2).toUpperCase();
  return ((parts[0]?.[0] ?? "") + (parts[1]?.[0] ?? "")).toUpperCase();
}
