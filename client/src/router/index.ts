import { createRouter, createWebHistory } from 'vue-router'
import routes from './routes.rotuer'
import { createRouterGuards } from './guard.router';
import type { App } from 'vue';

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes
})

export default router

export const setupRouter = async (app: App) => {
  // 创建路由守卫
  createRouterGuards(router);
  app.use(router);
  // 路由准备就绪后挂载APP实例
  await router.isReady();
};