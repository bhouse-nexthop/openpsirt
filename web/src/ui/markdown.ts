import MarkdownIt from "markdown-it";
import DOMPurify from "dompurify";

// Rendering moved here when the API stopped returning markup, so this carries
// the half of the policy that travels with rendering (DESIGN-text.md). The
// other half — what may be submitted at all, which links survive, what a
// reference resolves to — still runs on the server before anything is stored,
// because it needs data and authorization checks no browser holds.
//
// What is asserted about this is the *output*, not the configuration. Asking
// the allowlist whether it allows something proves only that it agrees with
// itself.

// The fenced-block tags that may reach a class attribute. The same list the
// server's sanitizer holds: a language tag is somebody's input, and three
// backticks followed by chosen text landing in markup is small and real.
//
// An unknown language keeps its block and loses the label rather than failing.
// Refusing to render over a language nobody listed would make the tool argue
// with people about syntax highlighting.
const LANGUAGES = new Set([
  "bash", "c", "cpp", "diff", "dockerfile", "go", "hcl", "ini", "java",
  "javascript", "json", "makefile", "markdown", "nginx", "none", "patch",
  "perl", "php", "python", "ruby", "rust", "shell", "sql", "text", "toml",
  "typescript", "xml", "yaml",
]);

const md = new MarkdownIt({
  // Raw markup is disabled at the parser rather than stripped afterwards.
  // Something never turned into a tag cannot be a tag that was missed.
  html: false,
  linkify: true,
  breaks: false,
  // The server emits the language as a class and stops there; so does this.
  // Colouring is applied afterwards, over already-sanitized markup.
  highlight: () => "",
});

// The language tag, allowlisted, as a class the highlighter can find.
md.renderer.rules.fence = (tokens, index) => {
  const token = tokens[index];
  if (!token) return "";
  const stated = (token.info || "").trim().split(/\s+/)[0]?.toLowerCase() ?? "";
  const language = LANGUAGES.has(stated) ? stated : "";
  const body = md.utils.escapeHtml(token.content);
  const attr = language ? ` class="language-${language}"` : "";
  return `<pre><code${attr}>${body}</code></pre>\n`;
};

// Nothing is fetched from anywhere, ever. An image fires from the browser of
// every person who reads the text, from inside the network, telling whoever
// wrote it who is looking at which finding and when — on an undisclosed
// finding that is a disclosure channel. The element is not permitted at all,
// rather than restricted by address, because the address is not the problem.
const ALLOWED_TAGS = [
  "p", "br", "hr", "strong", "em", "del", "code", "pre", "blockquote",
  "ul", "ol", "li", "a", "h1", "h2", "h3", "h4", "h5", "h6",
  "table", "thead", "tbody", "tr", "th", "td",
];

const ALLOWED_ATTR = ["href", "title", "class"];

// Only the schemes a link may use. Everything else is dropped, autolinked
// text included.
const SCHEMES = /^(?:https?:|mailto:)/i;

// Anything that leaves this is what a reader sees, so the class attribute is
// held to the one prefix the highlighter needs rather than left open — a class
// is a small thing to permit and a large thing to permit freely.
function tidy(node: Element) {
  if (node.tagName === "A") {
    const href = node.getAttribute("href") ?? "";
    if (!SCHEMES.test(href)) {
      node.removeAttribute("href");
    } else {
      // Nothing linked from here is ours. No referrer, and no handle back to
      // this window from whatever opens.
      node.setAttribute("rel", "noreferrer noopener nofollow");
      node.setAttribute("target", "_blank");
    }
  }
  const className = node.getAttribute("class");
  if (className !== null && !/^language-[a-z0-9+#-]+$/.test(className)) {
    node.removeAttribute("class");
  }
}

let hooked = false;
function hook() {
  if (hooked) return;
  DOMPurify.addHook("afterSanitizeAttributes", (node) => {
    if (node instanceof Element) tidy(node);
  });
  hooked = true;
}

// render turns markdown into markup a browser may be handed.
//
// Sanitized on the way out every time, never stored. A rule tightened next
// month then applies to text written last year, which it could not if the
// markup had been kept when the text arrived.
export function render(source: string): string {
  hook();
  return DOMPurify.sanitize(md.render(source), {
    ALLOWED_TAGS,
    ALLOWED_ATTR,
    // rel and target are set by the hook above rather than accepted from the
    // text, so they are not in ALLOWED_ATTR and cannot be supplied.
    ADD_ATTR: ["rel", "target"],
    ALLOW_DATA_ATTR: false,
    ALLOW_ARIA_ATTR: false,
  });
}
