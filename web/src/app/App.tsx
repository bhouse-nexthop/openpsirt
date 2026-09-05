import { Suspense, lazy, useSyncExternalStore } from "react";
import { Navigate, Route, Routes } from "react-router-dom";
import { useWho } from "./session";
import { belongTo } from "./drafts";
import { snapshot, subscribe } from "./ended";
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
const Tree = lazy(() => import("../screens/Tree").then((m) => ({ default: m.Tree })));
const Compare = lazy(() =>
  import("../screens/Compare").then((m) => ({ default: m.Compare })),
);
const People = lazy(() => import("../screens/People").then((m) => ({ default: m.People })));
const Work = lazy(() => import("../screens/Work").then((m) => ({ default: m.Work })));
const Queue = lazy(() => import("../screens/Queue").then((m) => ({ default: m.Queue })));
const Decision = lazy(() =>
  import("../screens/Decision").then((m) => ({ default: m.Decision })),
);
const Unassigned = lazy(() =>
  import("../screens/Unassigned").then((m) => ({ default: m.Unassigned })),
);
const Together = lazy(() =>
  import("../screens/Together").then((m) => ({ default: m.Together })),
);
const Inventories = lazy(() =>
  import("../screens/Inventories").then((m) => ({ default: m.Inventories })),
);
const Settings = lazy(() => import("../screens/Settings").then((m) => ({ default: m.Settings })));
const Audit = lazy(() => import("../screens/Audit").then((m) => ({ default: m.Audit })));

const build = "/products/:product/streams/:stream/variants/:variant";

export function App() {
  const who = useWho();
  const ended = useSyncExternalStore(subscribe, snapshot, snapshot);

  // Whose drafts this page reads and writes, decided here because this is the
  // one place that knows who is signed in and every screen below it takes the
  // answer for granted. Nobody recognized means no drafts are kept at all
  // rather than drafts kept under nobody's name (UIX-31).
  belongTo(who.data?.identity);

  if (who.isPending) return <Waiting />;

  // Nobody signed in. Not an error state — it is what a fresh browser looks
  // like, and the only thing to offer is a way in.
  if (!who.data) return <SignIn />;

  return (
    <>
      {ended && <Resume />}
      <Shell who={who.data}>
      <Suspense fallback={<p className="hint">Loading…</p>}>
      <Routes>
        <Route path="/" element={<Home who={who.data} />} />
        <Route path="/review-queue" element={<Queue />} />
        <Route path="/unassigned" element={<Unassigned />} />
        <Route path="/decisions/:id" element={<Decision who={who.data} />} />
        <Route path="/products" element={<Products who={who.data} />} />
        <Route path="/products/:product/streams" element={<Streams />} />
        <Route path="/products/:product/streams/:stream" element={<Variants />} />
        <Route path="/products/:product/variants" element={<Variants />} />
        {/* The list at whatever the picker selects, and the same screen at the
            address a build's other screens share (UIX-53). */}
        <Route path="/products/:product/findings" element={<Findings />} />
        <Route path={`${build}/findings`} element={<Findings />} />
        <Route path={`${build}/findings/:vulnerability/components/:component`} element={<Finding />} />
        <Route path={`${build}/components`} element={<Tree />} />
        <Route path={`${build}/components/:component/decide`} element={<Together />} />
        <Route path={`${build}/scans`} element={<Inventories />} />
        <Route path="/products/:product/comparison" element={<Compare />} />
        <Route path="/people" element={<People />} />
        <Route path="/work" element={<Work />} />
        <Route path="/audit" element={<Audit />} />
        <Route path="/settings" element={<Settings />} />
        {/* A path the page does not know either. Sending somebody home is
            better than a dead end, and the address bar already told them
            where they tried to go. */}
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
      </Suspense>
      </Shell>
    </>
  );
}

// Resume offers a way back in over the screen somebody was already on.
//
// Over it rather than instead of it: the words are safe either way, because a
// draft is written as it is typed, but the finding they were reading, the
// filters they had set and the row they had open are not — and a sign-in page
// that replaced all of it would throw those away for nothing. The way in
// carries the address of this screen, so the round trip through the provider
// comes back here (UIX-32).
//
// It is not dismissible. The session is gone; there is nothing behind this to
// do, and a control that closed it would only hide the reason the next thing
// somebody pressed did not work.
function Resume() {
  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-label="Your session ended"
      className="fixed inset-0 z-50 bg-[color-mix(in_srgb,var(--ink)_55%,transparent)] backdrop-blur-sm"
    >
      <SignIn resuming />
    </div>
  );
}

function Waiting() {
  return (
    <div className="flex min-h-dvh items-center justify-center">
      <p className="hint">Loading…</p>
    </div>
  );
}
