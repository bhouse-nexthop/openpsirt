/// <reference types="vitest" />
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

// The built assets are embedded into the Go binary and served from it, so the
// output lands where //go:embed can see it and nothing is fetched from a CDN
// at run time — an air-gapped install is a normal install for this tool.
export default defineConfig({
  plugins: [react(), tailwindcss()],
  build: { outDir: "dist", emptyOutDir: true, assetsDir: "assets" },
  // The renderer carries the sanitizing that used to run on the server, so it
  // is tested against the same corpus of payloads. DOMPurify needs a DOM to
  // sanitize in, which is what jsdom is here for.
  test: { environment: "jsdom", include: ["src/**/*.test.ts"] },
  server: {
    // In development the API is a separate process. Same-origin in production,
    // so nothing here needs CORS and no origin is configured in two places.
    proxy: { "/v1": "http://localhost:8080" },
  },
});
