import { fileURLToPath, URL } from "node:url";

import { ConfigEnv, defineConfig, loadEnv } from "vite";
import vue from "@vitejs/plugin-vue";
import vueJsx from "@vitejs/plugin-vue-jsx";
import vueDevTools from "vite-plugin-vue-devtools";
import AutoImport from "unplugin-auto-import/vite";
import Components from "unplugin-vue-components/vite";
import { resolve } from "path";

// https://vite.dev/config/
export default defineConfig(({ mode }: ConfigEnv) => {
  const pathSrc = fileURLToPath(new URL("./src", import.meta.url));
  const env = loadEnv(mode, process.cwd());
  return {
    base: env.VITE_BASE_URL,
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
      port: env.VITE_PORT as unknown as number,
      open: env.VITE_OPEN === "true",
    },
  };
});
