import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

// Dev server proxies /api -> a local API (mock or real) exactly the way nginx
// does in the container (strip the /api prefix), so dev and prod behave the same.
export default defineConfig({
  plugins: [react(), tailwindcss()],
  build: {
    // The 18 hand-drawn SVG components (~1MB of traced path data) are inlined
    // into the bundle deliberately: one chunk, no asset waterfall, and path
    // data gzips ~4x. Raise the warning ceiling to match reality.
    chunkSizeWarningLimit: 1600,
  },
  server: {
    proxy: {
      "/api": {
        target: "http://localhost:8099",
        rewrite: (p) => p.replace(/^\/api/, ""),
      },
    },
  },
});
