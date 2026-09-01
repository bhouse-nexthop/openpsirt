import { useQuery } from "@tanstack/react-query";
import { useParams } from "react-router-dom";
import { api } from "../api/client";
import { unwrap } from "../api/queries";
import { Empty } from "../ui/Empty";
import { Failed } from "../ui/Failed";

// What was measured, when, and whether it worked. A build pipeline finds out
// here whether its inventory was usable, because an upload answers before
// anything has been parsed — a success there means accepted, not valid.
//
// A product quietly dropping out of scanning is the failure that makes
// everything else wrong, so when something was last seen is the point of this
// screen rather than a detail on it.
export function Scans() {
  const { product = "", stream = "", variant = "" } = useParams();

  const scans = useQuery({
    queryKey: ["scans", product, stream, variant],
    queryFn: async () =>
      unwrap(
        await api.GET("/v1/products/{product}/streams/{stream}/variants/{variant}/scans", {
          params: { path: { product, stream, variant } },
        }),
      ),
  });

  if (scans.isPending) return <p className="hint">Loading…</p>;
  if (scans.isError) return <Failed error={scans.error} what="The scans could not be read." />;

  const items = scans.data?.items ?? [];

  return (
    <>
      <div className="screen-head">
        <h2>Scans</h2>
        <p>{product} · {stream} · {variant} — newest first.</p>
      </div>

      {items.length === 0 ? (
        <Empty
          title="Nothing has been sent here."
          detail="A build pipeline uploads an inventory, and what became of it appears here."
        />
      ) : (
        <div className="tablewrap">
          <table>
            <thead>
              <tr>
                <th>Received</th>
                <th>Built</th>
                <th>State</th>
                <th>Serial</th>
              </tr>
            </thead>
            <tbody>
              {items.map((scan) => (
                <tr key={scan.scan_id}>
                  <td>{scan.received_at?.replace("T", " ").slice(0, 16)}</td>
                  <td className="hint">{scan.built_at?.replace("T", " ").slice(0, 16)}</td>
                  <td>
                    <State state={scan.state} />
                    {scan.failure && (
                      <div className="hint" style={{ marginTop: 4 }}>{scan.failure}</div>
                    )}
                  </td>
                  <td className="id hint">{scan.serial}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <p className="hint" style={{ marginTop: 12 }}>
        An upload answers before anything is parsed, so a success there means accepted rather
        than valid. This is where a pipeline finds out which it was.
      </p>
    </>
  );
}

// The four states an upload passes through, and the one it can end in.
function State({ state }: { state?: string }) {
  const colour: Record<string, string> = {
    reading: "var(--wait)",
    scanning: "var(--wait)",
    scanned: "var(--ok)",
    failed: "var(--sev-critical)",
  };
  const means: Record<string, string> = {
    reading: "accepted, not yet parsed",
    scanning: "parsed; the vulnerability scan is still running",
    scanned: "complete",
    failed: "it did not finish, and the reason is beside it",
  };
  return (
    <span className="chip" title={means[state ?? ""]} style={{ color: colour[state ?? ""] ?? "var(--muted)" }}>
      {state}
    </span>
  );
}
