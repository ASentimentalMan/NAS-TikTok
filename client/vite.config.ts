import { fileURLToPath, URL } from "node:url";

import { defineConfig, loadEnv, type ConfigEnv } from "vite";
import vue from "@vitejs/plugin-vue";
import vueJsx from "@vitejs/plugin-vue-jsx";
import vueDevTools from "vite-plugin-vue-devtools";
import AutoImport from "unplugin-auto-import/vite";
import Components from "unplugin-vue-components/vite";
import { resolve } from "path";

export const createApiProxyRewrite = (baseUrl: string) => {
  const basePath = baseUrl.startsWith("/") && !baseUrl.startsWith("//")
    ? baseUrl.replace(/\/+$/, "")
    : "";
  return (requestPath: string) => `${basePath}${requestPath}`;
};

// https://vite.dev/config/
export default defineConfig(({ mode }: ConfigEnv) => {
  const pathSrc = fileURLToPath(new URL("./src", import.meta.url));
  const env = loadEnv(mode, process.cwd());
  const baseUrl = env.VITE_BASE_URL || "/";
  return {
    base: baseUrl,
    plugins: [
      vue(),
      vueJsx(),
      vueDevTools(),
      AutoImport({
        imports: ["vue", "vue-router", "pinia", "vue-i18n", "@vueuse/core"],
        dts: resolve(pathSrc, "types", "auto-imports.d.ts"),
      }),
      Components({
        dts: resolve(pathSrc, "types", "components.d.ts"),
      }),
    ],
    resolve: {
      alias: {
        "@": pathSrc,
      },
    },
    server: {
      host: "0.0.0.0",
      port: Number(env.VITE_PORT || 5173),
      open: env.VITE_OPEN === "true",
      proxy: {
        "/api": {
          target: env.VITE_API_PROXY || `http://127.0.0.1:${env.VITE_API_PORT || 3000}`,
          changeOrigin: false,
          rewrite: createApiProxyRewrite(baseUrl),
        },
      },
    },
  };
});
