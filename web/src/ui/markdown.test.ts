import { describe, expect, it } from "vitest";
import { render } from "./markdown";

// The live markup a rendering produced. Checked instead of the whole string
// because escaped text is the correct outcome and contains the same words:
// `&lt;svg/onload=…&gt;` is a safe rendering of a dangerous input, and a check
// that could not tell the two apart would fail on the thing working.
function tagsIn(rendered: string): string[] {
  return rendered.match(/<[^>]*>/g) ?? [];
}

// What must not survive rendering, whatever route it takes in. Deliberately
// crude and deliberately broad: nothing executable, nothing that fetches, and
// no attribute a browser will run. A subtle check here would be a second place
// to get the rules wrong.
//
// Checked against the output rather than against the configuration — asking
// the allowlist whether it allows something proves only that it agrees with
// itself.
const forbidden = [
  "<script",
  "javascript:",
  "onerror",
  "onload",
  "onclick",
  "onmouseover",
  "onfocus",
  "<iframe",
  "<object",
  "<embed",
  "<svg",
  "<img",
  "data:text/html",
  "vbscript:",
  "<style",
  "<link",
  "<meta",
  "<base",
  "srcdoc",
  "formaction",
];

// The same corpus the server's renderer is held to, because the control moved
// here when rendering did. Markup and markdown that has been used to get
// script past sanitizers, plus the shapes specific to markdown itself.
const corpus = [
  `<script>alert(1)</script>`,
  `<img src=x onerror=alert(1)>`,
  `<svg/onload=alert(1)>`,
  `<iframe src="javascript:alert(1)"></iframe>`,
  `<body onload=alert(1)>`,
  `<a href="javascript:alert(1)">click</a>`,
  `[click](javascript:alert(1))`,
  `[click](JaVaScRiPt:alert(1))`,
  `[click](java&#115;cript:alert(1))`,
  `[click](\tjavascript:alert(1))`,
  `[click](data:text/html;base64,PHNjcmlwdD5hbGVydCgxKTwvc2NyaXB0Pg==)`,
  `![img](javascript:alert(1))`,
  `![img](https://evil.example/pixel.gif)`,
  `<a href="vbscript:msgbox(1)">x</a>`,
  `<div style="background:url(javascript:alert(1))">x</div>`,
  `<object data="data:text/html,<script>alert(1)</script>"></object>`,
  `<embed src="data:text/html,<script>alert(1)</script>">`,
  `<base href="https://evil.example/">`,
  `<meta http-equiv="refresh" content="0;url=javascript:alert(1)">`,
  `<link rel=stylesheet href="https://evil.example/x.css">`,
  `<form action="javascript:alert(1)"><button>go</button></form>`,
  `<button formaction="javascript:alert(1)">go</button>`,
  `<iframe srcdoc="&lt;script&gt;alert(1)&lt;/script&gt;"></iframe>`,
  "```<script>alert(1)</script>\ncode\n```",
  "```javascript:alert(1)\ncode\n```",
  '``` "><script>alert(1)</script>\ncode\n```',
  `<p onmouseover="alert(1)">hover</p>`,
  `<a href="#" onclick="alert(1)">x</a>`,
  `<input type="text" onfocus="alert(1)" autofocus>`,
  `<style>@import 'https://evil.example/x.css';</style>`,
  `<!--<script>alert(1)</script>-->`,
  `<math><mtext><script>alert(1)</script></mtext></math>`,
  `<xmp><script>alert(1)</script></xmp>`,
  `<noscript><p title="</noscript><script>alert(1)</script>">`,
  `&lt;script&gt;alert(1)&lt;/script&gt;`,
  `<scr<script>ipt>alert(1)</scr</script>ipt>`,
  `<a href=" javascript:alert(1)">x</a>`,
  `<a href="jav&#x0A;ascript:alert(1)">x</a>`,
];

describe("the renderer", () => {
  it("lets nothing in the corpus through", () => {
    for (const payload of corpus) {
      const rendered = render(payload);
      for (const tag of tagsIn(rendered)) {
        const lowered = tag.toLowerCase();
        for (const bad of forbidden) {
          expect(
            lowered,
            `${payload} survived as ${rendered} (live markup ${tag} carries ${bad})`,
          ).not.toContain(bad);
        }
      }
    }
  });

  it("fetches nothing from anywhere", () => {
    // An image fires from the browser of every person who reads the text,
    // from inside the network. On an undisclosed finding that is a disclosure
    // channel, so the element is not permitted at all.
    const out = render("![a](https://example.com/p.gif)\n\n<img src='https://example.com/q.gif'>");
    expect(out).not.toContain("<img");
    expect(out).not.toContain("example.com/p.gif");
  });

  it("keeps the links people actually write", () => {
    // The point of the allowlist is that it lets ordinary writing through.
    // A sanitizer that refuses real links is one people work around.
    const out = render(
      "See [the advisory](https://example.com/a) or [mail us](mailto:x@example.com).",
    );
    expect(out).toContain('href="https://example.com/a"');
    expect(out).toContain('href="mailto:x@example.com"');
    expect(out).toContain('rel="noreferrer noopener nofollow"');
  });

  it("drops a scheme that is merely harmless rather than permitted", () => {
    // SEC-13 narrows further than a sanitizer's own default does: only http,
    // https and mailto survive. A file or ftp link is not executable and would
    // pass a check that only looks for danger — this is the one that asserts
    // the allowlist is an allowlist rather than a denylist.
    // Asserted on the live markup, not the whole string: a destination the
    // parser declined to make a link of stays in the text, which is a safe
    // outcome that contains the same characters.
    for (const link of ["ftp://example.com/x", "file:///etc/passwd", "tel:+15551234"]) {
      for (const tag of tagsIn(render(`[x](${link})`))) {
        expect(tag, `${link} survived as ${tag}`).not.toContain("href=");
      }
    }
  });

  it("labels a language it knows and drops one it does not", () => {
    // An unknown language keeps its block and loses the label rather than
    // failing — refusing text over a language nobody listed would make the
    // tool argue with people about syntax highlighting.
    expect(render("```go\nx := 1\n```")).toContain('class="language-go"');
    const unknown = render("```nosuchlang\nx := 1\n```");
    expect(unknown).toContain("<code>");
    expect(unknown).not.toContain("nosuchlang");
    expect(unknown).toContain("x := 1");
  });

  it("renders ordinary markdown", () => {
    const out = render("# Title\n\nSome **bold** and `code`.\n\n- one\n- two");
    expect(out).toContain("<h1>");
    expect(out).toContain("<strong>");
    expect(out).toContain("<code>");
    expect(out).toContain("<li>");
  });
});

describe("files attached here", () => {
  const token = "0f9a1b2c3d4e5f60718293a4b5c6d7e8";

  it("renders an image of one from this origin", () => {
    // The content security policy permits images from this origin and no
    // other, so the reference becomes a path here rather than an address at
    // whoever's store the operator runs (ATT-13).
    const html = render(`![a screenshot](attachment:${token})`);
    expect(html).toContain(`src="/v1/attachments/${token}"`);
    expect(html).toContain('referrerpolicy="no-referrer"');
  });

  it("links to one without sending the reader away", () => {
    const html = render(`[the log](attachment:${token})`);
    expect(html).toContain(`href="/v1/attachments/${token}"`);
    // Ours, so not opened in another tab with a policy meant for somebody
    // else's site.
    expect(html).not.toContain('target="_blank"');
  });

  it("takes the whole element away from an image pointing anywhere else", () => {
    // Refused at submission, so this is text written before that rule — and a
    // source that is merely stripped leaves a broken-image icon in the middle
    // of somebody's reasoning.
    for (const source of [
      "![shot](https://example.org/s.png)",
      "![shot](data:image/png;base64,AAAA)",
      "![shot](/static/s.png)",
      "![shot](attachment:not-a-token)",
    ]) {
      const html = render(source);
      expect(html).not.toContain("<img");
      expect(html).not.toContain("example.org");
    }
  });

  it("does not resolve a reference shaped differently", () => {
    const html = render(`[x](attachment:${token.toUpperCase()})`);
    expect(html).not.toContain("/v1/attachments/");
  });
});

describe("identifiers people paste", () => {
  it("links a CVE to the record that defines it", () => {
    // UIX-24. The record rather than the enrichment most people mean by
    // "look up a CVE": they are different documents from different
    // organizations, and linking the summary as the source hides that they
    // disagree.
    const html = render("Fixed by the maintainer, see CVE-2026-31431.");
    expect(html).toContain('href="https://www.cve.org/CVERecord?id=CVE-2026-31431"');
    expect(html).toContain(">CVE-2026-31431</a>");
    expect(html).toContain('rel="noreferrer noopener nofollow"');
  });

  it("links a GitHub advisory identifier", () => {
    const html = render("See GHSA-cfh5-3ghh-wfjx for the write-up.");
    expect(html).toContain("https://github.com/advisories/GHSA-cfh5-3ghh-wfjx");
  });

  it("leaves alone anything that only looks like one", () => {
    // A link that lands on a record for the wrong thing costs more than no
    // link, because it is followed before it is disbelieved.
    for (const source of ["CVE-26-1", "CVE-2026-1", "NOTACVE-2026-31431", "GHSA-zzzz-zzzz-zzzz"]) {
      expect(render(source)).not.toContain("cve.org");
      expect(render(source)).not.toContain("github.com/advisories");
    }
  });

  it("does not link one inside code, or inside a link somebody wrote", () => {
    expect(render("Write `CVE-2026-31431` like that")).not.toContain("cve.org");
    expect(render("```\nCVE-2026-31431\n```\n")).not.toContain("cve.org");
    // A link inside a link is not something a browser renders sensibly.
    const nested = render("[CVE-2026-31431](https://example.org/x)");
    expect(nested).toContain("https://example.org/x");
    expect(nested).not.toContain("cve.org");
  });

  it("keeps the rest of the sentence around it", () => {
    const html = render("Before CVE-2026-31431 after.");
    expect(html).toContain("Before ");
    expect(html).toContain(" after.");
  });

  it("still lets nothing dangerous through", () => {
    // The autolinking runs after the sanitizer, so this checks that it did not
    // put back what the sanitizer had taken out. Asserted on the live markup
    // rather than on the string: escaped text is the correct outcome here and
    // contains the same words, which is what tagsIn exists to tell apart.
    const html = render("<img src=x onerror=alert(1)> CVE-2026-31431");
    for (const tag of tagsIn(html)) {
      expect(tag).not.toMatch(/onerror/i);
      expect(tag).not.toMatch(/^<img/i);
    }
    expect(html).toContain("cve.org");
  });
});
