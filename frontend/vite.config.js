import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// The build is embedded into the Go binary (internal/web/dist) and served under /static/.
// `npm run dev` proxies API calls to a trawl running on :8088.
const backend = "http://127.0.0.1:8088";

export default defineConfig({
  plugins: [react()],
  base: "/static/",
  build: {
    outDir: "../internal/web/dist",
    emptyOutDir: true,
    assetsInlineLimit: 0, // CSP: no inline data: scripts/fonts surprises
  },
  server: {
    proxy: Object.fromEntries(
      ["/api", "/suggest", "/img", "/favicon", "/opensearch.xml"].map((p) => [p, backend]).concat([
        // /read?format=md is served by Go; the reader page itself is the app
        ["^/read\\?.*format=md", backend],
      ]),
    ),
  },
});
