import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useParams } from "react-router-dom";
import { api } from "../api/client";
import { unwrap } from "../api/queries";
import { useWho } from "../app/session";
import { AddButton, Declare, Field } from "../ui/Declare";
import { Crumbs } from "../ui/Crumbs";
import { Empty } from "../ui/Empty";
import { Failed } from "../ui/Failed";

// A branch and a tag are different shapes of thing — one moves and is rebuilt,
// one never changes again — so they are labelled rather than blended into a
// single list of names. A tag names the branch it was cut from, which is what
// lets a branch be compared against its last release.
export function Streams() {
  const { product = "" } = useParams();
  const queries = useQueryClient();
  const who = useWho();
  const [adding, setAdding] = useState(false);
  const [name, setName] = useState("");
  const [kind, setKind] = useState<"branch" | "tag">("branch");
  const [parent, setParent] = useState("");
  const streams = useQuery({
    queryKey: ["streams", product],
    queryFn: async () =>
      unwrap(await api.GET("/v1/products/{product}/streams", { params: { path: { product } } })),
  });

  const declare = useMutation({
    mutationFn: async () =>
      unwrap(
        await api.POST("/v1/products/{product}/streams", {
          params: { path: { product } },
          body: {
            name: name.trim(),
            kind,
            ...(kind === "tag" && parent.trim() ? { parent: parent.trim() } : {}),
          },
        }),
      ),
    onSuccess: () => {
      setName("");
      setParent("");
      setAdding(false);
      void queries.invalidateQueries({ queryKey: ["streams", product] });
    },
  });

  if (streams.isPending) return <p className="hint">Loading…</p>;
  if (streams.isError) {
    return <Failed error={streams.error} what="The branches and tags could not be read." />;
  }

  const items = streams.data?.items ?? [];
  const branches = items.filter((s) => s.kind !== "tag").map((s) => s.name ?? "");
  return (
    <>
      <Crumbs product={product} />
      <div className="screen-head">
        <h2>Branches and tags</h2>
        <p>The releases of {product}, past and in progress. A branch moves; a tag never does.</p>
        {who.data?.admin && <AddButton label="Add branch or tag" onClick={() => setAdding(true)} />}
      </div>

      {items.length === 0 ? (
        <Empty
          title="Nothing is declared here yet."
          detail="A branch or a tag is declared before a scan can be filed against it."
        />
      ) : (
        <div className="tablewrap">
          <table>
            <thead>
              <tr>
                <th>Name</th>
                <th>Kind</th>
                <th>Cut from</th>
                <th className="num">Open</th>
                <th>Last inventory</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {items.map((stream) => (
                <tr key={stream.name} className="row">
                  <td>
                    <Link
                      to={`/products/${encodeURIComponent(product)}/streams/${encodeURIComponent(stream.name ?? "")}`}
                      className="id"
                    >
                      {stream.name}
                    </Link>
                  </td>
                  <td>
                    <span className={stream.kind === "tag" ? "state agreed" : "state open"}>
                      {stream.kind === "tag" ? "Tag" : "Branch"}
                    </span>
                  </td>
                  <td>{stream.parent ? <span className="id">{stream.parent}</span> : <span style={{ color: "var(--faint)" }}>—</span>}</td>
                  <td className="num">{(stream.open ?? 0).toLocaleString()}</td>
                  <td className="hint">{stream.last_scan_at ? stream.last_scan_at.slice(0, 10) : "never"}</td>
                  <td>
                    <Link
                      to={`/products/${encodeURIComponent(product)}/streams/${encodeURIComponent(stream.name ?? "")}`}
                      className="linkish"
                    >
                      Variants
                    </Link>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <Declare
        title="Add branch or tag"
        open={adding}
        onClose={() => setAdding(false)}
        onSubmit={() => declare.mutate()}
        error={declare.error}
        busy={declare.isPending || name.trim() === ""}
        ok={kind === "tag" ? "Add tag" : "Add branch"}
        hint="A tag names the branch it was cut from, which is what lets a decision made on the branch carry into it rather than being made again. A new branch inherits most of its triage by matching."
      >
        <div className="field">
          <label>Product</label>
          <input type="text" value={product} disabled />
        </div>
        <Field label="Name" value={name} onChange={setName} placeholder="202411" hint="How scans name it" />
        <div className="field">
          <label htmlFor="declare-kind">Kind</label>
          <select id="declare-kind" value={kind} onChange={(event) => setKind(event.target.value as "branch" | "tag")}>
            <option value="branch">Branch — moves, scanned nightly</option>
            <option value="tag">Tag — frozen, scanned on a schedule</option>
          </select>
        </div>
        {kind === "tag" && (
          <div className="field">
            <label htmlFor="declare-cut-from">Cut from</label>
            <select id="declare-cut-from" value={parent} onChange={(event) => setParent(event.target.value)}>
              <option value="">nothing — a line of its own</option>
              {branches.map((branch) => (
                <option key={branch} value={branch}>
                  {branch}
                </option>
              ))}
            </select>
          </div>
        )}
      </Declare>
    </>
  );
}
