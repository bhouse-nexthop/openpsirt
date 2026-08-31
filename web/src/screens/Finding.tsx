import { useQuery } from "@tanstack/react-query";
import { Link, useParams } from "react-router-dom";
import { api } from "../api/client";
import { unwrap } from "../api/queries";
import { Failed } from "../ui/Failed";
import { Crumbs } from "../ui/Crumbs";
import { Severity } from "../ui/Severity";

// Everything a decision is made from, in one request. There may be thousands
// of findings and very few people, so the difference between a finding that
// carries its own evidence and one that sends somebody to a search engine is
// the difference between a queue that gets worked and one that does not.
export function Finding() {
  const { product = "", stream = "", variant = "", vulnerability = "", component = "" } = useParams();
  const at = { product, stream, variant, vulnerability, component };

  const finding = useQuery({
    queryKey: ["finding", at],
    queryFn: async () =>
      unwrap(
        await api.GET(
          "/v1/products/{product}/streams/{stream}/variants/{variant}/findings/{vulnerability}/components/{component}",
          { params: { path: at } },
        ),
      ),
  });

  if (finding.isPending) return <p className="text-sm text-muted">Loading…</p>;
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
    <div className="max-w-3xl">
      <Crumbs product={product} stream={stream} variant={variant} />
      <Link to={back} className="mb-3 inline-block text-sm text-muted hover:text-ink">
        ← All findings
      </Link>

      <header className="mb-5">
        <div className="flex flex-wrap items-center gap-2">
          <h1 className="text-xl font-semibold tracking-tight">{it.vulnerability}</h1>
          <Severity word={it.severity} exploited={it.exploited} />
          {it.score ? <Rated score={it.score} vector={it.vector} /> : null}
        </div>
        <p className="mt-1 text-sm text-muted">
          {it.component} {it.version}
          {it.upstream && ` · forked from ${it.upstream}`}
        </p>
        {(it.aliases ?? []).length > 0 && (
          <p className="mt-1 text-sm text-muted">also known as {(it.aliases ?? []).join(", ")}</p>
        )}
      </header>

      {/* What a scan file said is shown, never rendered (SEC-16). This text
          came from a producer rather than from a person typing into this tool,
          so it is displayed as text and nothing in it is markup. */}
      {it.description && (
        <section className="mb-5">
          <p className="whitespace-pre-wrap text-sm">{it.description}</p>
        </section>
      )}

      <dl className="mb-6 grid gap-x-6 gap-y-2 text-sm sm:grid-cols-2">
        <Fact label="Upstream has" value={fixWord(it.fix_state, it.fixed_in)} />
        {it.arrived_from && (
          <Fact
            label="Bumped from"
            value={`${it.arrived_from} — and the issue came with it`}
          />
        )}
        {typeof it.likelihood === "number" && (
          <Fact label="Chance of exploitation" value={`${Math.round(it.likelihood * 100)}%`} />
        )}
        {(it.weaknesses ?? []).length > 0 && (
          <Fact label="Kind of flaw" value={(it.weaknesses ?? []).join(", ")} />
        )}
      </dl>

      <Places at={at} places={it.places ?? []} />

      <References advisory={it.advisory} refs={it.references ?? []} />
    </div>
  );
}

function Rated({ score, vector }: { score: number; vector?: string }) {
  return (
    <span className="text-sm text-muted" title={vector || undefined}>
      {score.toFixed(1)}
    </span>
  );
}

function fixWord(state?: string, version?: string): string {
  if (state === "fixed") return version ? `fixed in ${version}` : "fixed upstream";
  if (state === "wont-fix") return "said it will not fix this";
  return "no fix published";
}

function Fact({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <dt className="text-muted">{label}</dt>
      <dd>{value}</dd>
    </div>
  );
}

// Every place the component sits at here. A place is the component and what
// directly pulled it in — the pair, not the whole route — and it is what a
// decision is recorded against.
function Places({
  at,
  places,
}: {
  at: { product: string; stream: string; variant: string; vulnerability: string };
  places: { place: string; consumer?: string; suppressed?: boolean }[];
}) {
  if (places.length === 0) return null;
  return (
    <section className="mb-6">
      <h2 className="mb-2 text-sm font-semibold">
        Where it sits
        <span className="ml-2 font-normal text-muted">
          {places.length} {places.length === 1 ? "place" : "places"}
        </span>
      </h2>
      <ul className="divide-y divide-edge overflow-hidden rounded-lg border border-edge">
        {places.map((place) => (
          <li key={place.place} className="flex flex-wrap items-center gap-2 bg-raised px-3 py-2 text-sm">
            <span>{place.consumer ? `under ${place.consumer}` : "under the product itself"}</span>
            {place.suppressed && (
              <span className="rounded bg-sunken px-1.5 py-0.5 text-xs text-muted ring-1 ring-inset ring-edge">
                the build already argued this away
              </span>
            )}
            <Link
              to={
                `/products/${encodeURIComponent(at.product)}` +
                `/streams/${encodeURIComponent(at.stream)}` +
                `/variants/${encodeURIComponent(at.variant)}` +
                `/findings/${encodeURIComponent(at.vulnerability)}` +
                `/places/${encodeURIComponent(place.place)}`
              }
              className="ml-auto text-accent hover:underline"
            >
              What was decided
            </Link>
          </li>
        ))}
      </ul>
    </section>
  );
}

// Patches first, because the commit that fixes something is what somebody
// triaging most often wants and is the hardest to find by searching.
function References({ advisory, refs }: { advisory?: string; refs: { url: string; kind: string }[] }) {
  const all = advisory ? [{ url: advisory, kind: "advisory" }, ...refs] : refs;
  if (all.length === 0) return null;
  const order = { patch: 0, advisory: 1, report: 2, other: 3 } as Record<string, number>;
  const sorted = [...all].sort((a, b) => (order[a.kind] ?? 9) - (order[b.kind] ?? 9));
  return (
    <section>
      <h2 className="mb-2 text-sm font-semibold">References</h2>
      <ul className="flex flex-col gap-1 text-sm">
        {sorted.map((ref) => (
          <li key={ref.url} className="flex items-baseline gap-2">
            <span className="w-16 shrink-0 text-xs text-muted">{ref.kind}</span>
            {/* Nothing this tool links to is ours, so nothing carries the
                referrer or a handle back to this window. */}
            <a
              href={ref.url}
              target="_blank"
              rel="noreferrer noopener"
              className="break-all text-accent hover:underline"
            >
              {ref.url}
            </a>
          </li>
        ))}
      </ul>
    </section>
  );
}
