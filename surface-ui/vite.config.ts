import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import { fileURLToPath, URL } from "node:url";

// Local-dev only: when VITE_TOOLKIT_SRC=1, resolve the toolkit to its SOURCE
// instead of the published dist, so Vite processes it as app code — that gives
// HMR on the linked lib and lets import.meta.env reach the toolkit. Off by
// default (CI/prod use the published dist).
const TOOLKIT_SRC_ALIAS = process.env.VITE_TOOLKIT_SRC
  ? [
      // The "/styles" subpath must come FIRST (more specific) — Vite matches
      // aliases in order and the bare-name entry would otherwise swallow it.
      {
        find: "@dakasa-yggdrasil/surface-toolkit/styles",
        replacement: fileURLToPath(new URL("../../surface-toolkit/src/theme/tokens.css", import.meta.url))
      },
      {
        find: "@dakasa-yggdrasil/surface-toolkit",
        replacement: fileURLToPath(new URL("../../surface-toolkit/src/index.ts", import.meta.url))
      }
    ]
  : [];

// Proxy /api/v1/* to the staging Yggdrasil so local dev sees real instances +
// surface queries without needing a local core.
const YGGDRASIL_TARGET = process.env.VITE_YGGDRASIL_URL ?? "https://yggdrasil.dakasa.me";
// Optional dev-only admin token so the proxied requests succeed without a
// SSO session. Read from VITE_DEV_ADMIN_TOKEN — never bundled into prod.
const DEV_ADMIN_TOKEN = process.env.VITE_DEV_ADMIN_TOKEN;

export default defineConfig({
  plugins: [react()],
  resolve: { alias: TOOLKIT_SRC_ALIAS },
  base: process.env.VITE_BASE_PATH ?? "/s/efi/",
  build: { sourcemap: true },
  server: {
    proxy: {
      "/api": {
        target: YGGDRASIL_TARGET,
        changeOrigin: true,
        secure: true,
        configure: (proxy) => {
          if (DEV_ADMIN_TOKEN) {
            proxy.on("proxyReq", (proxyReq) => {
              proxyReq.setHeader("Authorization", `Bearer ${DEV_ADMIN_TOKEN}`);
            });
          }
        }
      }
    }
  }
});
