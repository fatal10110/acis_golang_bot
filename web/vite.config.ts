import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

// The build lands in the Go package that embeds it (internal/botpanel,
// //go:embed web); git ignores it. `npm run dev` proxies the API to a
// `bots ui` running on its default address.
export default defineConfig({
  plugins: [react(), tailwindcss()],
  build: {
    outDir: "../internal/botpanel/web/app",
    emptyOutDir: true,
    // No inline script or data: URL: the panel's CSP refuses both.
    modulePreload: { polyfill: false },
    assetsInlineLimit: 0,
  },
  server: {
    proxy: { "/api": "http://127.0.0.1:8090" },
  },
  test: {
    environment: "jsdom",
    setupFiles: ["./src/test/setup.ts"],
    css: false,
    restoreMocks: true,
  },
});
