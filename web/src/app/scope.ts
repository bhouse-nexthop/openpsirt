import { matchPath, useLocation } from "react-router-dom";

// What you are looking at.
//
// Read from the path rather than with useParams, because the frame is drawn
// outside the routes it wraps — useParams there is always empty, which is a
// silent kind of wrong: the rail and the picker never learn what was picked,
// and every screen below them works fine.
//
// A screen that names a build in its path is the authority for that build.
// Everything else — home, the queue, the product list — remembers the last one
// instead, so walking away from a build and back does not lose it.
const BUILD = "/products/:product/streams/:stream/variants/:variant";

const SHAPES = [
  `${BUILD}/*`,
  BUILD,
  "/products/:product/streams/:stream",
  "/products/:product/streams",
  "/products/:product",
];

export type Scoped = { product?: string; stream?: string; variant?: string };

const KEPT = "openpsirt.scope";

// Remembered for the tab rather than the browser: it is where somebody is
// working right now, not a preference, and a second tab looking at another
// product should not drag the first one with it.
export function remember(scope: Scoped) {
  try {
    window.sessionStorage.setItem(KEPT, JSON.stringify(scope));
  } catch {
    // A browser that refuses storage still works; it just forgets.
  }
}

function remembered(): Scoped {
  try {
    const kept = window.sessionStorage.getItem(KEPT);
    return kept ? (JSON.parse(kept) as Scoped) : {};
  } catch {
    return {};
  }
}

export function useScope(): Scoped {
  const { pathname } = useLocation();
  for (const shape of SHAPES) {
    const hit = matchPath(shape, pathname);
    if (hit) {
      const { product, stream, variant } = hit.params;
      const scope = { product, stream, variant };
      if (product) remember(scope);
      return scope;
    }
  }
  return remembered();
}

// where a scope change should land, given where somebody already is.
//
// Staying put is the point: changing what you are looking at is a property of
// the screen rather than a journey to another one, so a build-scoped screen
// swaps its build and everything else stays exactly where it was.
export function rescoped(pathname: string, to: Required<Scoped>): string | null {
  const hit = matchPath(`${BUILD}/*`, pathname) ?? matchPath(BUILD, pathname);
  if (!hit) return null;
  const rest = (hit.params as { "*"?: string })["*"] ?? "";
  const base =
    `/products/${encodeURIComponent(to.product)}` +
    `/streams/${encodeURIComponent(to.stream)}` +
    `/variants/${encodeURIComponent(to.variant)}`;
  return rest ? `${base}/${rest}` : `${base}/findings`;
}
