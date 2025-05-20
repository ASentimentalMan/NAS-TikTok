import type { Router } from "vue-router";

export const createRouterGuards = (router: Router) => {
  router.beforeEach(async (to, from, next) => {
    const token = localStorage.getItem('token');
    if (token) {
      next();
    } else {
      const whiteList = ['login'];
      if (whiteList.includes(to.name as string)) {
        next();
      } else {
        next({
          name: "login",
          query: { redirect: to.fullPath },
          replace: true,
        });
      }
    }
  });

  router.onError((error) => {
    console.error("路由错误", error);
  });
};