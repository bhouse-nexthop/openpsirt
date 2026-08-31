import createClient from "openapi-fetch";
import type { paths } from "./schema";

// One client for the whole application, generated from the same OpenAPI
// document the server publishes (UIX-19). Nothing here hand-writes a path or a
// response shape, so an endpoint that changes shape is a compile error rather
// than a screen that renders undefined.
export const api = createClient<paths>({
  baseUrl: "/",
  // The session is a cookie the browser holds; nothing in this application
  // ever sees the token. Sending credentials is therefore the whole of
  // authentication here.
  credentials: "same-origin",
});

// csrfCookie is the value a page has to echo on a write. It is deliberately
// readable by script, where the session cookie is not — that asymmetry is what
// makes echoing it evidence the request came from a page rather than from a
// form somebody else's site submitted.
function csrfCookie(): string {
  for (const part of document.cookie.split(";")) {
    const [name, ...rest] = part.trim().split("=");
    if (name === "openpsirt_csrf") return decodeURIComponent(rest.join("="));
  }
  return "";
}

// Every unsafe request carries the token. Registered as middleware rather than
// passed per call, because the one call somebody forgets is the one that
// breaks in production and not in review.
api.use({
  onRequest({ request }) {
    if (request.method !== "GET" && request.method !== "HEAD") {
      const token = csrfCookie();
      if (token) request.headers.set("X-CSRF-Token", token);
    }
    return request;
  },
});
