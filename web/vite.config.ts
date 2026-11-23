import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// During development, proxy API calls to the Go backend so the SPA and API
// share an origin from the browser's perspective.
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      "/healthz": "http://localhost:8080",
      "/readyz": "http://localhost:8080",
      "/api": "http://localhost:8080",
    },
  },
});
