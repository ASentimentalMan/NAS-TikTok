import { mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";

const { toastShow } = vi.hoisted(() => ({ toastShow: vi.fn() }));

vi.mock("@/components/SimpleToast.vue", async () => {
  const { defineComponent, h } = await import("vue");
  return {
    default: defineComponent({
      setup(_props, { expose }) {
        expose({ show: toastShow });
        return () => h("div");
      },
    }),
  };
});

import App from "@/App.vue";
import { credentialPersistenceNotice } from "@/utils/saved-credentials";

describe("global credential persistence notice", () => {
  afterEach(() => {
    toastShow.mockReset();
    credentialPersistenceNotice.value = "";
  });

  it.each([
    "本机未能安全保存账号密码。",
    "账号偏好无法保存在浏览器中。",
  ])("shows a post-navigation storage notice: %s", async (message) => {
    const wrapper = mount(App, { global: { stubs: { RouterView: true } } });
    credentialPersistenceNotice.value = message;
    await wrapper.vm.$nextTick();

    expect(toastShow).toHaveBeenCalledWith(message);
    expect(credentialPersistenceNotice.value).toBe("");
    wrapper.unmount();
  });
});
