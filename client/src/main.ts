import App from "./App.vue";
import { setupRouter } from "./router";
import "@/assets/styles/reset.less";
import "@/assets/styles/custom.less";

const bootstrap = () => {
  const app = createApp(App);
  setupRouter(app);

  app.mount("#app");
};

bootstrap();
