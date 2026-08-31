import { Navigate, Route, Routes } from "react-router-dom";
import { useWho } from "./session";
import { Shell } from "./Shell";
import { SignIn } from "../screens/SignIn";
import { Findings } from "../screens/Findings";
import { Products } from "../screens/Products";

export function App() {
  const who = useWho();

  if (who.isPending) return <Waiting />;

  // Nobody signed in. Not an error state — it is what a fresh browser looks
  // like, and the only thing to offer is a way in.
  if (!who.data) return <SignIn />;

  return (
    <Shell who={who.data}>
      <Routes>
        <Route path="/" element={<Navigate to="/products" replace />} />
        <Route path="/products" element={<Products who={who.data} />} />
        <Route
          path="/products/:product/streams/:stream/variants/:variant/findings"
          element={<Findings />}
        />
        <Route path="*" element={<Navigate to="/products" replace />} />
      </Routes>
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
