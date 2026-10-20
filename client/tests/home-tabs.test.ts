import { flushPromises, mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("swiper/vue", async () => {
  const { defineComponent, h, onMounted } = await import("vue");
  const Swiper = defineComponent({
    props: { initialSlide: { type: Number, default: 0 } },
    emits: ["swiper", "slideChange", "progress"],
    setup(props, { slots, emit }) {
      onMounted(() => emit("swiper", { activeIndex: props.initialSlide }));
      return () => h("div", { "data-active-slide": props.initialSlide }, slots.default?.());
    },
  });
  const SwiperSlide = defineComponent({
    setup(_props, { slots }) {
      return () => h("div", slots.default?.());
    },
  });

  return { Swiper, SwiperSlide };
});

vi.mock("@/components/TabBar.vue", async () => {
  const { defineComponent, h } = await import("vue");
  return {
    default: defineComponent({
      props: { activeTabIndex: { type: Number, required: true } },
      setup(props) {
        return () =>
          h("div", { "data-testid": "active-tab-index" }, String(props.activeTabIndex));
      },
    }),
  };
});

vi.mock("@/views/home/components/Images.vue", async () => {
  const { defineComponent, h } = await import("vue");
  return { default: defineComponent({ setup: () => () => h("div") }) };
});

vi.mock("@/views/home/components/Videos.vue", async () => {
  const { defineComponent, h } = await import("vue");
  return { default: defineComponent({ setup: () => () => h("div") }) };
});

import Home from "@/views/home/index.vue";

describe("home tab initialization", () => {
  beforeEach(() => {
    vi.stubGlobal("matchMedia", vi.fn(() => ({ matches: false })));
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("synchronizes the selected tab with Swiper's initial slide", async () => {
    const wrapper = mount(Home);
    await flushPromises();

    expect(wrapper.get("[data-active-slide]").attributes("data-active-slide")).toBe("2");
    expect(wrapper.get('[data-testid="active-tab-index"]').text()).toBe("2");
    wrapper.unmount();
  });
});
