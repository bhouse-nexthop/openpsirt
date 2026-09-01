import { Fragment, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useParams, useSearchParams } from "react-router-dom";
import { api } from "../api/client";
import { unwrap } from "../api/queries";
import { Failed } from "../ui/Failed";
import { Severity } from "../ui/Severity";
import { JUSTIFICATIONS } from "../ui/Outcome";
import { Editor, forget } from "../ui/Editor";
import { Reach } from "../ui/Reach";

// Everything a decision is made from, in one request. There may be thousands
// of findings and very few people, so the difference between a finding that
// carries its own evidence and one that sends somebody to a search engine is
// the difference between a queue that gets worked and one that does not.
export function Finding() {
  const { product = "", stream = "", variant = "", vulnerability = "", component = "" } = useParams();
  const [params] = useSearchParams();
  const version = params.get("version") ?? "";
  const at = { product, stream, variant, vulnerability, component };

  const finding = useQuery({
    queryKey: ["finding", { product, stream, variant }, vulnerability, component, version],
    queryFn: async () =>
      unwrap(
        await api.GET(
          "/v1/products/{product}/streams/{stream}/variants/{variant}/findings/{vulnerability}/components/{component}",
          { params: { path: at, query: { version } } },
        ),
      ),
  });

  if (finding.isPending) return <p className="hint">Loading…</p>;
  if (finding.isError) {
    return <Failed error={finding.error} what="This finding could not be read." />;
  }
  const it = finding.data;
  if (!it) return null;

  const back =
    `/products/${encodeURIComponent(product)}` +
    `/streams/${encodeURIComponent(stream)}` +
    `/variants/${encodeURIComponent(variant)}/findings`;

  return (
    <>
      <div className="screen-head">
        <h2>
          <span className="id">{it.vulnerability}</span> in <span className="id">{it.component}</span>{" "}
          {it.exploited && <span className="kev">Exploited</span>}
        </h2>
        <p>
          {product} · {stream} · {variant} · {(it.places ?? []).length}{" "}
          {(it.places ?? []).length === 1 ? "place" : "places"}{" "}
          <Link to={back} className="linkish">← back to findings</Link>
        </p>
      </div>

      {/* A version that moved and took the issue with it. Two things follow:
          the old reasoning probably still stands, and somebody's remediation
          did not land. */}
      {it.arrived_from && (
        <div className="alert" style={{ marginBottom: 14 }}>
          <strong>This was bumped and the issue came with it</strong>
          <br />
          <span>
            <span className="id">{it.arrived_from}</span> → <span className="id">{it.version}</span>
            {it.fixed_in && <> · <span className="id">{it.fixed_in}</span> is what fixes it</>}
            . The old reasoning probably still stands, and somebody's remediation did not land.
          </span>
        </div>
      )}

      <div className="evidence">
        {it.description && (
          <div className="block">
            <h4>What it is</h4>
            {/* What a scan file said is shown, never rendered (SEC-16). This
                came from a producer rather than from a person typing here. */}
            <p style={{ whiteSpace: "pre-wrap" }}>{it.description}</p>
          </div>
        )}

        <div className="block">
          <h4>How bad</h4>
          <div className="scores">
            <div className="score">
              <span className="n">{it.score ? it.score.toFixed(1) : "—"}</span>
              <span className="l">CVSS</span>
            </div>
            <div className="score">
              <span className="n">
                {typeof it.likelihood === "number" ? it.likelihood.toFixed(2) : "—"}
              </span>
              <span className="l">Likelihood</span>
            </div>
            <div className="score">
              <span className="n">{(it.weaknesses ?? [])[0] ?? "—"}</span>
              <span className="l">Weakness</span>
            </div>
            <div className="score">
              <span className="n">{it.exploited ? "Yes" : "No"}</span>
              <span className="l">Exploited</span>
            </div>
          </div>
          {it.vector && (
            <p className="mono" style={{ fontSize: "var(--step--1)", color: "var(--muted)" }}>
              {it.vector}
            </p>
          )}
          <p className="hint">
            <Severity word={it.severity} /> — being exploited is a fact
            about the world rather than a judgment, and it outranks the score.
          </p>
        </div>

        <References advisory={it.advisory} refs={it.references ?? []} />

        <div className="block">
          <h4>Upstream</h4>
          <p>
            <span className="id">{it.component} {it.version}</span>
            {it.upstream && <> — cut from <span className="id">{it.upstream}</span></>}
            {". "}
            {it.fix_state === "fixed" && it.fixed_in ? (
              <>Fixed upstream in <span className="id">{it.fixed_in}</span>
                {it.fixed_at && <>, available since {it.fixed_at.slice(0, 10)}</>}.</>
            ) : it.fix_state === "wont-fix" ? (
              <>Upstream has said it will not fix this.</>
            ) : (
              <>No fix has been published.</>
            )}
          </p>
        </div>

        <Places at={at} places={it.places ?? []} />

      </div>

      <div className="deciding">
        <Decide
          at={at}
          component={it.component ?? ""}
          version={it.version ?? ""}
          places={it.places ?? []}
        />

        {(it.aliases ?? []).length > 0 && (
          <div className="block">
            <h4>Also known as</h4>
            <p className="id">{(it.aliases ?? []).join(" · ")}</p>
            <p className="hint">
              Identity spans the names: whichever one a report used, it is the same issue here.
            </p>
          </div>
        )}
      </div>
    </>
  );
}

// Every place the component sits at. A place is the component and what
// directly pulled it in — the pair, not the route — and it is what a decision
// is recorded against.
type Sitting = {
  place: string;
  consumer?: string;
  decision?: number;
  suppressed?: boolean;
  chain?: { component: string; version?: string }[] | null;
};

// One judgment about this finding, covering every place it sits at.
//
// The same form the per-place screen uses — the same outcomes, the same
// reasons, the same editor with its draft kept and its preview through the
// renderer that publishes (UIX-21, UIX-22) — with one thing added: which of
// the places it covers. All of them unless somebody says otherwise (TRI-29,
// TRI-37).
//
// Built from the shared pieces rather than beside them. A second decision form
// that looked similar and behaved differently is how two screens come to
// disagree about what a decision is.
function Decide({
  at,
  component,
  version,
  places,
}: {
  at: { product: string; stream: string; variant: string; vulnerability: string };
  component: string;
  version: string;
  places: Sitting[];
}) {
  const queries = useQueryClient();
  const open = places.filter((place) => !place.decision);
  const [covering, setCovering] = useState<Set<string> | null>(null);
  const [outcome, setOutcome] = useState("not-applicable");
  const [justification, setJustification] = useState(JUSTIFICATIONS[0]?.value ?? "");
  const [mitigation, setMitigation] = useState("");
  const [until, setUntil] = useState("");
  const [reasoning, setReasoning] = useState("");

  // Everything still open, until somebody says otherwise.
  const chosen = covering ?? new Set(open.map((place) => place.place));
  const every = chosen.size === open.length;
  const draftKey = `decide:${at.product}:${at.vulnerability}:${component}`;

  const decide = useMutation({
    mutationFn: async () =>
      unwrap(
        await api.POST(
          "/v1/products/{product}/streams/{stream}/variants/{variant}/findings/{vulnerability}/components/{component}/decision",
          {
            params: { path: { ...at, component }, query: version ? { version } : {} },
            body: {
              outcome: outcome as "affected" | "not-applicable" | "deferred" | "wont-fix",
              reasoning,
              ...(outcome === "not-applicable"
                ? {
                    justification: justification as
                      | "component_not_present"
                      | "vulnerable_code_not_present"
                      | "vulnerable_code_not_in_execute_path"
                      | "vulnerable_code_cannot_be_controlled_by_adversary"
                      | "inline_mitigations_already_exist",
                  }
                : {}),
              ...(outcome === "deferred" ? { deferred_until: until } : {}),
              // Sent only when it narrows. Absent means all of them, which is
              // the server's default too — saying it twice invites the two to
              // disagree.
              ...(every ? {} : { places: [...chosen] }),
            },
          },
        ),
      ),
    onSuccess: () => {
      forget(draftKey);
      setCovering(null);
      setReasoning("");
      void queries.invalidateQueries({ queryKey: ["finding"] });
      void queries.invalidateQueries({ queryKey: ["queue"] });
    },
  });

  const answered = places.length - open.length;
  const needsJustification = outcome === "not-applicable";
  const needsDate = outcome === "deferred";
  const ready =
    reasoning.trim() !== "" &&
    chosen.size > 0 &&
    (!needsJustification || justification) &&
    (!needsJustification ||
      justification !== "inline_mitigations_already_exist" ||
      mitigation.trim() !== "") &&
    (!needsDate || until);

  if (open.length === 0) {
    return (
      <div className="card">
        <h3>Decide</h3>
        <p className="reading" style={{ margin: 0 }}>
          Every place this sits at has been answered. Revising one of those claims is done from
          the claim itself, so a second claim about the same code cannot stand beside the first.
        </p>
      </div>
    );
  }

  return (
    <div className="card">
      <h3>Decide</h3>
      {answered > 0 && (
        <p className="hint" style={{ margin: "-4px 0 12px" }}>
          {answered} of {places.length} places already answered. This covers what is left.
        </p>
      )}

      <div className="field">
        <label htmlFor="outcome">What is true here</label>
        <select id="outcome" value={outcome} onChange={(event) => setOutcome(event.target.value)}>
          <option value="not-applicable">It does not apply to us</option>
          <option value="affected">It applies and needs fixing</option>
          <option value="deferred">It applies, but not until a date</option>
          <option value="wont-fix">It applies and will not be fixed</option>
        </select>
      </div>

      {/* The claim that something does not affect us *is* which of the
          recognized reasons applies, so it is not a note beside the outcome —
          it is the outcome. */}
      {needsJustification && (
        <div className="field">
          <label htmlFor="justification">Which reason</label>
          <select
            id="justification"
            value={justification}
            onChange={(event) => setJustification(event.target.value)}
          >
            {JUSTIFICATIONS.map((each) => (
              <option key={each.value} value={each.value}>
                {each.label}
              </option>
            ))}
          </select>
        </div>
      )}

      {/* The one reason that rests on configuration rather than on code, and
          so the one nothing here will notice going away. Naming it does not
          fix that — it makes the claim something the next person can check
          (TRI-39). */}
      {needsJustification && justification === "inline_mitigations_already_exist" && (
        <div className="field">
          <label htmlFor="mitigation">What stops it</label>
          <input
            id="mitigation"
            type="text"
            value={mitigation}
            placeholder="the firewall rule, the setting, the service that is not exposed"
            onChange={(event) => setMitigation(event.target.value)}
          />
          <span className="hint">
            Nothing here watches configuration, so this claim will not lapse when the thing that
            stops it is removed. Say what to go and check.
          </span>
        </div>
      )}

      {needsDate && (
        <div className="field">
          <label htmlFor="until">Until when</label>
          <input
            id="until"
            type="date"
            value={until}
            onChange={(event) => setUntil(event.target.value)}
          />
          <span className="hint">A deferral with no date is a decision never to look again.</span>
        </div>
      )}

      <div className="field">
        <label>What this covers</label>
        <p className="hint" style={{ margin: "0 0 6px" }}>
          {chosen.size} of {open.length} {open.length === 1 ? "place" : "places"}, all of them
          unless you say otherwise. A place you untick stays open and nothing is recorded against
          it.
        </p>
        <div className="tree">
          {open.map((place) => (
            <label key={place.place} className="node" style={{ cursor: "pointer" }}>
              <input
                type="checkbox"
                checked={chosen.has(place.place)}
                onChange={(event) => {
                  const next = new Set(chosen);
                  if (event.target.checked) next.add(place.place);
                  else next.delete(place.place);
                  setCovering(next);
                }}
              />
              <span className="id">{place.consumer || UNPLACED}</span>
            </label>
          ))}
        </div>
      </div>

      {/* How far it travels beyond this build, said before it is recorded
          rather than discovered afterwards (TRI-30). Answerable for one place
          at a time, so it is shown when the claim covers one. */}
      {chosen.size === 1 && (
        <Reach at={{ ...at, place: [...chosen][0] ?? "" }} />
      )}
      {chosen.size > 1 && (
        <p className="hint">
          Each of these {chosen.size} places carries its own claim and reaches its own set of
          builds. Cover one at a time to see how far that one travels.
        </p>
      )}

      <div className="field">
        <label>Why</label>
        <Editor
          value={reasoning}
          onChange={setReasoning}
          draftKey={draftKey}
          label="Reasoning"
          mentions={{ product: at.product }}
          placeholder="What makes this true for this component, at these places?"
        />
      </div>

      {decide.error != null && (
        <Failed error={decide.error} what="That judgment could not be recorded." />
      )}
      {decide.data && (
        <div className="alert" style={{ marginBottom: 12 }}>
          <strong>
            Recorded against {decide.data.recorded}{" "}
            {decide.data.recorded === 1 ? "place" : "places"}
          </strong>
          <br />
          <span>
            {decide.data.needs_approval
              ? "Waiting for a second person to agree."
              : "In force now."}
            {(decide.data.left ?? 0) > 0 && (
              <>
                {" "}
                {decide.data.left} {decide.data.left === 1 ? "place is" : "places are"} still open,
                because they were not covered.
              </>
            )}
          </span>
        </div>
      )}

      <button
        type="button"
        className="btn"
        disabled={!ready || decide.isPending}
        onClick={() => decide.mutate()}
      >
        Record it
      </button>
      <span className="hint" style={{ marginLeft: 10 }}>
        Most outcomes wait for a second person before they take effect.
      </span>
    </div>
  );
}

// What a place is called when nothing pulled it in.
//
// Not "the product itself", which was the old wording and is a claim: it says
// the build depends on this directly. For a Go module vendored into a binary
// that is false, and the inventory never said it — a lockfile is not a
// filesystem, so the producer attaches what it finds there straight to the
// image because it has nowhere better to put it. What is actually known is
// that nothing recorded a consumer, and saying so points at the producer
// rather than inventing a relationship.
const UNPLACED = "nothing recorded what pulls this in";

function Places({
  at,
  places,
}: {
  at: { product: string; stream: string; variant: string; vulnerability: string };
  places: Sitting[];
}) {
  if (places.length === 0) return null;
  const build =
    `/products/${encodeURIComponent(at.product)}` +
    `/streams/${encodeURIComponent(at.stream)}` +
    `/variants/${encodeURIComponent(at.variant)}`;

  return (
    <div className="block">
      <h4>Where it sits</h4>
      {/* The complete chain, root to component, with the version at each step
          (UIX-14). The immediate parent alone cannot answer it: where a
          component is reached several ways the parent is often the same word
          twice, and two identical rows are not two answers. */}
      <div className="tree">
        {places.map((place) => {
          const chain = place.chain ?? [];
          // Nothing above it but the build. The chain would be two rows saying
          // nothing, so it says the one true thing instead.
          const bare = chain.length <= 1 || !place.consumer;
          if (bare) {
            return (
              <div key={place.place} className="node here">
                <span className="rule">└</span>
                <span className="id">{chain[chain.length - 1]?.component ?? ""}</span>
                <span className="hint">{UNPLACED}</span>
                {place.decision != null && (
                  <Link to={`/decisions/${place.decision}`} className="linkish">
                    decided
                  </Link>
                )}
              </div>
            );
          }
          return (
            <Fragment key={place.place}>
              {chain.map((step, depth) => {
                const last = depth === chain.length - 1;
                return (
                  <div
                    key={`${place.place} ${depth}`}
                    className={`node${last ? " here" : ""}`}
                    style={{ paddingLeft: depth * 18 }}
                  >
                    <span className="rule">└</span>
                    <span className="id">{step.component}</span>
                    {step.version && <span className="ver">{step.version}</span>}
                    {last && place.suppressed && (
                      <span className="state">the build already argued this away</span>
                    )}
                    {/* Only where a claim actually stands. A link on every row
                        saying "what was decided" answered a question nobody
                        asked, and told you nothing about whether anything had
                        been. */}
                    {last && place.decision != null && (
                      <Link to={`/decisions/${place.decision}`} className="linkish">
                        decided
                      </Link>
                    )}
                  </div>
                );
              })}
            </Fragment>
          );
        })}
      </div>
      <Link
        to={`${build}/components?at=${encodeURIComponent(places[0]?.chain?.at(-1)?.component ?? "")}`}
        className="linkish"
      >
        Show where this sits in the build →
      </Link>
      <p className="hint">
        One judgment covers every place running the same code at the same versions; a place at a
        different version is a separate judgment.
      </p>
    </div>
  );
}

// Patches first: for somebody deciding whether to backport rather than
// upgrade, the change itself is the answer and is the hardest thing to find.
function References({
  advisory,
  refs,
}: {
  advisory?: string;
  refs: { url?: string; kind?: string }[];
}) {
  const all = advisory ? [{ url: advisory, kind: "advisory" }, ...refs] : refs;
  if (all.length === 0) return null;
  const order: Record<string, number> = { patch: 0, advisory: 1, report: 2, other: 3 };
  const sorted = [...all].sort((a, b) => (order[a.kind ?? "other"] ?? 9) - (order[b.kind ?? "other"] ?? 9));
  return (
    <div className="block">
      <h4>Evidence</h4>
      <ul className="refs">
        {sorted.slice(0, 12).map((ref) => (
          <li key={ref.url}>
            <span className={ref.kind === "patch" ? "kind patch" : "kind"}>{ref.kind}</span>{" "}
            {/* Nothing linked from here is ours, so nothing carries the
                referrer or a handle back to this window. */}
            <a href={ref.url} target="_blank" rel="noreferrer noopener">
              {(ref.url ?? "").replace(/^https?:\/\//, "")}
            </a>
          </li>
        ))}
      </ul>
      <p className="hint">
        Patches first: for somebody deciding whether to backport rather than upgrade, the change
        itself is the answer.
      </p>
    </div>
  );
}
