import { Suspense, lazy } from "react";
import { Navigate, Route, Routes } from "react-router-dom";
import { useWho } from "./session";
import { Shell } from "./Shell";
import { SignIn } from "../screens/SignIn";
import { Findings } from "../screens/Findings";
import { Products } from "../screens/Products";
import { Streams } from "../screens/Streams";
import { Variants } from "../screens/Variants";

// Split by route, so a screen carries the weight of what it actually needs.
// The findings list has to stay usable against a full-size product and has no
// business downloading a charting library; the markdown renderer is only
// needed where somebody reads or writes a justification.
const Home = lazy(() => import("../screens/Home").then((m) => ({ default: m.Home })));
const Finding = lazy(() => import("../screens/Finding").then((m) => ({ default: m.Finding })));
const PlaceDecision = lazy(() =>
  import("../screens/PlaceDecision").then((m) => ({ default: m.PlaceDecision })),
);
const Tree = lazy(() => import("../screens/Tree").then((m) => ({ default: m.Tree })));
const Queue = lazy(() => import("../screens/Queue").then((m) => ({ default: m.Queue })));

const build = "/products/:product/streams/:stream/variants/:variant";

export function App() {
  const who = useWho();

  if (who.isPending) return <Waiting />;

  // Nobody signed in. Not an error state — it is what a fresh browser looks
  // like, and the only thing to offer is a way in.
  if (!who.data) return <SignIn />;

  return (
    <Shell who={who.data}>
      <Suspense fallback={<p className="text-sm text-muted">Loading…</p>}>
      <Routes>
        <Route path="/" element={<Home who={who.data} />} />
        <Route path="/review-queue" element={<Queue />} />
        <Route path="/products" element={<Products who={who.data} />} />
        <Route path="/products/:product/streams" element={<Streams />} />
        <Route path="/products/:product/streams/:stream" element={<Variants />} />
        <Route path={`${build}/findings`} element={<Findings />} />
        <Route path={`${build}/findings/:vulnerability/components/:component`} element={<Finding />} />
        <Route path={`${build}/findings/:vulnerability/places/:place`} element={<PlaceDecision />} />
        <Route path={`${build}/components`} element={<Tree />} />
        {/* A path the page does not know either. Sending somebody home is
            better than a dead end, and the address bar already told them
            where they tried to go. */}
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
      </Suspense>
    </Shell>
  );
}

function Waiting() {
  return (
    <div className="flex min-h-dvh items-center justify-center">
      <p className="text-sm text-muted">Loading…</p>
    </div>
  );
}
