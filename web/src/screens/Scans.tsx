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

  // Every build of this product, so a build that has gone silent is named
  // here rather than being invisible on the screen about scanning. The one in
  // front of you is the likeliest thing to be looking at and the least likely
  // to be the problem: a build nobody is filing against is one nobody is
  // looking at either.
  const scanning = useQuery({
    queryKey: ["scanning", product],
    queryFn: async () =>
      unwrap(await api.GET("/v1/scanning", { params: { query: { product } } })),
  });
  const quiet = (scanning.data?.items ?? []).filter((b) => b.quiet);

  if (scans.isPending) return <p className="hint">Loading…</p>;
  if (scans.isError) return <Failed error={scans.error} what="The scans could not be read." />;

  const items = scans.data?.items ?? [];

  return (
    <>
      <div className="screen-head">
        <h2>Scans</h2>
        <p>{product} · {stream} · {variant} — newest first.</p>
      </div>

      {quiet.map((build) => (
        <div className="alert" key={`${build.stream}\u0000${build.variant}`}>
          <strong>
            {build.stream} · {build.variant} has not been scanned
            {build.last_received_at ? ` for ${build.quiet_days} days` : " at all"}
          </strong>
          <br />
          <span>
            {build.last_received_at
              ? "Nothing has failed — nothing has arrived. A build that stops being scanned looks healthy, because no new findings appear against it."
              : `Declared ${build.quiet_days} days ago, and nothing has ever been filed against it.`}
          </span>
        </div>
      ))}

      {/* What the numbers on every other screen were arrived at with. Without
          it, a build with nothing wrong and a build last measured against a
          months-old vulnerability database read identically. */}
      {scans.data?.measured_against && (
        <p className="hint" style={{ marginTop: -6 }}>
          Last measured by <span className="id">{scans.data.measured_against.scanner}</span>
          {scans.data.measured_against.scanner_version && (
            <> {scans.data.measured_against.scanner_version}</>
          )}
          {scans.data.measured_against.database_version && (
            <>, against vulnerability data{" "}
              <span className="id">{scans.data.measured_against.database_version}</span></>
          )}
          {scans.data.measured_against.ran_at && (
            <>, on {scans.data.measured_against.ran_at.slice(0, 10)}</>
          )}
          .
        </p>
      )}

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
                {/* What the run changed. A row saying only "scanned" leaves
                    the thing worth knowing — whether anything moved — to be
                    worked out from the findings list. */}
                <th className="num">Opened</th>
                <th className="num">Closed</th>
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
                  {/* Counted as issues at components, the way the findings
                      list counts, rather than as places. A run covers a build
                      rather than an upload, so where several uploads share one
                      run the numbers sit on the newest of them and the rest
                      are blank rather than repeating it. */}
                  <td className="num">{scan.opened || (scan.state === "scanned" ? "—" : "")}</td>
                  <td className="num">{scan.closed || (scan.state === "scanned" ? "—" : "")}</td>
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
