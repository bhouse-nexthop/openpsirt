// Which of the three looks this browser draws (UIX-50).
//
// A look is a token set on the root element and nothing else — the markup
// is the same under all three. It is a personal preference kept in the
// browser, the same rule as saved filters: it changes what one person sees
// and nothing anybody else is shown.

export const LOOKS = [
  { name: "dojo", label: "Dojo", said: "dark rail, light surface", a: "#141a23", b: "#2b62e3" },
  { name: "ledger", label: "Ledger", said: "all light, hairline", a: "#ffffff", b: "#0f766e" },
  { name: "obsidian", label: "Obsidian", said: "dark throughout", a: "#0e1117", b: "#6ea8ff" },
] as const;

export type Look = (typeof LOOKS)[number]["name"];

const KEPT = "openpsirt.look";

export function currentLook(): Look {
  try {
    const kept = window.localStorage.getItem(KEPT);
    if (kept && LOOKS.some((each) => each.name === kept)) return kept as Look;
  } catch {
    // A browser that refuses storage gets the default.
  }
  return "dojo";
}

export function applyLook(look: Look) {
  document.documentElement.setAttribute("data-look", look);
  try {
    window.localStorage.setItem(KEPT, look);
  } catch {
    // The look still applies for this page.
  }
}
