import { useEffect, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useLocation, useNavigate } from "react-router-dom";
import { api } from "../api/client";
import { unwrap } from "../api/queries";
import { remember, rescoped, useScope } from "./scope";

// What you are looking at, and how to change it.
//
// The choosing happens inside the panel rather than by walking through
// listing screens: picking a product narrows the branches beside it, picking a
// branch narrows the variants, and only the last choice moves you. Sending
// somebody to a page of products to pick one makes changing scope a
// navigation, when it is a property of the screen they are already on.
export function Scope() {
  const at = useScope();
  const [open, setOpen] = useState(false);
  const box = useRef<HTMLDivElement>(null);
  const navigate = useNavigate();
  const { pathname } = useLocation();

  // What has been picked in here but not yet gone anywhere. Seeded from the
  // address so opening the panel starts where you are.
  const [product, setProduct] = useState(at.product ?? "");
  const [stream, setStream] = useState(at.stream ?? "");

  useEffect(() => {
    setProduct(at.product ?? "");
    setStream(at.stream ?? "");
  }, [at.product, at.stream]);

  useEffect(() => {
    if (!open) return;
    function away(event: MouseEvent) {
      if (box.current && !box.current.contains(event.target as Node)) setOpen(false);
    }
    function key(event: KeyboardEvent) {
      if (event.key === "Escape") setOpen(false);
    }
    document.addEventListener("mousedown", away);
    document.addEventListener("keydown", key);
    return () => {
      document.removeEventListener("mousedown", away);
      document.removeEventListener("keydown", key);
    };
  }, [open]);

  const products = useQuery({
    queryKey: ["products"],
    enabled: open,
    queryFn: async () => unwrap(await api.GET("/v1/products", {})),
  });
  const streams = useQuery({
    queryKey: ["streams", product],
    enabled: open && !!product,
    queryFn: async () =>
      unwrap(await api.GET("/v1/products/{product}/streams", { params: { path: { product } } })),
  });
  const variants = useQuery({
    queryKey: ["variants", product, stream],
    enabled: open && !!product && !!stream,
    queryFn: async () =>
      unwrap(
        await api.GET("/v1/products/{product}/streams/{stream}/variants", {
          params: { path: { product, stream } },
        }),
      ),
  });

  function go(variant: string) {
    setOpen(false);
    // Stay where you are. A screen that names a build swaps its build and
    // keeps doing whatever it was doing; anything else simply remembers the
    // choice, because changing scope is not a reason to move somebody.
    const next = rescoped(pathname, { product, stream, variant });
    if (next) {
      navigate(next);
      return;
    }
    remember({ product, stream, variant });
    // A re-render is needed for the bar to catch up with what was remembered.
    navigate(pathname, { replace: true });
  }

  return (
    <div className="scopebar" ref={box}>
      {/* All three, always. Which one is pressed does not matter — they open
          the same panel — but a control that appears only once its parent is
          chosen hides that there is a choice to make at all. */}
      <button type="button" className="scope" aria-expanded={open} onClick={() => setOpen(!open)}>
        <span className="label">Product</span>
        {at.product || "pick one"}
        <span className="caret">▾</span>
      </button>
      <span className="sep">/</span>
      <button
        type="button"
        className={at.product ? "scope" : "scope only"}
        aria-expanded={open}
        onClick={() => setOpen(!open)}
      >
        {at.stream || (at.product ? "branch or tag" : "—")}
        <span className="caret">▾</span>
      </button>
      <span className="sep">/</span>
      <button
        type="button"
        className={at.stream ? "scope" : "scope only"}
        aria-expanded={open}
        onClick={() => setOpen(!open)}
      >
        {at.variant || (at.stream ? "variant" : "—")}
        <span className="caret">▾</span>
      </button>

      <div className={open ? "picker open" : "picker"}>
        <div>
          <h5>Product</h5>
          {(products.data?.items ?? []).map((each) => (
            <button
              key={each.name}
              type="button"
              className="opt"
              aria-current={each.name === product ? "true" : undefined}
              onClick={() => {
                setProduct(each.name ?? "");
                // A branch belongs to a product, so changing the product
                // cannot leave the one beside it standing.
                setStream("");
              }}
            >
              {each.display_name || each.name}
            </button>
          ))}
          {products.data && (products.data.items ?? []).length === 0 && (
            <p className="hint">You can reach no product yet.</p>
          )}
        </div>

        <div>
          <h5>Branch or tag</h5>
          {!product && <p className="hint">Pick a product first.</p>}
          {(streams.data?.items ?? []).map((each) => (
            <button
              key={each.name}
              type="button"
              className="opt"
              aria-current={each.name === stream ? "true" : undefined}
              onClick={() => setStream(each.name ?? "")}
            >
              {each.name} <span className="hint">{each.kind}</span>
            </button>
          ))}
          {product && streams.data && (streams.data.items ?? []).length === 0 && (
            <p className="hint">Nothing is declared under this product yet.</p>
          )}
        </div>

        <div>
          <h5>Variant</h5>
          {!stream && <p className="hint">Pick a branch or tag first.</p>}
          {(variants.data?.items ?? []).map((each) => (
            <button
              key={each.name}
              type="button"
              className="opt"
              aria-current={each.name === at.variant ? "true" : undefined}
              onClick={() => go(each.name ?? "")}
            >
              {each.name}
            </button>
          ))}
          {stream && variants.data && (variants.data.items ?? []).length === 0 && (
            <p className="hint">Nothing has been scanned on this line yet.</p>
          )}
        </div>

        <p className="hint" style={{ gridColumn: "1 / -1" }}>
          A variant appears once a build has filed a scan against it, so a release that predates
          one does not list it. Choosing one takes you to its findings.
        </p>
      </div>
    </div>
  );
}
