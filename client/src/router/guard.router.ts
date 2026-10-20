import type { Router } from "vue-router";
import { checkSession } from "@/apis/index";

export const createRouterGuards = (router: Router) => {
  router.beforeEach(async (to, from, next) => {
    const whiteList = ['login'];
    if (whiteList.includes(to.name as string)) {
      next();
      return;
    }
    const session = await checkSession();
    if (session !== "unauthenticated") {
      next();
      return;
    }
    localStorage.removeItem("token");
    next({ name: "login", query: { redirect: to.fullPath }, replace: true });
  });

  router.onError((error) => {
    console.error("路由错误", error);
  });
};
