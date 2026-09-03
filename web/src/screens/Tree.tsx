import { useEffect, useMemo, useState } from "react";
import { useQueries, useQuery } from "@tanstack/react-query";
import { Link, useParams, useSearchParams } from "react-router-dom";
import { api } from "../api/client";
import { unwrap } from "../api/queries";
import { Empty } from "../ui/Empty";
import { Failed } from "../ui/Failed";
import { Crumbs } from "../ui/Crumbs";
import { Icon } from "../ui/Icons";

type At = { product: string; stream: string; variant: string };
type Node = {
  component: string;
  version: string;
  // What is open against this component itself, and what is open in everything
  // under it. A container holds none of its own, so the second is the number
  // that says whether a branch is worth opening.
  findings: number;
  beneath: number;
  children: number;
};

// How many of a wide node's leaves are drawn before it says how many more
// there are. A build's root has thousands, and drawing them all is how a tree
// stops being readable at the first step.
const LEAVES = 5;

// How many of its branches are drawn. Everything that opens is structure
// rather than contents, and hiding structure behind "show all" is what made
// this a list: the root has 96 children that open and 5,174 that do not, so a
// truncation that does not tell them apart shows none of the 96.
const BRANCHES = 15;

// A count past this is emphasised, so the branch worth descending is visible
// without reading every number on the way down.
const HOT = 500;

const aroundKey = (at: At, component: string, version = "") =>
  ["tree-around", at, component, version] as const;

// A name is not always enough: a build ships some libraries at several
// versions, and a finding arriving here says which one it meant.
const fetchAround = (at: At, component: string, version = "") => async () =>
  unwrap(
    await api.GET(
      "/v1/products/{product}/streams/{stream}/variants/{variant}/components/{component}/around",
      { params: { path: { ...at, component }, query: version ? { version } : {} } },
    ),
  );

// The dependency graph, drawn as a tree and expanded a node at a time.
//
// Not a full render: a real image holds thousands of components and tens of
// thousands of edges, which neither draws nor reads (UIX-04). What makes
// opening nodes worth doing is that every one of them carries how many findings
// are open beneath it, so descending follows the findings rather than being
// exploration (UIX-02).
//
// Which component is selected lives in the URL, so a link carries it.
export function Tree() {
  const { product = "", stream = "", variant = "" } = useParams();
  const at = useMemo(() => ({ product, stream, variant }), [product, stream, variant]);
  const [params, setParams] = useSearchParams();
  const focus = params.get("at") ?? "";

  const [opened, setOpened] = useState<Set<string>>(() => new Set());
  const [widened, setWidened] = useState<Set<string>>(() => new Set());
  const term = params.get("q") ?? "";
  const [typed, setTyped] = useState(term);

  const top = useQuery({
    queryKey: ["tree-top", at, term],
    queryFn: async () =>
      unwrap(
        await api.GET("/v1/products/{product}/streams/{stream}/variants/{variant}/components", {
          params: { path: at, query: term ? { q: term } : {} },
        }),
      ),
  });

  const root = (top.data?.root ?? null) as Node | null;
  const rootName = root?.component ?? "";
  const searching = term !== "";
  const found = (top.data?.items ?? []) as Node[];

  function search(next: string) {
    const now = new URLSearchParams(params);
    if (next.trim()) now.set("q", next.trim());
    else now.delete("q");
    setParams(now);
  }

  // The root starts open, because a tree whose only visible row is the thing
  // you already knew you were looking at has told you nothing. Closing it
  // afterwards sticks: this runs once per build, not once per render.
  //
  // Arriving from a finding brings the chain along, and every step of it is
  // opened so the component is on screen under the parents that pull it in,
  // rather than the reader being left at the root to find it again.
  const path = params.get("path") ?? "";
  useEffect(() => {
    if (!rootName) return;
    const steps = path.split("\u001f").filter(Boolean);
    setOpened((prev) => {
      const next = new Set(prev);
      next.add(rootName);
      for (const step of steps) next.add(step);
      return next;
    });
    // A wide node draws a handful of its branches and says how many more
    // there are; a step on the way to the component has to be drawn whether
    // or not it is among that handful, so each parent on the path is widened.
    if (steps.length > 0) {
      setWidened((prev) => {
        const next = new Set(prev);
        next.add(rootName);
        for (const step of steps) next.add(step);
        return next;
      });
    }
  }, [rootName, path]);

  // Children are read for each node the reader has opened. The root's own are
  // already in hand from the query above, so it is not asked for twice.
  const wanted = useMemo(
    () => [...opened].filter((name) => name !== "" && name !== rootName),
    [opened, rootName],
  );
  const branches = useQueries({
    queries: wanted.map((name) => ({
      queryKey: aroundKey(at, name),
      queryFn: fetchAround(at, name),
    })),
  });

  // What is selected. The key matches the one above, so selecting a node that
  // is already open costs nothing.
  const version = params.get("version") ?? "";
  const selected = useQuery({
    queryKey: aroundKey(at, focus, version),
    queryFn: fetchAround(at, focus, version),
    enabled: focus !== "",
  });

  const below = useMemo(() => {
    const map = new Map<string, Node[] | undefined>();
    if (rootName) map.set(rootName, (top.data?.items ?? []) as Node[]);
    wanted.forEach((name, i) => {
      const answer = branches[i]?.data;
      map.set(name, answer ? ((answer.below ?? []) as Node[]) : undefined);
    });
    return map;
  }, [rootName, top.data, wanted, branches]);

  function toggle(name: string) {
    setOpened((prev) => {
      const next = new Set(prev);
      if (next.has(name)) next.delete(name);
      else next.add(name);
      return next;
    });
  }

  // Selecting also opens, because the question "what is under this" is the one
  // being asked by clicking it.
  function select(name: string) {
    setParams(name ? { at: name } : {});
    if (name) setOpened((prev) => (prev.has(name) ? prev : new Set(prev).add(name)));
  }

  return (
    <div>
      <Crumbs product={product} stream={stream} variant={variant} />
      <div className="screen-head">
        <h2>Dependencies</h2>
        <p>
          {product} · {stream} · {variant} — {(top.data?.components ?? 0).toLocaleString()} components,{" "}
          {(top.data?.edges ?? 0).toLocaleString()} edges
        </p>
      </div>

      {/* Searching is the way in, and browsing is for answering "what else is
          under this" once somebody is already somewhere. Eight thousand
          components under a root with five thousand children is not a graph
          anybody reaches the middle of by opening nodes. */}
      <div className="treehead">
        <form
          className="searchbox"
          onSubmit={(event) => {
            event.preventDefault();
            search(typed);
          }}
        >
          <Icon name="search" />
          <input
            type="text"
            value={typed}
            aria-label="Find a component"
            placeholder="Find a component — openssl, linux, python…"
            onChange={(event) => setTyped(event.target.value)}
          />
        </form>
        {searching && (
          <button type="button" className="linkish" onClick={() => { setTyped(""); search(""); }}>
            Back to the tree
          </button>
        )}
        <span className="found">counts are cumulative: what is open beneath a node as well as on it</span>
      </div>

      <div className="detail">
        <div>
          <div className="card">
            {top.isPending && <p className="hint">Loading…</p>}
            {top.isError && <Failed error={top.error} what="The build's contents could not be read." />}
            {!searching && !top.isPending && !top.isError && !root && (
              <Empty
                title="This build has no root."
                detail="Nothing has been scanned here, or the inventory described no component that everything else descends from."
              />
            )}
            {searching && !top.isPending && !top.isError && (
              found.length === 0 ? (
                <Empty
                  title={`Nothing here is called "${term}".`}
                  detail="Matched on part of a name, ignoring case. A component that is in the inventory but not in this build will not appear."
                />
              ) : (
                <Matches found={found} focus={focus} onSelect={select} />
              )
            )}
            {!searching && root && (
              <Branches
                root={root}
                below={below}
                opened={opened}
                widened={widened}
                focus={focus}
                onToggle={toggle}
                onSelect={select}
                onWiden={(name) => setWidened((prev) => new Set(prev).add(name))}
              />
            )}
            <p className="hint" style={{ margin: "12px 0 0" }}>
              {searching
                ? "Matches anywhere in the build, most findings first. Selecting one shows what pulls it in."
                : "Children load when a node is opened. The count on a node is what is open in everything under it."}
            </p>
          </div>
        </div>

        <Pane
          at={at}
          focus={focus}
          rootName={rootName}
          above={(selected.data?.above ?? []) as Node[]}
          belowCount={(selected.data?.below ?? []).length}
          node={findNode(focus, root, below)}
          pending={focus !== "" && selected.isPending}
          error={selected.isError ? selected.error : null}
        />
      </div>
    </div>
  );
}

// The node a name refers to, wherever it has already been read. The tree holds
// every answer the pane needs except the count of what is under a component
// nothing has opened yet, which is what the pane's own query is for.
function findNode(name: string, root: Node | null, below: Map<string, Node[] | undefined>): Node | null {
  if (!name) return null;
  if (root && root.component === name) return root;
  for (const kids of below.values()) {
    const hit = kids?.find((k) => k.component === name);
    if (hit) return hit;
  }
  return null;
}

// A search answers with a set of components rather than a position, so it is
// drawn as a list and not as a tree with one branch. Selecting one moves the
// pane to it, which is where "what pulls this in" is answered.
function Matches({
  found,
  focus,
  onSelect,
}: {
  found: Node[];
  focus: string;
  onSelect: (name: string) => void;
}) {
  return (
    <div className="tree">
      {found.map((node, i) => (
        <div
          key={`${node.component}\u0000${node.version}\u0000${i}`}
          className={`node openable${node.component === focus ? " here" : ""}`}
        >
          <span className="rule">·</span>
          <span className="id" style={{ cursor: "pointer" }} onClick={() => onSelect(node.component)}>
            {node.component}
          </span>
          <span className="ver">{node.version}</span>
          <span
            className={`count${node.beneath > HOT ? " hot" : node.beneath === 0 ? " none" : ""}`}
          >
            {node.beneath.toLocaleString()}
          </span>
        </div>
      ))}
    </div>
  );
}

// One flat list of indented rows rather than nested lists, so the rule down the
// left stays a straight line whatever a branch does.
function Branches({
  root,
  below,
  opened,
  widened,
  focus,
  onToggle,
  onSelect,
  onWiden,
}: {
  root: Node;
  below: Map<string, Node[] | undefined>;
  opened: Set<string>;
  widened: Set<string>;
  focus: string;
  onToggle: (name: string) => void;
  onSelect: (name: string) => void;
  onWiden: (name: string) => void;
}) {
  const rows: React.ReactNode[] = [];
  // A component already drawn higher up is marked rather than expanded again.
  // Without this the same library is redrawn under every parent that pulls it
  // in, and a graph with any sharing at all never finishes.
  const seen = new Set<string>();

  function walk(node: Node, depth: number, path: string) {
    const name = node.component;
    const repeated = seen.has(name);
    seen.add(name);

    const isOpen = opened.has(name);
    const openable = node.children > 0 && !repeated;
    const kids = below.get(name);

    rows.push(
      <div
        key={path}
        className={`node${name === focus ? " here" : ""}${openable ? " openable" : ""}`}
        style={{ paddingLeft: depth * 20 }}
      >
        <span
          className="rule"
          style={openable ? { cursor: "pointer" } : undefined}
          onClick={
            openable
              ? (event) => {
                  event.stopPropagation();
                  onToggle(name);
                }
              : undefined
          }
        >
          {openable ? (isOpen ? "▾" : "▸") : "·"}
        </span>
        <span className="id" style={{ cursor: "pointer" }} onClick={() => onSelect(name)}>
          {name}
        </span>
        <span className="ver">{node.version}</span>
        {repeated && <span className="repeat">shown above</span>}
        {/* What is open in everything under it, not only on it. A container
            holds none of its own, so counting only itself said every one of
            them was clean while the packages inside held thousands. */}
        <span
          className={`count${node.beneath > HOT ? " hot" : node.beneath === 0 ? " none" : ""}`}
          title={
            node.children > 0
              ? `${node.beneath.toLocaleString()} open in here, ${node.findings.toLocaleString()} against this component itself`
              : undefined
          }
        >
          {node.beneath.toLocaleString()}
        </span>
      </div>,
    );

    if (!isOpen || repeated) return;

    if (kids === undefined) {
      rows.push(
        <div key={`${path}/…`} className="node" style={{ paddingLeft: (depth + 1) * 20 }}>
          <span className="rule">·</span>
          <span className="hint">Loading…</span>
        </div>,
      );
      return;
    }

    // Branches and leaves are truncated separately, because they answer
    // different questions: what this is made of, and what is wrong inside it.
    // One limit over both hides the first at the second's expense.
    const branches = kids.filter((kid) => kid.children > 0);
    const leaves = kids.filter((kid) => kid.children === 0);
    const all = widened.has(name);
    const shown = all
      ? kids
      : [...branches.slice(0, BRANCHES), ...leaves.slice(0, LEAVES)];
    const hidden = kids.length - shown.length;
    if (hidden > 0) {
      rows.push(
        <button
          key={`${path}/more`}
          type="button"
          className="more"
          style={{ marginLeft: (depth + 1) * 20 }}
          onClick={() => onWiden(name)}
        >
          Show all {kids.length.toLocaleString()} under {name} — {shown.length} shown
        </button>,
      );
    }
    shown.forEach((kid, i) => walk(kid, depth + 1, `${path}/${i}:${kid.component}`));
  }

  walk(root, 0, root.component);
  return <div className="tree">{rows}</div>;
}

// What is selected: what pulls it in, what is open against it, and what it
// pulls in. Upward is the direction people actually use — somebody arrives
// from a finding and asks why the component is here.
function Pane({
  at,
  focus,
  rootName,
  above,
  belowCount,
  node,
  pending,
  error,
}: {
  at: At;
  focus: string;
  rootName: string;
  above: Node[];
  belowCount: number;
  node: Node | null;
  pending: boolean;
  error: unknown;
}) {
  const build =
    `/products/${encodeURIComponent(at.product)}` +
    `/streams/${encodeURIComponent(at.stream)}` +
    `/variants/${encodeURIComponent(at.variant)}`;
  const list = `${build}/findings`;
  if (!focus) {
    return (
      <div>
        <div className="card">
          <h3>Nothing selected</h3>
          <p className="reading">
            Pick a component on the left to see its dependents, its dependencies, and what is open
            against it.
          </p>
        </div>
      </div>
    );
  }
  if (pending) {
    return (
      <div>
        <div className="card">
          <p className="hint">Loading…</p>
        </div>
      </div>
    );
  }
  if (error) {
    return (
      <div>
        <div className="card">
          <Failed error={error} what="What sits around this could not be read." />
        </div>
      </div>
    );
  }

  return (
    <div>
      <div className="card">
        <h3>{focus}</h3>
        {node && (
          <p className="hint" style={{ margin: "0 0 10px" }}>
            <span className="id">{node.version}</span>
          </p>
        )}

        <div className="upward">
          <h5>Dependents</h5>
          {above.length === 0 ? (
            <p className="reading" style={{ margin: 0 }}>
              {focus === rootName
                ? "Nothing — this is the product itself."
                : "Nothing — the build contains it directly."}
            </p>
          ) : (
            <ol>
              {above.map((parent) => (
                <li key={parent.component}>
                  <span className="id">{parent.component}</span>
                </li>
              ))}
            </ol>
          )}
          {above.length > 1 && (
            <p className="reading" style={{ marginTop: 7 }}>
              Reached {above.length} ways: one component with several edges, not several copies.
            </p>
          )}
        </div>

        <div className="evblock">
          <h4>Open issues</h4>
          <p style={{ fontSize: "var(--step-2)", fontWeight: 700, margin: 0, letterSpacing: "-0.02em" }}>
            {node ? node.beneath.toLocaleString() : "—"}
          </p>
          {node && node.children > 0 && (
            <p className="hint">
              in everything under it by this path. {node.findings.toLocaleString()} at this
              component here.
            </p>
          )}
          {node && node.children === 0 && (
            <p className="hint">at this component, under the parent shown.</p>
          )}
          {/* The count is a way in, not a fact to admire: the list it counts
              is one link away, narrowed on the server the way the list
              narrows everything else. */}
          {node && node.beneath > 0 && (
            <p style={{ margin: "8px 0 0", display: "flex", flexWrap: "wrap", gap: "6px 14px" }}>
              {focus === rootName ? (
                <Link to={`${list}`} className="linkish">
                  View all {node.beneath.toLocaleString()} findings →
                </Link>
              ) : (
                <>
                  {node.findings > 0 && (
                    <Link to={`${list}?component=${encodeURIComponent(focus)}`} className="linkish">
                      Findings at this component →
                    </Link>
                  )}
                  {node.children > 0 && (
                    <Link to={`${list}?beneath=${encodeURIComponent(focus)}`} className="linkish">
                      Findings under it →
                    </Link>
                  )}
                </>
              )}
            </p>
          )}
        </div>

        <div className="evblock">
          <h4>Dependencies</h4>
          <p className="hint">
            {belowCount ? `${belowCount.toLocaleString()} components` : "Nothing — it is a leaf"}
          </p>
        </div>

        {node && node.findings > 0 && (
          <Link to={`${build}/components/${encodeURIComponent(focus)}/decide`} className="linkish">
            Bulk decision →
          </Link>
        )}
      </div>
    </div>
  );
}
