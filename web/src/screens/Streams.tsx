import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useParams } from "react-router-dom";
import { api } from "../api/client";
import { unwrap } from "../api/queries";
import { Empty } from "../ui/Empty";
import { Failed } from "../ui/Failed";
import { Crumbs } from "../ui/Crumbs";
import { Declare, Field } from "../ui/Declare";
import { useWho } from "../app/session";

// A branch and a tag are different shapes of thing — one moves and is rebuilt,
// one never changes again — so they are labelled rather than blended into a
// single list of names.
export function Streams() {
  const { product = "" } = useParams();
  const queries = useQueryClient();
  const who = useWho();
  const [name, setName] = useState("");
  const [kind, setKind] = useState<"branch" | "tag">("branch");
  const [parent, setParent] = useState("");
  const streams = useQuery({
    queryKey: ["streams", product],
    queryFn: async () =>
      unwrap(
        await api.GET("/v1/products/{product}/streams", {
          params: { path: { product } },
        }),
      ),
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
      void queries.invalidateQueries({ queryKey: ["streams", product] });
    },
  });

  if (streams.isPending) return <p className="text-sm text-[var(--muted)]">Loading…</p>;
  if (streams.isError) {
    return <Failed error={streams.error} what="The branches and tags could not be read." />;
  }

  const items = streams.data?.items ?? [];
  return (
    <div>
      <Crumbs product={product} />
      <div className="screen-head">
        <h2>Branches and tags</h2>
        <p>The releases of a product, past and in progress. A branch moves; a tag never does.</p>
      </div>
      <Declare
        what="a branch or tag"
        hint="A tag names the branch it was cut from, which is what lets a decision made on the branch carry into it rather than being made again."
        onSubmit={() => declare.mutate()}
        error={declare.error}
        busy={declare.isPending || name.trim() === ""}
        can={who.data?.admin ?? false}
      >
        <Field label="Name" value={name} onChange={setName} placeholder="202411" hint="How scans name it" />
        <div className="field" style={{ margin: 0, minWidth: 130 }}>
          <label htmlFor="declare-kind">Kind</label>
          <select
            id="declare-kind"
            value={kind}
            onChange={(event) => setKind(event.target.value as "branch" | "tag")}
          >
            <option value="branch">branch — rebuilt</option>
            <option value="tag">tag — never changes</option>
          </select>
        </div>
        {kind === "tag" && (
          <Field
            label="Cut from"
            value={parent}
            onChange={setParent}
            placeholder="master"
            hint="The branch it was cut from"
          />
        )}
      </Declare>
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
                <th>Came from</th>
                <th className="num">Open</th>
                <th>Last scan</th>
              </tr>
            </thead>
            <tbody>
              {items.map((stream) => (
                <tr key={stream.name} className="row">
                  <td>
                    <Link
                      to={`/products/${encodeURIComponent(product)}/streams/${encodeURIComponent(stream.name)}`}
                      className="id"
                    >
                      {stream.name}
                    </Link>
                  </td>
                  <td><Kind kind={stream.kind} /></td>
                  <td className="hint">{stream.parent ?? "—"}</td>
                  <td className="num">{(stream.open ?? 0).toLocaleString()}</td>
                  {/* A line that has stopped being built looks identical to a
                      healthy one until somebody opens it. */}
                  <td className="hint">
                    {stream.last_scan_at ? stream.last_scan_at.slice(0, 10) : "never"}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

function Kind({ kind }: { kind?: string }) {
  return (
    <span className="rounded bg-[var(--raised)] px-1.5 py-0.5 text-xs text-[var(--muted)] ring-1 ring-inset ring-edge">
      {kind === "tag" ? "tag" : "branch"}
    </span>
  );
}
